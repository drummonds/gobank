package bank

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/core"
)

// The bank's book: the totals over every customer account and the
// customer count, derived from the ledger's and the customers' contract
// views rather than kept as running totals under a lock (ADR-0004). The
// answer is cached for bookTTL so the dashboard's polling does not
// hammer the database, and a write that moves the book invalidates it,
// so what follows a command is what the command did.

// bookTTL is how long a reading of the book is served before it is read
// again.
const bookTTL = 2 * time.Second

// book is the read model's cache.
type book struct {
	mu        sync.Mutex
	at        time.Time
	valid     bool
	savings   luca.Amount
	lending   luca.Amount
	customers int
}

func (k *book) invalidate() {
	k.mu.Lock()
	k.valid = false
	k.mu.Unlock()
}

// readBook is the book as of now, from the cache while it is fresh.
func (b *Bank) readBook(ctx context.Context) (savings, lending luca.Amount, customers int) {
	b.book.mu.Lock()
	defer b.book.mu.Unlock()
	if b.book.valid && time.Since(b.book.at) < bookTTL {
		return b.book.savings, b.book.lending, b.book.customers
	}
	return b.refreshBookLocked(ctx)
}

// freshBook is the book read now, whatever the cache holds.
func (b *Bank) freshBook(ctx context.Context) (savings, lending luca.Amount, customers int) {
	b.book.mu.Lock()
	defer b.book.mu.Unlock()
	return b.refreshBookLocked(ctx)
}

// refreshBookLocked reads the book and caches it. Must be called with
// book.mu held; readers wait on one another, so the database sees one
// reading at a time.
func (b *Bank) refreshBookLocked(ctx context.Context) (savings, lending luca.Amount, customers int) {
	savings, lending, err := b.customerBalances(ctx)
	if err != nil {
		log.Print(err)
		return b.book.savings, b.book.lending, b.book.customers
	}
	customers, err = b.customers.Count(ctx)
	if err != nil {
		log.Print(err)
		return b.book.savings, b.book.lending, b.book.customers
	}
	b.book.savings, b.book.lending, b.book.customers = savings, lending, customers
	b.book.at, b.book.valid = time.Now(), true
	return savings, lending, customers
}

