package bank

import (
	"context"
	"fmt"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/core"
)

// The bank's book: the totals over every customer account and the
// customer count, derived from the ledger's and the customers' contract
// views rather than kept as running totals under a lock (ADR-0004). The
// views aggregate every account, which takes seconds on a large bank, so
// the book and the interest to date are readings taken in the
// background: pages serve the last reading at once, and one older than
// bookTTL is taken again behind them. A write that moves the book
// invalidates it, so what follows a command is what the command did.

// bookTTL is how long a reading of the book is served before it is taken
// again.
const bookTTL = 2 * time.Second

// bookFigures is one reading of the book: the totals by family, each
// product's share, and the customer count.
type bookFigures struct {
	savings, lending luca.Amount
	products         map[string]productBook // by product ID
	customers        int
	cash             luca.Amount // the Asset:Cash balance: deposits in, drawdowns out
}

// productBook is one product's share of the book.
type productBook struct {
	accounts int
	balance  luca.Amount
}

// readBook is the read behind the book: the balances through the live
// view and the customer count.
func (b *Bank) readBook(ctx context.Context) (bookFigures, error) {
	products, err := b.productBooks(ctx)
	if err != nil {
		return bookFigures{}, err
	}
	customers, err := b.customers.Count(ctx)
	if err != nil {
		return bookFigures{}, err
	}
	cash, err := b.ledger.Balance(b.ledger.Chart.Cash)
	if err != nil {
		return bookFigures{}, fmt.Errorf("bank: cash: %w", err)
	}
	k := bookFigures{products: products, customers: customers, cash: cash}
	// The book's totals are magnitudes read from the signed ledger
	// (ADR-0008): savings balances are liabilities and negative.
	for id, pb := range products {
		if family, ok := b.products.Family(id); ok && family == gbp.FamilyLending {
			k.lending += pb.balance
		} else {
			k.savings -= pb.balance
		}
	}
	return k, nil
}

// freshBook is the book read now, in front of the caller: what a day's
// close and a snapshot are made from.
func (b *Bank) freshBook(ctx context.Context) (savings, lending luca.Amount, customers int) {
	k := b.book.fresh(ctx)
	return k.savings, k.lending, k.customers
}

// productBooks counts every customer account and sums its live position
// — the latest projection plus the movements valued after its day — per
// product, through the customers' and the ledger's views. The views give
// money in major units as a NUMERIC; it is scaled to whole minor units
// before summing so the sum is exact on both drivers.
func (b *Bank) productBooks(ctx context.Context) (map[string]productBook, error) {
	rows, err := b.db.QueryContext(ctx, fmt.Sprintf(`SELECT ca.product_id, COUNT(*), COALESCE(SUM(CAST(ROUND(lp.balance * %d) AS BIGINT)), 0)
		FROM contract_customer_accounts ca
		JOIN contract_ledger_live_positions lp ON lp.account_id = ca.ledger_account_id
		GROUP BY ca.product_id`, ledger.Unit))
	if err != nil {
		return nil, fmt.Errorf("bank: book: %w", err)
	}
	defer rows.Close()
	out := map[string]productBook{}
	for rows.Next() {
		var productID string
		var pb productBook
		if err := rows.Scan(&productID, &pb.accounts, &pb.balance); err != nil {
			return nil, fmt.Errorf("bank: book: %w", err)
		}
		out[productID] = pb
	}
	return out, rows.Err()
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
	f := b.interest.get(ctx)
	return f.loanIncome, f.depositExpense, nil
}

// interestFigures is one reading of the interest to date: the two P&L
// figures, and the accrued-but-unapplied part of each in whole pence,
// which the balance sheet shows as receivable and payable. One reading
// serves both statements, so they agree whatever its age.
type interestFigures struct {
	loanIncome, depositExpense     luca.Amount
	accruedLending, accruedSavings luca.Amount // magnitudes, pence
}

