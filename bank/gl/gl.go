// Package gl is the bank's general ledger (ADR-0005, stage 6a): a second
// go-luca ledger in the bank's database, named apart by the gl_ prefix,
// whose chart never grows with customers. It holds a control account per
// product and the bank's own accounts, is posted per event from the
// sub-ledger's movements (inserts only) and by a journal at the GL close
// that follows the start-of-day pass, and is reconciled to the
// sub-ledger daily. It reads the sub-ledger and the customers' accounts
// through their contract views alone.
package gl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/bank/products"
	"git.bytestone.uk/hum3/gobank/bank/schema"
)

// Prefix names the general ledger's tables and views apart from the
// sub-ledger's: gl_movements, contract_gl_ledger_movements.
const Prefix = "gl_"

// The chart: the bank's own accounts at the sub-ledger's paths, and the
// roots the control accounts hang under, one per product.
const (
	EquityCapital   = ledger.EquityCapital
	ExpenseInterest = ledger.ExpenseInterest
	IncomeInterest  = ledger.IncomeInterest
	SavingsRoot     = ledger.SavingsRoot // SavingsRoot:<product>
	LoansRoot       = ledger.LoansRoot   // LoansRoot:<product>
)

// Chart is the bank's own accounts on the general ledger, resolved to
// account IDs when it opens.
type Chart struct {
	EquityCapital, ExpenseInterest, IncomeInterest string
}

// querier is the connection or the transaction the ledger's own tables
// and the sub-ledger's views are read and written through.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// opening is the general ledger's opening on record: the day it opened,
// and whether it adopted a bank already running, whose sub-ledger holds
// events the GL was never posted for. An adopted GL's first close takes
// each control's opening balance from the sub-ledger instead of
// journalling.
type opening struct {
	day     time.Time
	adopted bool
}

// Ledger is the general ledger over the bank's database. go-luca's
// ledger is embedded, prefixed.
type Ledger struct {
	*luca.SQLLedger
	Chart    Chart
	controls map[string]string // product ID → control account ID
	db       *sql.DB
	q        querier
	opening  opening
}

// Open opens the general ledger on db, creating its tables and views on
// first use, resolves the chart and a control account per product in the
// catalogue, and records its opening if it has none: on day, adopting a
// running bank or not.
func Open(db *sql.DB, catalogue []products.Product, day time.Time, adopting bool) (*Ledger, error) {
	books, err := luca.NewSQLLedger(db, luca.Prefix(Prefix))
	if err != nil {
		return nil, fmt.Errorf("gl: open: %w", err)
	}
	g := &Ledger{SQLLedger: books, controls: map[string]string{}, db: db, q: db}
	for _, c := range []struct {
		path string
		dst  *string
	}{
		{EquityCapital, &g.Chart.EquityCapital},
		{ExpenseInterest, &g.Chart.ExpenseInterest},
		{IncomeInterest, &g.Chart.IncomeInterest},
	} {
		if *c.dst, err = g.account(c.path); err != nil {
			return nil, err
		}
	}
	for _, p := range catalogue {
		root := SavingsRoot
		if p.Family == gbp.FamilyLending {
			root = LoansRoot
		}
		id, err := g.account(root + ":" + p.ID)
		if err != nil {
			return nil, err
		}
		g.controls[p.ID] = id
	}
	if err := g.recordOpening(context.Background(), day, adopting); err != nil {
		return nil, err
	}
	return g, nil
}

// account is the ID of the account at path, opened if it is not there.
func (g *Ledger) account(path string) (string, error) {
	acct, err := g.GetAccount(path)
	if err != nil {
		return "", fmt.Errorf("gl: account %s: %w", path, err)
	}
	if acct == nil {
		if acct, err = g.CreateAccount(path, ledger.Currency, ledger.Exponent, 0); err != nil {
			return "", fmt.Errorf("gl: open %s: %w", path, err)
		}
	}
	return acct.ID, nil
}