// customerBalances sums every customer account's live position — the
// latest projection plus the movements valued after its day — per
// family, through the customers' and the ledger's views. The views give
// money in major units as a NUMERIC; it is scaled to whole minor units
// before summing so the sum is exact on both drivers.
func (b *Bank) customerBalances(ctx context.Context) (savings, lending luca.Amount, err error) {
	rows, err := b.db.QueryContext(ctx, fmt.Sprintf(`SELECT ca.product_id, COALESCE(SUM(CAST(ROUND(lp.balance * %d) AS BIGINT)), 0)
		FROM contract_customer_accounts ca
		JOIN contract_ledger_live_positions lp ON lp.account_id = ca.ledger_account_id
		GROUP BY ca.product_id`, ledger.Unit))
	if err != nil {
		return 0, 0, fmt.Errorf("bank: book: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var productID string
		var amount luca.Amount
		if err := rows.Scan(&productID, &amount); err != nil {
			return 0, 0, fmt.Errorf("bank: book: %w", err)
		}
		if p, ok := b.products.ByID(productID); ok && p.Family == gbp.FamilyLending {
			lending += amount
		} else {
			savings += amount
		}
	}
	return savings, lending, rows.Err()
}

// lendingHeadroom is how much more the bank can lend while keeping its
// reserve ratio, given the book plus what an opening in progress adds.
func lendingHeadroom(savings, lending luca.Amount, ratio float64) luca.Amount {
	maxLoans := luca.Amount(float64(savings) * (1 - ratio))
	return maxLoans - lending
}

// --- Interest to date ---

// interestTotals is the bank's customer interest to date on an accrual
// basis: loan interest income and deposit interest expense, each the
// interest applied plus the interest accrued but not yet applied. The
// applications are the balances of the ledger P&L accounts (balance = in
// - out, so they run negative as interest is recognised); the accrual is
// on every account's latest position, summed per product through the
// ledger's end-of-day view and truncated to whole pence per family.
func (b *Bank) interestTotals(ctx context.Context) (loanIncome, depositExpense luca.Amount, err error) {
	chart := b.ledger.Chart
	income, err := b.ledger.Balance(chart.IncomeInterest)
	if err != nil {
		return 0, 0, fmt.Errorf("bank: interest income: %w", err)
	}
	expense, err := b.ledger.Balance(chart.ExpenseInterest)
	if err != nil {
		return 0, 0, fmt.Errorf("bank: interest expense: %w", err)
	}
	accruedSavings, accruedLending, err := b.accruedByFamily(ctx)
	if err != nil {
		return 0, 0, err
	}
	return -income + accruedLending/poundsE7PerPenny, -expense + accruedSavings/poundsE7PerPenny, nil
}

// poundsE7PerPenny is the number of 7dp-pound units in one penny.
const poundsE7PerPenny = 100_000

// accruedByFamily sums accrued-but-unapplied interest over every customer
// account's latest position, per family, at 7dp of a pound.
func (b *Bank) accruedByFamily(ctx context.Context) (savings, lending luca.Amount, err error) {
	// The view publishes accrued as a NUMERIC at 7dp; scaled to an integer
	// before summing so the sum is exact on both drivers.
	rows, err := b.db.QueryContext(ctx, `SELECT ca.product_id, COALESCE(SUM(CAST(ROUND(p.accrued * 10000000) AS BIGINT)), 0)
		FROM contract_customer_accounts ca
		JOIN contract_ledger_eod_positions p ON p.account_id = ca.ledger_account_id
		WHERE p.day = (SELECT MAX(q.day) FROM contract_ledger_eod_positions q WHERE q.account_id = ca.ledger_account_id)
		GROUP BY ca.product_id`)
	if err != nil {
		return 0, 0, fmt.Errorf("bank: accrued interest: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var productID string
		var e7 int64
		if err := rows.Scan(&productID, &e7); err != nil {
			return 0, 0, fmt.Errorf("bank: accrued interest: %w", err)
		}
		if p, ok := b.products.ByID(productID); ok && p.Family == gbp.FamilyLending {
			lending += luca.Amount(e7)
		} else {
			savings += luca.Amount(e7)
		}
	}
	return savings, lending, rows.Err()
}

// --- core.BookQueries ---

// Position implements core.BookQueries: the day and its figures from
// the bank, the book from the read model.
func (b *Bank) Position(ctx context.Context) (core.Position, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	return b.position(ctx), nil
}

func (b *Bank) position(ctx context.Context) core.Position {
	savings, lending, customers := b.readBook(ctx)
	b.mu.Lock()
	defer b.mu.Unlock()
	return core.Position{
		Day:              b.day,
		DayCount:         b.dayCount,
		Customers:        customers,
		Savings:          savings,
		Lending:          lending,
		Cash:             savings - lending,
		RequiredReserves: requiredReserves(savings, b.reserveRatio),
		ReserveRatio:     b.reserveRatio,
		BoERate:          b.boeRate,
		BoEInterest:      b.boeInterestTotalLocked(),
		NIMBps:           b.nimBps,
		DayComplete:      b.dayComplete,
	}
}

// ProfitAndLoss implements core.BookQueries: the bank's P&L to date.
func (b *Bank) ProfitAndLoss(ctx context.Context) (core.ProfitAndLoss, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	return b.profitAndLoss(ctx)
}

func (b *Bank) profitAndLoss(ctx context.Context) (core.ProfitAndLoss, error) {
	b.mu.Lock()
	dayCount := b.dayCount
	boeInterest := b.boeInterestTotalLocked()
	b.mu.Unlock()
	loanIncome, depositExpense, err := b.interestTotals(ctx)
	if err != nil {
		return core.ProfitAndLoss{}, err
	}
	return core.ProfitAndLoss{
		DayCount:               dayCount,
		LoanInterestIncome:     loanIncome,
		BoEInterestIncome:      boeInterest,
		DepositInterestExpense: depositExpense,
		OperatingCosts:         opCostPerDay * luca.Amount(dayCount),
	}, nil
}

// BalanceSheet implements core.BookQueries.
func (b *Bank) BalanceSheet(ctx context.Context) (core.BalanceSheet, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	pos := b.position(ctx)
	pl, err := b.profitAndLoss(ctx)
	if err != nil {
		return core.BalanceSheet{}, err
	}
	holdings, err := b.treasury.Holdings(ctx)
	if err != nil {
		return core.BalanceSheet{}, err
	}
	var gilts luca.Amount
	for _, h := range holdings {
		gilts += h.FaceValue
	}
	retained := pl.NetProfit()
	return core.BalanceSheet{
		DayCount:           pos.DayCount,
		Loans:              pos.Lending,
		Gilts:              gilts,
		CashAtBoE:          max(pos.Savings-pos.Lending+retained-gilts, 0), // gilts are bought from cash
		Deposits:           pos.Savings,
		RetainedEarnings:   retained,
		RiskWeightedAssets: pos.Lending,
	}, nil
}

// History implements core.BookQueries: the daily series on record.
func (b *Bank) History(ctx context.Context) (core.History, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	return b.history.Series(ctx)
}
