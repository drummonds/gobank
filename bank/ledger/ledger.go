// Package ledger is the bank's books of account (ADR-0002 stage 5, story
// 1.5.3): go-luca's double-entry ledger over the bank's database, with the
// chart of accounts resolved, one lock per account, the posting of an
// event with projections and the reading of an account's live position.
// go-luca owns the tables and publishes the contract views
// (contract_ledger_movements, contract_ledger_eod_positions,
// contract_ledger_live_positions) when its schema runs; the components
// above post through this package and read through the views.
package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
	"sync"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
)

// The chart of accounts: the bank's own accounts at fixed paths, and the
// roots customer accounts hang under, by the product's family.
const (
	EquityCapital   = "Equity:Capital"            // the bank's own money: a movement from it into a customer account is a deposit or a loan drawdown
	ExpenseInterest = "Expense:Interest"          // interest applied to savings
	IncomeInterest  = "Income:Interest"           // interest applied to loans
	IncomeBoE       = "Income:Interest:BoE"       // BoE reserve interest, recognised as it accrues
	AccruedBoE      = "Asset:AccruedInterest:BoE" // the receivable it builds up in
	BoEReserves     = "Asset:BoEReserves"         // the reserves it is received into
	SavingsRoot     = "Liability:Savings"         // SavingsRoot:<customer>:<product>
	LoansRoot       = "Asset:Loans"               // LoansRoot:<customer>:<product>
)

// Every account the bank opens is GBP in pence.
const (
	Currency = "GBP"
	Exponent = -2
)

// positionScale is the decimal places the views give accrued interest.
const positionScale = 7

// Chart is the bank's own accounts, resolved to ledger account IDs when
// the ledger opens.
type Chart struct {
	EquityCapital, ExpenseInterest, IncomeInterest, IncomeBoE, AccruedBoE, BoEReserves string
}

// id is the chart's ID for a path, "" if the path is not on the chart.
func (c Chart) id(path string) string {
	switch path {
	case EquityCapital:
		return c.EquityCapital
	case ExpenseInterest:
		return c.ExpenseInterest
	case IncomeInterest:
		return c.IncomeInterest
	case IncomeBoE:
		return c.IncomeBoE
	case AccruedBoE:
		return c.AccruedBoE
	case BoEReserves:
		return c.BoEReserves
	}
	return ""
}

// Ledger is the books over the bank's database. go-luca's ledger is
// embedded: the components above read positions and balances and record
// movements through it directly.
type Ledger struct {
	*luca.SQLLedger
	Chart Chart
	db    *sql.DB
	locks *locks
}

// Open opens the books on db, creating go-luca's tables and views on first
// use, and resolves the chart of accounts, opening any account not yet
// there.
func Open(db *sql.DB) (*Ledger, error) {
	books, err := luca.NewSQLLedger(db)
	if err != nil {
		return nil, fmt.Errorf("ledger: open: %w", err)
	}
	l := &Ledger{SQLLedger: books, db: db, locks: &locks{}}
	for _, c := range []struct {
		path string
		dst  *string
	}{
		{EquityCapital, &l.Chart.EquityCapital},
		{ExpenseInterest, &l.Chart.ExpenseInterest},
		{IncomeInterest, &l.Chart.IncomeInterest},
		{IncomeBoE, &l.Chart.IncomeBoE},
		{AccruedBoE, &l.Chart.AccruedBoE},
		{BoEReserves, &l.Chart.BoEReserves},
	} {
		id, err := l.Account(c.path)
		if err != nil {
			return nil, err
		}
		*c.dst = id
	}
	return l, nil
}

// WithTx is the same books bound to tx: what it writes is in the
// transaction. The chart and the locks are shared.
func (l *Ledger) WithTx(tx *sql.Tx) *Ledger {
	return &Ledger{SQLLedger: l.SQLLedger.WithTx(tx), Chart: l.Chart, db: l.db, locks: l.locks}
}

// Account is the ID of the account at path, opened if it is not there.
func (l *Ledger) Account(path string) (string, error) {
	if id := l.Chart.id(path); id != "" {
		return id, nil
	}
	acct, err := l.GetAccount(path)
	if err != nil {
		return "", fmt.Errorf("ledger: account %s: %w", path, err)
	}
	if acct == nil {
		if acct, err = l.CreateAccount(path, Currency, Exponent, 0); err != nil {
			return "", fmt.Errorf("ledger: open %s: %w", path, err)
		}
	}
	return acct.ID, nil
}