// recordOpening writes the opening once; a record already there stands.
func (g *Ledger) recordOpening(ctx context.Context, day time.Time, adopting bool) error {
	var adopted int
	err := g.q.QueryRowContext(ctx, `SELECT day, adopted FROM gl_opening`).Scan(&g.opening.day, &adopted)
	switch {
	case err == nil:
		g.opening.day, g.opening.adopted = g.opening.day.UTC(), adopted != 0
		return nil
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("gl: opening: %w", err)
	}
	g.opening = opening{day: dayOf(day), adopted: adopting}
	if _, err := g.q.ExecContext(ctx, `INSERT INTO gl_opening (day, adopted) VALUES ($1, $2)`, g.opening.day, boolInt(adopting)); err != nil {
		return fmt.Errorf("gl: opening: %w", err)
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// dayOf is t's day at midnight UTC.
func dayOf(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// WithTx is the same ledger bound to tx: what it writes, to its own
// tables and go-luca's, is in the transaction. The chart is shared.
func (g *Ledger) WithTx(tx *sql.Tx) *Ledger {
	return &Ledger{SQLLedger: g.SQLLedger.WithTx(tx), Chart: g.Chart, controls: g.controls, db: g.db, q: tx, opening: g.opening}
}

// Control is the control account for a product; ok is false for a
// product not in the catalogue.
func (g *Ledger) Control(productID string) (id string, ok bool) {
	id, ok = g.controls[productID]
	return id, ok
}

// Funding posts a deposit or a loan drawdown: equity to the product's
// control, valued on day, carrying the payment reference. An insert
// only: the control's position moves at the close.
func (g *Ledger) Funding(day time.Time, productID string, amount luca.Amount, reference string) error {
	control, ok := g.controls[productID]
	if !ok {
		return fmt.Errorf("gl: funding %s: no control account for product %s", reference, productID)
	}
	if _, err := g.RecordMovement(g.Chart.EquityCapital, control, amount, luca.CodeBookTransfer, day, reference); err != nil {
		return fmt.Errorf("gl: funding %s: %w", reference, err)
	}
	return nil
}

// Transfer posts a transfer between customer accounts: control to control
// when the products differ, nothing within one product.
func (g *Ledger) Transfer(day time.Time, fromProduct, toProduct string, amount luca.Amount, reference string) error {
	if fromProduct == toProduct {
		return nil
	}
	from, ok := g.controls[fromProduct]
	if !ok {
		return fmt.Errorf("gl: transfer %s: no control account for product %s", reference, fromProduct)
	}
	to, ok := g.controls[toProduct]
	if !ok {
		return fmt.Errorf("gl: transfer %s: no control account for product %s", reference, toProduct)
	}
	if _, err := g.RecordMovement(from, to, amount, luca.CodeBookTransfer, day, reference); err != nil {
		return fmt.Errorf("gl: transfer %s: %w", reference, err)
	}
	return nil
}

// Schema: the general ledger's own record beside go-luca's prefixed
// tables. The opening (one row), the closes (the posted-through day is
// the latest), the journal (one row per day, product and code) and the
// reconciliations (one row per day and product).
var Schema = schema.Component{Name: "gl", Migrations: []schema.Migration{
	{Version: 1, Statements: []string{
		`CREATE TABLE IF NOT EXISTS gl_opening (
			day TIMESTAMP NOT NULL,
			adopted INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS gl_closes (
			day TIMESTAMP PRIMARY KEY,
			closed_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS gl_journal (
			day TIMESTAMP NOT NULL,
			product_id VARCHAR(64) NOT NULL,
			code VARCHAR(32) NOT NULL,
			amount BIGINT NOT NULL,
			movement_id VARCHAR(36) NOT NULL,
			PRIMARY KEY (day, product_id, code)
		)`,
		`CREATE TABLE IF NOT EXISTS gl_reconciliations (
			day TIMESTAMP NOT NULL,
			product_id VARCHAR(64) NOT NULL,
			control BIGINT NOT NULL,
			sub_ledger BIGINT NOT NULL,
			PRIMARY KEY (day, product_id)
		)`,
	}},
}}
