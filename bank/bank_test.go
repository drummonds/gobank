package bank

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	_ "git.bytestone.uk/hum3/go-postgres"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/customers"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/bank/schema"
	"git.bytestone.uk/hum3/gobank/core"
)

// A bank in a test: over an in-memory database with every component's
// schema applied, on a clock the test moves and a flat base rate.
type fixture struct {
	t     *testing.T
	db    *sql.DB
	now   time.Time
	bank  *Bank
	ctx   context.Context
	ratio float64
}

var opening = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

func migrate(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, c := range Schemas() {
		for _, m := range c.Migrations {
			for _, stmt := range m.Statements {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatalf("schema %s v%d: %v", c.Name, m.Version, err)
				}
			}
		}
	}
}

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pglike", "file::memory:?_pragma=temp_store(2)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	migrate(t, db)
	return db
}

func open(t *testing.T) *fixture {
	t.Helper()
	return reopen(t, openDB(t), opening)
}

// reopen is the next process over db: a bank opened on it at wall time at.
func reopen(t *testing.T, db *sql.DB, at time.Time) *fixture {
	t.Helper()
	f := &fixture{t: t, db: db, now: at, ctx: context.Background(), ratio: DefaultReserveRatio}
	b, err := Open(db, Options{
		Clock: core.ClockFunc(func() time.Time { return f.now }),
		Rates: core.BaseRateFunc(func(time.Time) float64 { return 0.05 }),
		Seed:  42,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.bank = b
	return f
}

// nextDay moves the clock on a day and has the bank follow it.
func (f *fixture) nextDay() {
	f.t.Helper()
	f.now = f.now.AddDate(0, 0, 1)
	if _, err := f.bank.StartDay(f.ctx); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) position() core.Position {
	f.t.Helper()
	p, err := f.bank.Position(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

var ada = core.NewCustomer{
	KYC:      core.KYC{Verified: true, RiskRating: "Standard"},
	PII:      core.PII{Name: "Ada Lovelace", NI: "AB123456C", DOB: "1815-12-10", Address: "12 St James's Square", Email: "ada@example.com", Phone: "07000 000001"},
	Accounts: []core.NewAccount{{ProductID: gbp.EasyAccess().ID, Opening: 1000_00}, {ProductID: gbp.PersonalLoan().ID, Opening: 2000_00}},
}

func saver(name string, amount luca.Amount) core.NewCustomer {
	return core.NewCustomer{PII: core.PII{Name: name}, Accounts: []core.NewAccount{{ProductID: gbp.EasyAccess().ID, Opening: amount}}}
}

// Every component's schema is listed once, in version order.
func TestSchemasAreEveryComponents(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Schemas() {
		if seen[c.Name] {
			t.Errorf("schema %s listed twice", c.Name)
		}
		seen[c.Name] = true
		for i, m := range c.Migrations {
			if m.Version != i+1 {
				t.Errorf("schema %s: migration %d listed where version %d is due", c.Name, m.Version, i+1)
			}
		}
	}
	for _, want := range []string{"products", "customers", "payments", "treasury", "history"} {
		if !seen[want] {
			t.Errorf("schema %s missing", want)
		}
	}
	var _ schema.Component = Schemas()[0]
}

// A fresh bank is on its clock's day with nothing on the books; the
// opening day is on record.
func TestAFreshBankIsOnItsClocksDay(t *testing.T) {
	f := open(t)
	p := f.position()
	if !p.Day.Equal(opening) || p.DayCount != 0 || p.Customers != 0 || p.Savings != 0 || p.Lending != 0 || !p.DayComplete || p.BoERate != 0.05 {
		t.Errorf("position %+v; want the opening day, empty, complete", p)
	}
	h, _ := f.bank.History(f.ctx)
	if len(h.Balances) != 1 || !h.Balances[0].Date.Equal(opening) {
		t.Errorf("history %+v; want the opening day's snapshot", h)
	}
}

// Opening a customer is a command: through it alone the customer is
// registered, each account is funded by a payment on record and has its
// position for the day it opens, the loan is kept within the bank's
// lending headroom, and the position shows it at once.
func TestOpenCustomerFundsAndProjectsOnTheOpeningDay(t *testing.T) {
	f := open(t)
	rec, err := f.bank.OpenCustomer(f.ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != core.CustomerID(1) {
		t.Errorf("first customer is %s", rec.ID)
	}
	const deposit = luca.Amount(1000_00)
	wantLoan := luca.Amount(float64(deposit) * (1 - DefaultReserveRatio))
	if len(rec.Accounts) != 2 || rec.Accounts[0].Balance != deposit || rec.Accounts[1].Balance != wantLoan {
		t.Fatalf("accounts %+v; want the deposit %d and the loan trimmed to %d", rec.Accounts, deposit, wantLoan)
	}
	cust, _ := f.bank.Customers().ByID(f.ctx, rec.ID)
	for _, a := range cust.Accounts {
		pos, err := f.bank.Ledger().PositionAt(a.LedgerAccountID, opening)
		if err != nil || pos == nil {
			t.Fatalf("%s: no position on the opening day: %v %v", a.ProductName, pos, err)
		}
		if pos.Balance != a.Balance || pos.Accrued.Num != int64(a.Balance)*gbp.RateBps(a.Rate) {
			t.Errorf("%s: position %+v; want balance %d accruing a day at %v", a.ProductName, pos, a.Balance, a.Rate)
		}
	}
	if pp, _ := f.bank.PaymentPage(f.ctx, 1); pp.Total != 2 || pp.Payments[0].Type != core.PaymentLoan || pp.Payments[1].Type != core.PaymentDeposit {
		t.Errorf("payments %+v; want the deposit and the loan, newest first", pp)
	}
	p := f.position()
	if p.Savings != deposit || p.Lending != wantLoan || p.Customers != 1 {
		t.Errorf("position %+v; want savings %d, lending %d, one customer, at once", p, deposit, wantLoan)
	}
	if pending, _ := f.bank.Catalogue().AnyUnprojected(f.ctx, opening); pending {
		t.Error("the day's pass has work left after the opening")
	}
	// A lending-only opening with no deposits on the books lends nothing.
	none, err := f.bank.OpenCustomer(f.ctx, core.NewCustomer{PII: core.PII{Name: "No Headroom"}, Accounts: []core.NewAccount{{ProductID: gbp.Mortgage().ID, Opening: 50_000_00}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(none.Accounts) != 1 || none.Accounts[0].Balance != 0 {
		t.Errorf("accounts %+v; want one empty lending account", none.Accounts)
	}
	if pp, _ := f.bank.PaymentPage(f.ctx, 1); pp.Total != 2 {
		t.Errorf("%d payments, want still 2: an empty account raises none", pp.Total)
	}
	for _, c := range []struct {
		n    core.NewCustomer
		want error
	}{
		{core.NewCustomer{}, core.ErrInvalidAmount},
		{core.NewCustomer{Accounts: []core.NewAccount{{ProductID: "no-such-product"}}}, core.ErrNotFound},
	} {
		if _, err := f.bank.OpenCustomer(f.ctx, c.n); !errors.Is(err, c.want) {
			t.Errorf("OpenCustomer(%+v): %v, want %v", c.n, err, c.want)
		}
	}
}

// The bank follows its clock a day at a time: each day begun is on
// record, every account's position for it is written by the pass, and the
// day is complete when they all are.
func TestStartDayFollowsTheClock(t *testing.T) {
	f := open(t)
	if _, err := f.bank.OpenCustomer(f.ctx, ada); err != nil {
		t.Fatal(err)
	}
	if day, err := f.bank.StartDay(f.ctx); err != nil || !day.Equal(opening) {
		t.Errorf("StartDay on the clock's day moved to %s, %v", day, err)
	}
	f.now = f.now.AddDate(0, 0, 3) // the clock jumps three days
	day, err := f.bank.StartDay(f.ctx)
	if err != nil || !day.Equal(opening.AddDate(0, 0, 3)) {
		t.Fatalf("StartDay = %s, %v; want three days on", day, err)
	}
	p := f.position()
	if p.DayCount != 3 || !p.DayComplete {
		t.Errorf("position %+v; want day 3, complete", p)
	}
	h, _ := f.bank.History(f.ctx)
	if len(h.Balances) != 4 {
		t.Errorf("%d days on record, want 4", len(h.Balances))
	}
	cust, _ := f.bank.Customers().ByID(f.ctx, core.CustomerID(1))
	for _, a := range cust.Accounts {
		for d := range 4 {
			on := opening.AddDate(0, 0, d)
			pos, err := f.bank.Ledger().PositionAt(a.LedgerAccountID, on)
			if err != nil || pos == nil || !pos.Day.Equal(on) {
				t.Errorf("%s: no position for %s: %v %v", a.ProductName, on.Format(time.DateOnly), pos, err)
			}
		}
	}
}

// A day's pass cut short is resumed by the next StartDay from the
// projections already written: the day does not advance again.
func TestACutShortPassIsResumed(t *testing.T) {
	f := open(t)
	for i := range 3 {
		if _, err := f.bank.OpenCustomer(f.ctx, saver("S", luca.Amount(100_00*(i+1)))); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(f.ctx)
	visited := 0
	f.bank.SetPassHook(func() {
		if visited++; visited == 1 {
			cancel()
		}
	})
	f.now = f.now.AddDate(0, 0, 1)
	f.bank.StartDay(ctx) //nolint:errcheck
	f.bank.SetPassHook(nil)
	day2 := opening.AddDate(0, 0, 1)
	if p := f.position(); !p.Day.Equal(day2) || p.DayComplete {
		t.Fatalf("after the cut-short day: %+v; want day 2, incomplete", p)
	}
	if pending, _ := f.bank.Catalogue().AnyUnprojected(f.ctx, day2); !pending {
		t.Fatal("nothing left unprojected; the test would be vacuous")
	}
	if _, err := f.bank.StartDay(f.ctx); err != nil {
		t.Fatal(err)
	}
	if p := f.position(); !p.Day.Equal(day2) || p.DayCount != 1 || !p.DayComplete {
		t.Errorf("after finishing the day: %+v; want still day 2, complete", p)
	}
	if n, _ := f.bank.Catalogue().CountUnprojected(f.ctx, day2); n != 0 {
		t.Errorf("%d accounts still unprojected", n)
	}
}

// assertPositionsAreTheTruth checks every account's position for today
// is what the read model shows, and that the BoE reserves position for
// the day just closed carries the bank's numerator.
func assertPositionsAreTheTruth(t *testing.T, f *fixture) {
	t.Helper()
	today := f.position().Day
	for _, c := range allCustomers(t, f) {
		for _, a := range c.Accounts {
			p, err := f.bank.Ledger().PositionAt(a.LedgerAccountID, today)
			if err != nil || p == nil {
				t.Fatalf("%s %s: position for %s: %v %v", c.ID, a.ProductName, today.Format(time.DateOnly), p, err)
			}
			if !p.Day.Equal(today) || p.Accrued.Den != gbp.AccrualDenominator {
				t.Errorf("%s %s: latest position %+v, want today's at %d", c.ID, a.ProductName, p, gbp.AccrualDenominator)
			}
			if a.AccruedE7 != accrualPoundsE7(p.Accrued.Num) || a.Balance != p.Balance {
				t.Errorf("%s %s: read model %d/%d != position %d/%d", c.ID, a.ProductName, a.Balance, a.AccruedE7, p.Balance, accrualPoundsE7(p.Accrued.Num))
			}
		}
	}
	closed := today.AddDate(0, 0, -1)
	p, err := f.bank.Ledger().PositionAt(f.bank.Ledger().Chart.BoEReserves, closed)
	if err != nil || p == nil || !p.Day.Equal(closed) {
		t.Fatalf("BoE reserves position for %s: %v %v", closed.Format(time.DateOnly), p, err)
	}
	f.bank.mu.Lock()
	numerator := f.bank.boeAccruedNumerator
	f.bank.mu.Unlock()
	if p.Accrued.Num != numerator {
		t.Errorf("stored BoE numerator %d != state %d", p.Accrued.Num, numerator)
	}
}

// accrualPoundsE7 converts an accrual numerator (minor units over
// gbp.AccrualDenominator = 3,650,000) into 7dp pounds, rounding to
// nearest: pounds_e7 = n * 10^7 / 365,000,000 = 2n/73.
func accrualPoundsE7(n int64) int64 {
	if n < 0 {
		return -accrualPoundsE7(-n)
	}
	return (2*n + 36) / 73
}

func allCustomers(t *testing.T, f *fixture) []customers.Record {
	t.Helper()
	page, total, err := f.bank.Customers().Page(f.ctx, 1, CustomersPerPage)
	if err != nil || total != len(page) {
		t.Fatalf("allCustomers: %d of %d, %v", len(page), total, err)
	}
	return page
}

// The daily pass projects every account's position (mid-month and across
// a month-end application) so the ledger alone carries the balance and
// accrued-but-unapplied interest; BoE reserve interest is modelled in
// accounts: income recognised daily into a receivable, moved into
// Asset:BoEReserves at month end, with the sub-penny remainder carried in
// the numerator (posted pence == floor(numerator/denominator)).
func TestPositionsAndBoEInterestAreTheLedgers(t *testing.T) {
	f := open(t)
	if _, err := f.bank.OpenCustomer(f.ctx, ada); err != nil {
		t.Fatal(err)
	}
	if _, err := f.bank.OpenCustomer(f.ctx, saver("Saver", 50_000_00)); err != nil { // excess cash accrues BoE interest
		t.Fatal(err)
	}
	chart := f.bank.Ledger().Chart
	assertBoE := func(applied bool) {
		t.Helper()
		f.bank.mu.Lock()
		numerator, posted, appliedTotal := f.bank.boeAccruedNumerator, f.bank.boePostedPence, f.bank.boeInterestApplied
		f.bank.mu.Unlock()
		if posted != numerator/gbp.AccrualDenominator {
			t.Errorf("posted pence %d != floor(numerator) %d", posted, numerator/gbp.AccrualDenominator)
		}
		holding, _ := f.bank.Ledger().Balance(chart.AccruedBoE)
		if holding != luca.Amount(posted) {
			t.Errorf("%s balance %d != posted pence %d", ledger.AccruedBoE, holding, posted)
		}
		reserves, _ := f.bank.Ledger().Balance(chart.BoEReserves)
		if reserves != appliedTotal {
			t.Errorf("%s balance %d != applied %d", ledger.BoEReserves, reserves, appliedTotal)
		}
		income, _ := f.bank.Ledger().Balance(chart.IncomeBoE)
		if want := -(appliedTotal + luca.Amount(posted)); income != want {
			t.Errorf("%s balance %d != %d", ledger.IncomeBoE, income, want)
		}
		if applied && appliedTotal <= 0 {
			t.Error("no BoE interest applied after month end")
		}
		if p := f.position(); p.BoEInterest != appliedTotal+luca.Amount(posted) {
			t.Errorf("position BoE interest %d, want applied %d plus posted %d", p.BoEInterest, appliedTotal, posted)
		}
	}
	for range 5 { // mid-month: accrual only
		f.nextDay()
	}
	assertPositionsAreTheTruth(t, f)
	assertBoE(false)
	var applications int
	f.db.QueryRow(`SELECT COUNT(*) FROM contract_ledger_movements WHERE code = $1 AND to_path LIKE $2`, luca.CodeInterestAccrual, ledger.SavingsRoot+"%").Scan(&applications)
	if applications != 0 {
		t.Errorf("%d customer applications before month end", applications)
	}
	for range 30 { // crosses the 31 Jan month end
		f.nextDay()
	}
	assertPositionsAreTheTruth(t, f)
	assertBoE(true)
	f.db.QueryRow(`SELECT COUNT(*) FROM contract_ledger_movements WHERE code = $1 AND to_path LIKE $2`, luca.CodeInterestAccrual, ledger.SavingsRoot+"%").Scan(&applications)
	if applications == 0 {
		t.Error("no savings interest applied across the month end")
	}
	if rows, err := f.db.Query(`SELECT 1 FROM accrual_state`); err == nil {
		rows.Close()
		t.Error("accrual_state still exists; the products component owns no table")
	}
}

// The bank's interest to date on an accrual basis is what the ledger
// shows: the P&L accounts carry the applications, the positions carry the
// accrual not yet applied, and the read model's totals agree with the
// accounts summed one by one.
func TestProfitAndLossAndBookFollowTheLedger(t *testing.T) {
	f := open(t)
	for i := range 3 {
		if _, err := f.bank.OpenCustomer(f.ctx, ada); err != nil {
			t.Fatal(err)
		}
		if _, err := f.bank.OpenCustomer(f.ctx, saver("S", luca.Amount(1000_00*(i+1)))); err != nil {
			t.Fatal(err)
		}
	}
	for range 35 { // crosses the 31 Jan application
		f.nextDay()
	}
	if _, err := f.bank.Transfer(f.ctx, core.Transfer{From: core.CustomerID(2), To: core.CustomerID(4), Amount: 10_00}); err != nil {
		t.Fatal(err)
	}
	var savings, lending, appliedSavings, appliedLending luca.Amount
	var accruedSavingsE7, accruedLendingE7 int64
	for _, c := range allCustomers(t, f) {
		for _, a := range c.Accounts {
			if a.Family == gbp.FamilySavings {
				savings += a.Balance
				appliedSavings += a.Interest
				accruedSavingsE7 += a.AccruedE7
			} else {
				lending += a.Balance
				appliedLending += a.Interest
				accruedLendingE7 += a.AccruedE7
			}
		}
	}
	if appliedSavings <= 0 || accruedSavingsE7 <= 0 || lending <= 0 {
		t.Fatalf("savings applied %d, accrued %d e7, lending %d: the test needs all three", appliedSavings, accruedSavingsE7, lending)
	}
	p := f.position()
	if p.Savings != savings || p.Lending != lending || p.Customers != 6 {
		t.Errorf("position savings %d lending %d customers %d; want %d, %d, 6 summed over the accounts", p.Savings, p.Lending, p.Customers, savings, lending)
	}
	pl, err := f.bank.ProfitAndLoss(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := appliedSavings + luca.Amount(accruedSavingsE7/poundsE7PerPenny); pl.DepositInterestExpense != want {
		t.Errorf("deposit interest expense %d, want applied %d + accrued %d", pl.DepositInterestExpense, appliedSavings, accruedSavingsE7/poundsE7PerPenny)
	}
	if want := appliedLending + luca.Amount(accruedLendingE7/poundsE7PerPenny); pl.LoanInterestIncome != want {
		t.Errorf("loan interest income %d, want applied %d + accrued %d", pl.LoanInterestIncome, appliedLending, accruedLendingE7/poundsE7PerPenny)
	}
	if pl.DayCount != 35 || pl.OperatingCosts != opCostPerDay*35 {
		t.Errorf("P&L %+v; want 35 days of operating cost", pl)
	}
	bs, _ := f.bank.BalanceSheet(f.ctx)
	if bs.Deposits != savings || bs.Loans != lending || bs.RetainedEarnings != pl.NetProfit() {
		t.Errorf("balance sheet %+v", bs)
	}
	if books, _ := f.bank.Products(f.ctx); len(books) != 6 {
		t.Errorf("%d products", len(books))
	}
}

// A transfer moves the money between the two customers' first savings
// accounts on the ledger at once and records a payment that settles on
// its own; it is refused for a bad amount, the same customer, an unknown
// one or more than the sender holds.
func TestTransfer(t *testing.T) {
	f := open(t)
	for _, n := range []core.NewCustomer{saver("A", 100_00), saver("B", 5_00)} {
		if _, err := f.bank.OpenCustomer(f.ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	a, b := core.CustomerID(1), core.CustomerID(2)
	for _, c := range []struct {
		t    core.Transfer
		want error
	}{
		{core.Transfer{From: a, To: b, Amount: 0}, core.ErrInvalidAmount},
		{core.Transfer{From: a, To: a, Amount: 1}, core.ErrSameCustomer},
		{core.Transfer{From: a, To: "cust-999", Amount: 1}, core.ErrNotFound},
		{core.Transfer{From: b, To: a, Amount: 6_00}, core.ErrInsufficientFunds},
	} {
		if _, err := f.bank.Transfer(f.ctx, c.t); !errors.Is(err, c.want) {
			t.Errorf("Transfer(%+v): %v, want %v", c.t, err, c.want)
		}
	}
	pay, err := f.bank.Transfer(f.ctx, core.Transfer{From: a, To: b, Amount: 30_00})
	if err != nil {
		t.Fatal(err)
	}
	if pay.Status != core.PaymentPending || pay.Reference != "PAY-000003" || pay.From != a || pay.To != b {
		t.Errorf("payment %+v", pay)
	}
	accounts, _ := f.bank.Accounts(f.ctx, a)
	if accounts[0].Balance != 70_00 {
		t.Errorf("sender's balance %d, want 7000 at once", accounts[0].Balance)
	}
	if p := f.position(); p.Savings != 105_00 {
		t.Errorf("savings %d after a transfer, want unchanged 10500", p.Savings)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		got, _ := f.bank.Payment(f.ctx, pay.ID)
		if got.Status == core.PaymentCompleted && !got.SettledAt.IsZero() {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("payment did not settle: %+v", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if tp, _ := f.bank.Transactions(f.ctx, b, 1); tp.Total != 2 || tp.Entries[0].Type != "Transfer In" || tp.Entries[0].Reference != pay.Reference {
		t.Errorf("recipient's statement %+v", tp.Entries)
	}
}

// A bank opened again over the same database resumes the run: the same
// day, customers, book, BoE interest and series, with the numbering
// carrying on.
func TestReopeningResumesTheRun(t *testing.T) {
	f := open(t)
	for range 3 {
		if _, err := f.bank.OpenCustomer(f.ctx, ada); err != nil {
			t.Fatal(err)
		}
	}
	for range 40 { // crosses a month end
		f.nextDay()
	}
	before := f.position()
	beforePL, _ := f.bank.ProfitAndLoss(f.ctx)
	beforeHist, _ := f.bank.History(f.ctx)

	g := reopen(t, f.db, f.now)
	after := g.position()
	if !after.Day.Equal(before.Day) || after.DayCount != before.DayCount || after.Customers != before.Customers ||
		after.Savings != before.Savings || after.Lending != before.Lending || after.BoEInterest != before.BoEInterest || after.NIMBps != before.NIMBps || !after.DayComplete {
		t.Errorf("resumed position %+v, want %+v", after, before)
	}
	if pl, _ := g.bank.ProfitAndLoss(g.ctx); pl != beforePL {
		t.Errorf("resumed P&L %+v, want %+v", pl, beforePL)
	}
	if hist, _ := g.bank.History(g.ctx); len(hist.Balances) != len(beforeHist.Balances) {
		t.Errorf("resumed series %d points, want %d", len(hist.Balances), len(beforeHist.Balances))
	}
	rec, err := g.bank.OpenCustomer(g.ctx, saver("Fourth", 10_00))
	if err != nil || rec.ID != core.CustomerID(4) {
		t.Errorf("the customer after a restart is %s, %v; want cust-004", rec.ID, err)
	}
	if pay, err := g.bank.Transfer(g.ctx, core.Transfer{From: core.CustomerID(4), To: core.CustomerID(1), Amount: 1_00}); err != nil || pay.ID != 8 {
		t.Errorf("the payment after a restart is %d, %v; want 8 after three customers' six and the fourth's deposit", pay.ID, err)
	}
	g.nextDay()
	if p := g.position(); p.DayCount != 41 {
		t.Errorf("day count after a restart and a day %d, want 41", p.DayCount)
	}
}

// Reopened over a wiped database the bank is on its first day again,
// under the same handle.
func TestReopenOverAWipedDatabaseIsAFreshBank(t *testing.T) {
	f := open(t)
	if _, err := f.bank.OpenCustomer(f.ctx, ada); err != nil {
		t.Fatal(err)
	}
	f.nextDay()
	f.now = opening
	if err := f.bank.Reopen(openDB(t)); err != nil {
		t.Fatal(err)
	}
	p := f.position()
	if !p.Day.Equal(opening) || p.DayCount != 0 || p.Customers != 0 || p.Savings != 0 || p.BoEInterest != 0 {
		t.Errorf("position after reopening %+v; want a bank on its first day", p)
	}
	if h, _ := f.bank.History(f.ctx); len(h.Balances) != 1 {
		t.Errorf("%d days on record after reopening, want 1", len(h.Balances))
	}
	if pr := f.bank.Progress(); pr.Active || !pr.LastDay.IsZero() {
		t.Errorf("progress after reopening %+v; want nothing", pr)
	}
	if rec, _ := f.bank.OpenCustomer(f.ctx, saver("First again", 1)); rec.ID != core.CustomerID(1) {
		t.Errorf("first customer after reopening is %s", rec.ID)
	}
}

// The book is read through the views and served from a cache; a command
// that moves it invalidates the cache, so what follows a command is what
// the command did, and the cache is refreshed when it goes stale.
func TestBookIsAReadModel(t *testing.T) {
	f := open(t)
	if _, err := f.bank.OpenCustomer(f.ctx, saver("A", 10_00)); err != nil {
		t.Fatal(err)
	}
	if p := f.position(); p.Savings != 10_00 {
		t.Fatalf("savings %d right after opening", p.Savings)
	}
	// A movement behind the read model's back is not seen until the cache
	// goes stale.
	cust, _ := f.bank.Customers().ByID(f.ctx, core.CustomerID(1))
	if err := f.bank.Ledger().Post(f.bank.Ledger().Chart.EquityCapital, cust.Accounts[0].LedgerAccountID, 5_00, luca.CodeBookTransfer, opening, "behind"); err != nil {
		t.Fatal(err)
	}
	if p := f.position(); p.Savings != 10_00 {
		t.Errorf("savings %d from a fresh reading, want the read 1000", p.Savings)
	}
	// An expired reading is served one last time while a new one is taken
	// behind the page.
	f.bank.book.expire()
	if p := f.position(); p.Savings != 10_00 {
		t.Errorf("savings %d from an expired reading, want the last 1000 straight away", p.Savings)
	}
	eventually(t, 15_00, func() int { return int(f.position().Savings) })
}

// Export writes the ledger and Import reads it back, opening accounts as
// needed and re-reading the bank's position.
func TestExportAndImport(t *testing.T) {
	f := open(t)
	if _, err := f.bank.OpenCustomer(f.ctx, saver("A", 10_00)); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := f.bank.Export(&buf); err != nil || buf.Len() == 0 {
		t.Fatalf("Export: %d bytes, %v", buf.Len(), err)
	}
	// go-luca's import refuses a commodity already on the books, and every
	// bank has GBP from its chart: the commodity line is dropped.
	var movements bytes.Buffer
	for line := range strings.SplitSeq(buf.String(), "\n") {
		if !strings.HasPrefix(line, "commodity ") {
			movements.WriteString(line + "\n")
		}
	}
	g := reopen(t, openDB(t), opening)
	if err := g.bank.Import(&movements); err != nil {
		t.Fatal(err)
	}
	if bal, _, err := g.bank.Ledger().BalanceByPath(ledger.SavingsRoot, opening.AddDate(0, 0, 1)); err != nil || bal != 10_00 {
		t.Errorf("imported savings %d, %v; want 1000", bal, err)
	}
}

// The ledger's business day is the bank's: StartDay advances it before
// the pass, so the live positions read the day's two slices rather than
// each account's latest row (go-luca v0.5.0), and a restart on the same
// day leaves it where it is.
func TestStartDayAdvancesTheLedgersBusinessDay(t *testing.T) {
	f := open(t)
	if _, err := f.bank.OpenCustomer(f.ctx, ada); err != nil {
		t.Fatal(err)
	}
	if _, err := f.bank.StartDay(f.ctx); err != nil {
		t.Fatal(err)
	}
	bd, err := f.bank.Ledger().BusinessDay()
	if err != nil || bd == nil || !bd.Day.Equal(opening) {
		t.Fatalf("ledger business day after the opening day = %v, %v; want %s", bd, err, opening.Format(time.DateOnly))
	}
	f.nextDay()
	f.nextDay()
	bd, err = f.bank.Ledger().BusinessDay()
	want := opening.AddDate(0, 0, 2)
	if err != nil || bd == nil || !bd.Day.Equal(want) || !bd.PrevDay.Equal(opening.AddDate(0, 0, 1)) {
		t.Fatalf("ledger business day after two days = %v, %v; want %s with the day before as previous", bd, err, want.Format(time.DateOnly))
	}
	g := reopen(t, f.db, f.now)
	if _, err := g.bank.StartDay(g.ctx); err != nil {
		t.Fatal(err)
	}
	if bd, err := g.bank.Ledger().BusinessDay(); err != nil || !bd.Day.Equal(want) {
		t.Errorf("ledger business day after a restart = %v, %v; want %s unchanged", bd, err, want.Format(time.DateOnly))
	}
}