// OpenCustomerAccount opens a customer's account for a product on the
// chart: under the loans root for a lending product, the savings root
// otherwise. Returns the ledger account ID.
func (l *Ledger) OpenCustomerAccount(customerID, productID string, family gbp.ProductFamily) (string, error) {
	root := SavingsRoot
	if family == gbp.FamilyLending {
		root = LoansRoot
	}
	path := fmt.Sprintf("%s:%s:%s", root, customerID, productID)
	acct, err := l.CreateAccount(path, Currency, Exponent, 0)
	if err != nil {
		return "", fmt.Errorf("ledger: open %s: %w", path, err)
	}
	return acct.ID, nil
}

// Post writes an event — a transfer, a funding — with projections, so
// both accounts' positions for the day move at once. The caller holds
// the accounts' locks for as long as its transaction holds their rows.
func (l *Ledger) Post(fromID, toID string, amount luca.Amount, code string, day time.Time, description string) error {
	if fromID == "" || toID == "" {
		return fmt.Errorf("ledger: post %s: both accounts are needed", description)
	}
	if _, err := l.RecordMovementWithProjections(fromID, toID, amount, code, day, description); err != nil {
		return fmt.Errorf("ledger: post %s: %w", description, err)
	}
	return nil
}

// Position is an account's position as the ledger publishes it now: the
// latest projection plus the movements since.
type Position struct {
	Balance   luca.Amount // minor units
	AccruedE7 int64       // ten-millionths of the major unit
}

// LivePosition reads one account from contract_ledger_live_positions,
// which publishes money in major units at the commodity's exponent and
// accrued interest at positionScale. An account with no row is an error.
func (l *Ledger) LivePosition(ctx context.Context, accountID string) (Position, error) {
	var balance, accrued string
	err := l.db.QueryRowContext(ctx, `SELECT balance, accrued FROM contract_ledger_live_positions WHERE account_id = $1`, accountID).
		Scan(&balance, &accrued)
	if err != nil {
		return Position{}, fmt.Errorf("ledger: live position %s: %w", accountID, err)
	}
	minor, err := parseScaled(balance, -Exponent)
	if err != nil {
		return Position{}, fmt.Errorf("ledger: live position %s: balance %q: %w", accountID, balance, err)
	}
	e7, err := parseScaled(accrued, positionScale)
	if err != nil {
		return Position{}, fmt.Errorf("ledger: live position %s: accrued %q: %w", accountID, accrued, err)
	}
	return Position{Balance: luca.Amount(minor), AccruedE7: e7}, nil
}

// parseScaled reads a NUMERIC rendered as text ("1001.27", "-0.4109589")
// as an integer at scale decimal places, exactly: no floating point, and
// an error if the text carries more non-zero places than scale.
func parseScaled(s string, scale int) (int64, error) {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	whole, frac := s, ""
	if before, after, ok := strings.Cut(s, "."); ok {
		whole, frac = before, after
	}
	if len(frac) > scale {
		if strings.Trim(frac[scale:], "0") != "" {
			return 0, fmt.Errorf("more than %d decimal places", scale)
		}
		frac = frac[:scale]
	}
	frac += strings.Repeat("0", scale-len(frac))
	digits := strings.TrimLeft(whole+frac, "0")
	if digits == "" {
		return 0, nil
	}
	var n int64
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int64(c-'0')
	}
	if neg {
		n = -n
	}
	return n, nil
}

// locks serialise the work on one account. An in-day event (a transfer,
// a funding) and the daily pass each rewrite the account's position
// inside a transaction that holds its rows; one of them completes before
// the other starts, and every other account is free meanwhile. That is
// the cost of projecting one day ahead: in-day events are heavier, and
// end-of-day processing is smeared across the day. A lock is held for as
// long as its transaction holds the rows, and several are taken in a
// fixed order, so no two holders can wait on each other.
//
// The locks are striped: an account maps to one of lockStripes mutexes by
// a hash of its ID, so the set costs a few kilobytes however many accounts
// exist (a mutex per account would be ~150 bytes each, never freed). Two
// accounts that share a stripe take turns needlessly; with a handful of
// concurrent holders that is rare.
type locks struct {
	m [lockStripes]sync.Mutex
}

const lockStripes = 4096

func stripe(id string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return int(h.Sum32() % lockStripes)
}

// Lock takes the accounts' locks, in a fixed order, and returns the
// release. An empty ID is skipped; the same account twice is one lock.
func (l *Ledger) Lock(ids ...string) (unlock func()) {
	stripes := make([]int, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			stripes = append(stripes, stripe(id))
		}
	}
	slices.Sort(stripes)
	stripes = slices.Compact(stripes)
	for _, s := range stripes {
		l.locks.m[s].Lock()
	}
	return func() {
		for i := len(stripes) - 1; i >= 0; i-- {
			l.locks.m[stripes[i]].Unlock()
		}
	}
}