// readInterest is the read behind interestTotals.
func (b *Bank) readInterest(ctx context.Context) (interestFigures, error) {
	chart := b.ledger.Chart
	income, err := b.ledger.Balance(chart.IncomeInterest)
	if err != nil {
		return interestFigures{}, fmt.Errorf("bank: interest income: %w", err)
	}
	expense, err := b.ledger.Balance(chart.ExpenseInterest)
	if err != nil {
		return interestFigures{}, fmt.Errorf("bank: interest expense: %w", err)
	}
	accruedSavings, accruedLending, err := b.accruedByFamily(ctx)
	if err != nil {
		return interestFigures{}, err
	}
	// Income is a credit and runs negative, the expense a debit and
	// positive; savings accrue negative (ADR-0008). Both figures are
	// magnitudes.
	return interestFigures{
		loanIncome:     -income + accruedLending/poundsE7PerPenny,
		depositExpense: expense - accruedSavings/poundsE7PerPenny,
		accruedLending: accruedLending / poundsE7PerPenny,
		accruedSavings: -accruedSavings / poundsE7PerPenny,
	}, nil
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
		if family, ok := b.products.Family(productID); ok && family == gbp.FamilyLending {
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
	k := b.book.get(ctx)
	b.mu.Lock()
	defer b.mu.Unlock()
	return core.Position{
		Day:              b.day,
		DayCount:         b.dayCount,
		Customers:        k.customers,
		Savings:          k.savings,
		Lending:          k.lending,
		Cash:             k.cash,
		RequiredReserves: requiredReserves(k.savings, b.reserveRatio),
		ReserveRatio:     b.reserveRatio,
		RiskFreeBps:      b.riskFreeBps,
		BoEInterest:      b.boeInterestTotalLocked(),
		NIMBps:           b.nimBps,
		DayComplete:      b.dayComplete,
		GL:               b.glStatus,
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
	dayCount, from, to := b.dayCount, b.opened, b.day
	b.mu.Unlock()
	loanIncome, depositExpense, err := b.interestTotals(ctx)
	if err != nil {
		return core.ProfitAndLoss{}, err
	}
	chart := b.ledger.Chart
	boeIncome, err := b.ledger.Balance(chart.IncomeBoE)
	if err != nil {
		return core.ProfitAndLoss{}, fmt.Errorf("bank: BoE interest income: %w", err)
	}
	operating, err := b.ledger.Balance(chart.ExpenseOperating)
	if err != nil {
		return core.ProfitAndLoss{}, fmt.Errorf("bank: operating costs: %w", err)
	}
	// Income runs negative in the bank's view (ADR-0008); the statement
	// shows magnitudes.
	return core.ProfitAndLoss{
		From: from, To: to, YearEnd: b.definition.YearEnd.Next(to),
		DayCount:               dayCount,
		LoanInterestIncome:     loanIncome,
		BoEInterestIncome:      -boeIncome,
		DepositInterestExpense: depositExpense,
		OperatingCosts:         operating,
	}, nil
}

// BalanceSheet implements core.BookQueries: every line is read from the
// ledger — the bank's own accounts by balance, the two books through the
// position — and the customers' accrued interest, which the P&L counts,
// is shown on both sides. The P&L is the same reading, so the sheet
// balances by construction.
func (b *Bank) BalanceSheet(ctx context.Context) (core.BalanceSheet, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	pos := b.position(ctx)
	pl, err := b.profitAndLoss(ctx)
	if err != nil {
		return core.BalanceSheet{}, err
	}
	chart := b.ledger.Chart
	balance := func(id, what string) (luca.Amount, error) {
		v, err := b.ledger.Balance(id)
		if err != nil {
			return 0, fmt.Errorf("bank: %s: %w", what, err)
		}
		return v, nil
	}
	reserves, err := balance(chart.BoEReserves, "BoE reserves")
	if err != nil {
		return core.BalanceSheet{}, err
	}
	accruedBoE, err := balance(chart.AccruedBoE, "BoE receivable")
	if err != nil {
		return core.BalanceSheet{}, err
	}
	gilts, err := balance(chart.Gilts, "gilts")
	if err != nil {
		return core.BalanceSheet{}, err
	}
	capital, err := balance(chart.EquityCapital, "capital")
	if err != nil {
		return core.BalanceSheet{}, err
	}
	accrued := b.interest.get(ctx) // the reading the P&L above was made from
	return core.BalanceSheet{
		AsAt:                      pos.Day,
		DayCount:                  pos.DayCount,
		Loans:                     pos.Lending,
		AccruedInterestReceivable: accrued.accruedLending + accruedBoE,
		Gilts:                     gilts,
		CashAtBoE:                 pos.Cash + reserves,
		Deposits:                  pos.Savings,
		AccruedInterestPayable:    accrued.accruedSavings,
		Capital:                   -capital, // equity runs negative in the bank's view
		RetainedEarnings:          pl.NetProfit(),
		RiskWeightedAssets:        pos.Lending,
	}, nil
}

// History implements core.BookQueries: the daily series on record.
func (b *Bank) History(ctx context.Context) (core.History, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	return b.history.Series(ctx)
}
