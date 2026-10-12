package bank

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	"git.bytestone.uk/hum3/gobank/modelbank"
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

// reopen is the next process over db: a bank opened on it at wall time
// at, with the market's rate for the pound published at 5% (a replay of
// the same publication changes nothing) and the publication's cascade
// settled, so the bank is at rest.
func reopen(t *testing.T, db *sql.DB, at time.Time) *fixture {
	t.Helper()
	f := reopenSilent(t, db, at)
	if err := f.bank.Publish(f.ctx, core.Publication{Currency: "GBP", Bps: 500, At: opening}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.bank.StartDay(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f
}

// reopenSilent is reopen with the market silent: no rate published.
func reopenSilent(t *testing.T, db *sql.DB, at time.Time) *fixture {
	t.Helper()
	f := &fixture{t: t, db: db, now: at, ctx: context.Background(), ratio: DefaultReserveRatio}
	b, err := Open(db, Options{
		Definition: modelbank.Definition(),
		Clock:      core.ClockFunc(func() time.Time { return f.now }),
		Seed:       42,
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
	Accounts: []core.NewAccount{{ProductID: "easy-access", Opening: 1000_00}, {ProductID: "personal-loan", Opening: 2000_00}},
}

func saver(name string, amount luca.Amount) core.NewCustomer {
	return core.NewCustomer{PII: core.PII{Name: name}, Accounts: []core.NewAccount{{ProductID: "easy-access", Opening: amount}}}
}

// The bank opens with its first admin (story 1.7.1): the user "admin"
// signs in with the deployment's password, which follows the deployment
// when the bank is reopened on it, and nobody signs in to a bank opened
// without one.
func TestBankOpensWithItsFirstAdmin(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	b, err := Open(db, Options{Definition: modelbank.Definition(), AdminPassword: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	admin, err := b.AuthenticateUser(ctx, "admin", "s3cret")
	if err != nil || !admin.HasRole("admin") {
		t.Fatalf("AuthenticateUser(admin) = %+v, %v", admin, err)
	}
	if got, err := b.User(ctx, admin.ID); err != nil || got.Login != "admin" {
		t.Errorf("User(%s) = %+v, %v", admin.ID, got, err)
	}
	if b, err = Open(db, Options{Definition: modelbank.Definition(), AdminPassword: "rotated"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AuthenticateUser(ctx, "admin", "rotated"); err != nil {
		t.Errorf("after reopening with a new password: %v", err)
	}
	if _, err := b.AuthenticateUser(ctx, "admin", "s3cret"); !errors.Is(err, core.ErrBadCredentials) {
		t.Errorf("the old password still signs in: %v", err)
	}
	if b, err = Open(openDB(t), Options{Definition: modelbank.Definition()}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AuthenticateUser(ctx, "admin", ""); !errors.Is(err, core.ErrBadCredentials) {
		t.Errorf("a bank opened without an admin password lets admin in: %v", err)
	}
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
	if !p.Day.Equal(opening) || p.DayCount != 0 || p.Customers != 0 || p.Savings != 0 || p.Lending != 0 || !p.DayComplete || p.RiskFreeBps != 500 {
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
	if len(rec.Accounts) != 2 || rec.Accounts[0].Balance != -deposit || rec.Accounts[1].Balance != wantLoan {
		t.Fatalf("accounts %+v; want the deposit as a liability %d and the loan trimmed to %d", rec.Accounts, -deposit, wantLoan)
	}
	cust, _ := f.bank.Customers().ByID(f.ctx, rec.ID)
	for _, a := range cust.Accounts {
		pos, err := f.bank.Ledger().PositionAt(a.LedgerAccountID, opening)
		if err != nil || pos == nil {
			t.Fatalf("%s: no position on the opening day: %v %v", a.ProductName, pos, err)
		}
		if pos.Balance != a.Balance || pos.Accrued.Num != int64(a.Balance)*a.RateBps {
			t.Errorf("%s: position %+v; want balance %d accruing a day at %d bps", a.ProductName, pos, a.Balance, a.RateBps)
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
	none, err := f.bank.OpenCustomer(f.ctx, core.NewCustomer{PII: core.PII{Name: "No Headroom"}, Accounts: []core.NewAccount{{ProductID: "mortgage", Opening: 50_000_00}}})
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
	// Savings interest is applied to the saver: the account is the movement's to.
	_ = f.db.QueryRow(`SELECT COUNT(*) FROM contract_ledger_movements WHERE code = $1 AND to_path LIKE $2`, luca.CodeInterestAccrual, ledger.SavingsRoot+"%").Scan(&applications)
	if applications != 0 {
		t.Errorf("%d customer applications before month end", applications)
	}
	for range 30 { // crosses the 31 Jan month end
		f.nextDay()
	}
	assertPositionsAreTheTruth(t, f)
	assertBoE(true)
	_ = f.db.QueryRow(`SELECT COUNT(*) FROM contract_ledger_movements WHERE code = $1 AND to_path LIKE $2`, luca.CodeInterestAccrual, ledger.SavingsRoot+"%").Scan(&applications)
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
	// Savings figures are liabilities and negative in the bank's view
	// (ADR-0008); the totals compared are magnitudes.
	for _, c := range allCustomers(t, f) {
		for _, a := range c.Accounts {
			if a.Family == gbp.FamilySavings {
				savings -= a.Balance
				appliedSavings -= a.Interest
				accruedSavingsE7 -= a.AccruedE7
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
	if pay.Status != core.PaymentCompleted || pay.SettledAt.IsZero() || pay.Reference != "PAY-000003" || pay.From != a || pay.To != b {
		t.Errorf("payment %+v; want completed at once: the posting is the settlement (ADR-0010)", pay)
	}
	accounts, _ := f.bank.Accounts(f.ctx, a)
	if accounts[0].Balance != -70_00 {
		t.Errorf("sender's balance %d, want -7000 at once (a liability, ADR-0008)", accounts[0].Balance)
	}
	if p := f.position(); p.Savings != 105_00 {
		t.Errorf("savings %d after a transfer, want unchanged 10500", p.Savings)
	}
	if got, _ := f.bank.Payment(f.ctx, pay.ID); got.Status != core.PaymentCompleted || !got.SettledAt.Equal(f.now) {
		t.Errorf("payment on record %+v, want completed at the clock's instant", got)
	}
	if owed, _ := f.bank.Owed(f.ctx); !owed.AtRest() {
		t.Errorf("after a transfer %+v is owed, want at rest", owed)
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
	if err := f.bank.Ledger().Post(cust.Accounts[0].LedgerAccountID, f.bank.Ledger().Chart.Cash, 5_00, luca.CodeBookTransfer, opening, "behind"); err != nil {
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
	if bal, _, err := g.bank.Ledger().BalanceByPath(ledger.SavingsRoot, opening.AddDate(0, 0, 1)); err != nil || bal != -10_00 {
		t.Errorf("imported savings %d, %v; want -1000, the liability", bal, err)
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

// The products are the catalogue with each product's share of the book:
// the accounts open on it and their balance, agreeing with the position
// by family.
func TestProductsAreTheBookPerProduct(t *testing.T) {
	f := open(t)
	if _, err := f.bank.OpenCustomer(f.ctx, ada); err != nil {
		t.Fatal(err)
	}
	if _, err := f.bank.OpenCustomer(f.ctx, saver("Bob", 250_00)); err != nil {
		t.Fatal(err)
	}
	products, err := f.bank.Products(f.ctx)
	if err != nil || len(products) != 6 {
		t.Fatalf("Products = %d, %v; want the 6 of the catalogue", len(products), err)
	}
	pos := f.position() // the loan is what the headroom allowed
	want := map[string]struct {
		accounts int
		balance  luca.Amount
	}{"easy-access": {2, -1250_00}, "personal-loan": {1, pos.Lending}} // a product's balance is the bank's view: savings negative
	var savings, lending luca.Amount
	for _, p := range products {
		w := want[p.ID]
		if p.Accounts != w.accounts || p.Balance != w.balance {
			t.Errorf("%s: %d accounts, balance %d; want %d, %d", p.ID, p.Accounts, p.Balance, w.accounts, w.balance)
		}
		if p.Family == string(gbp.FamilyLending) {
			lending += p.Balance
		} else {
			savings -= p.Balance
		}
	}
	if savings != pos.Savings || lending != pos.Lending {
		t.Errorf("products sum to %d savings, %d lending; the position is %d, %d", savings, lending, pos.Savings, pos.Lending)
	}
}

// The ledger is the bank's view (ADR-0008): debit-normal accounts (assets,
// expenses) positive, credit-normal (liabilities, equity, income) negative,
// a movement a signed amount from the counterparty to the account, the
// account's change. A deposit is negative from cash to the saver; a
// drawdown positive from cash to the loan; savings interest negative from
// the expense to the saver; loan interest positive from income to the
// loan; equity is no counterparty of customer flows.
func TestLedgerIsTheBanksView(t *testing.T) {
	f := open(t)
	if _, err := f.bank.OpenCustomer(f.ctx, ada); err != nil {
		t.Fatal(err)
	}
	books := f.bank.Ledger()
	bal := func(path string) luca.Amount {
		id, err := books.Account(path)
		if err != nil {
			t.Fatal(err)
		}
		b, err := books.Balance(id)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	cust, _ := f.bank.Customers().ByID(f.ctx, core.CustomerID(1))
	savings, loan := cust.Accounts[0], cust.Accounts[1]
	const deposit = luca.Amount(1000_00)
	drawn := loan.Balance
	if savings.Balance != -deposit {
		t.Errorf("savings balance %d, want %d: a liability is negative", savings.Balance, -deposit)
	}
	if drawn <= 0 || drawn > deposit {
		t.Fatalf("loan balance %d, want positive and within the headroom", drawn)
	}
	if got := bal(ledger.Cash); got != deposit-drawn {
		t.Errorf("Asset:Cash %d, want the deposit less the drawdown %d", got, deposit-drawn)
	}
	if got := bal(ledger.EquityCapital); got != 0 {
		t.Errorf("Equity:Capital %d, want untouched by customer flows", got)
	}
	// A month on, interest has been applied both ways.
	for range 32 {
		f.nextDay()
	}
	cust, _ = f.bank.Customers().ByID(f.ctx, core.CustomerID(1))
	savings, loan = cust.Accounts[0], cust.Accounts[1]
	if savings.Interest >= 0 || savings.Balance != -deposit+savings.Interest {
		t.Errorf("savings after a month: balance %d, interest %d; want interest negative and in the balance", savings.Balance, savings.Interest)
	}
	if loan.Interest <= 0 || loan.Balance != drawn+loan.Interest {
		t.Errorf("loan after a month: balance %d, interest %d; want interest positive and in the balance", loan.Balance, loan.Interest)
	}
	if got := bal(ledger.ExpenseInterest); got != -savings.Interest {
		t.Errorf("Expense:Interest %d, want the savings interest as a positive expense %d", got, -savings.Interest)
	}
	if got := bal(ledger.IncomeInterest); got != -loan.Interest {
		t.Errorf("Income:Interest %d, want the loan interest as a negative income %d", got, -loan.Interest)
	}
	// The book and the position read magnitudes from the signed ledger.
	pos := f.position()
	if pos.Savings != -savings.Balance || pos.Lending != loan.Balance || pos.Cash != bal(ledger.Cash) {
		t.Errorf("position %+v; want savings %d, lending %d, cash %d", pos, -savings.Balance, loan.Balance, bal(ledger.Cash))
	}
	pl, _ := f.bank.ProfitAndLoss(f.ctx)
	if pl.DepositInterestExpense <= 0 || pl.LoanInterestIncome <= 0 {
		t.Errorf("P&L %+v; want both interest figures positive magnitudes", pl)
	}
}

// A movement is read as written: a signed amount from one account to
// another, each with the balance it was left with, in the bank's view
// (ADR-0008). The statement line that shows it from one side carries its
// ID.
func TestMovementIsTheJournalEntry(t *testing.T) {
	f := open(t)
	if _, err := f.bank.OpenCustomer(f.ctx, saver("A", 10_00)); err != nil {
		t.Fatal(err)
	}
	page, _ := f.bank.Transactions(f.ctx, core.CustomerID(1), 1)
	if len(page.Entries) != 1 || page.Entries[0].Type != "Deposit" || page.Entries[0].Family != "Savings" {
		t.Fatalf("statement %+v; want the deposit on a savings account", page.Entries)
	}
	m, err := f.bank.Movement(f.ctx, page.Entries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != page.Entries[0].ID || m.Amount != -10_00 || m.Currency != "GBP" || m.Code != luca.CodeBookTransfer || m.Description != page.Entries[0].Reference {
		t.Errorf("movement %+v; want the deposit as -1000 from cash to the saver", m)
	}
	if m.From.Type != "Asset" || m.From.Path != ledger.Cash || m.From.Balance != 10_00 {
		t.Errorf("from leg %+v; want cash at 1000", m.From)
	}
	if m.To.Type != "Liability" || !strings.HasPrefix(m.To.Path, ledger.SavingsRoot+":"+core.CustomerID(1)) || m.To.Balance != -10_00 {
		t.Errorf("to leg %+v; want the saver's account at -1000", m.To)
	}
	if !m.ValueTime.Equal(opening) || m.KnowledgeTime.IsZero() {
		t.Errorf("movement times %v, %v; want valued on the opening day and known", m.ValueTime, m.KnowledgeTime)
	}
	if _, err := f.bank.Movement(f.ctx, "no-such-movement"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("unknown movement: %v, want ErrNotFound", err)
	}
}

// Without a publication for the pound the bank cannot start a day
// (ADR-0009): the reserve accrual has no rate, so the day is refused with
// nothing changed, and starts once the market has spoken. A publication at
// noon counts for its whole day.
func TestTheDayCannotStartWhileTheMarketIsSilent(t *testing.T) {
	f := reopenSilent(t, openDB(t), opening)
	if p := f.position(); p.RiskFreeBps != 0 {
		t.Errorf("a silent market has a rate: %+v", p)
	}
	f.now = f.now.AddDate(0, 0, 1)
	day, err := f.bank.StartDay(f.ctx)
	if err == nil || !strings.Contains(err.Error(), "Bank of England") || !day.Equal(opening) {
		t.Fatalf("StartDay with the market silent: %v, %v; want a refusal naming the market, still on the opening day", day, err)
	}
	if p := f.position(); !p.Day.Equal(opening) || p.DayCount != 0 {
		t.Errorf("the refused day moved the bank: %+v", p)
	}
	noon := opening.Add(12 * time.Hour)
	if err := f.bank.Publish(f.ctx, core.Publication{Currency: "GBP", Bps: 475, At: noon}); err != nil {
		t.Fatal(err)
	}
	if p := f.position(); p.RiskFreeBps != 475 {
		t.Errorf("after the publication the position shows %d, want 475", p.RiskFreeBps)
	}
	if _, err := f.bank.StartDay(f.ctx); err != nil {
		t.Fatalf("StartDay after the publication: %v", err)
	}
	if p := f.position(); p.DayCount != 1 || p.RiskFreeBps != 475 {
		t.Errorf("after the day started: %+v", p)
	}
}

// The bank reports what it owes for the instant its clock is at
// (ADR-0010): days behind the clock, accounts unprojected, parameter
// changes unfired, payments in flight. A market publication the clock
// reaches makes every account owed a ParameterChange, which StartDay
// fires, and the bank is at rest again; the clock moving on a day owes
// the day until StartDay runs it.
func TestOwedIsCountsAndStartDayClearsThem(t *testing.T) {
	f := open(t)
	for _, n := range []core.NewCustomer{ada, saver("S", 50_00)} {
		if _, err := f.bank.OpenCustomer(f.ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	if owed, err := f.bank.Owed(f.ctx); err != nil || !owed.AtRest() {
		t.Fatalf("a fresh bank with customers owes %+v, %v; want at rest", owed, err)
	}
	noon := opening.Add(12 * time.Hour)
	if err := f.bank.Publish(f.ctx, core.Publication{Currency: "GBP", Bps: 525, At: noon}); err != nil {
		t.Fatal(err)
	}
	if owed, _ := f.bank.Owed(f.ctx); owed.Events != 0 {
		t.Errorf("before noon %d events are owed, want 0: the publication is ahead of the clock", owed.Events)
	}
	f.now = noon
	owed, _ := f.bank.Owed(f.ctx)
	if owed.Events != 1 || owed.Days != 0 || owed.Unprojected != 0 {
		t.Errorf("at noon %+v is owed, want the publication alone", owed)
	}
	if _, err := f.bank.StartDay(f.ctx); err != nil {
		t.Fatal(err)
	}
	if owed, _ := f.bank.Owed(f.ctx); !owed.AtRest() {
		t.Errorf("after StartDay at noon %+v is owed, want at rest", owed)
	}
	f.now = opening.AddDate(0, 0, 2)
	if owed, _ := f.bank.Owed(f.ctx); owed.Days != 2 {
		t.Errorf("with the clock two days on %+v is owed, want 2 days", owed)
	}
	if _, err := f.bank.StartDay(f.ctx); err != nil {
		t.Fatal(err)
	}
	if owed, _ := f.bank.Owed(f.ctx); !owed.AtRest() {
		t.Errorf("after the days %+v is owed, want at rest", owed)
	}
}

// A staff member's product setting is upcoming until its instant, then
// owed to the product's accounts and in force; the command refuses what
// is not the product's to set.
func TestSetParameterIsUpcomingThenInForce(t *testing.T) {
	f := open(t)
	if _, err := f.bank.OpenCustomer(f.ctx, ada); err != nil {
		t.Fatal(err)
	}
	later := opening.Add(36 * time.Hour)
	if err := f.bank.SetParameter(f.ctx, core.ParameterSetting{ProductID: "easy-access", Key: "rate_bps", Value: "175", Effective: later, By: "alice"}); err != nil {
		t.Fatal(err)
	}
	up, err := f.bank.UpcomingSettings(f.ctx)
	if err != nil || len(up) != 1 || up[0].Owner != "easy-access" || up[0].Key != "rate_bps" || up[0].Value != "175" || !up[0].Effective.Equal(later) || !up[0].Decided.Equal(f.now) || up[0].By != "alice" || up[0].Scope != "version" {
		t.Errorf("UpcomingSettings = %+v, %v; want the one setting, decided at the clock's instant", up, err)
	}
	if owed, _ := f.bank.Owed(f.ctx); owed.Events != 0 {
		t.Errorf("before the instant %+v is owed", owed)
	}
	f.now = later
	if owed, _ := f.bank.Owed(f.ctx); owed.Events != 1 || owed.Days != 1 {
		t.Errorf("at the instant %+v is owed, want the setting's event and the day", owed)
	}
	if _, err := f.bank.StartDay(f.ctx); err != nil {
		t.Fatal(err)
	}
	if owed, _ := f.bank.Owed(f.ctx); !owed.AtRest() {
		t.Errorf("after StartDay %+v is owed", owed)
	}
	if up, _ := f.bank.UpcomingSettings(f.ctx); len(up) != 0 {
		t.Errorf("in force but still upcoming: %+v", up)
	}
	for _, bad := range []core.ParameterSetting{
		{ProductID: "easy-access", Key: "no-such", Value: "1", Effective: later.Add(time.Hour), By: "alice"},
		{ProductID: "easy-access", Key: "rate_bps", Value: "x", Effective: later.Add(time.Hour), By: "alice"},
		{ProductID: "easy-access", Key: "rate_bps", Value: "1", Effective: later.Add(-time.Hour), By: "alice"},
		{ProductID: "easy-access", Key: "rate_bps", Value: "1", Effective: later.Add(time.Hour)},
	} {
		if err := f.bank.SetParameter(f.ctx, bad); !errors.Is(err, core.ErrInvalidSetting) {
			t.Errorf("SetParameter(%+v) = %v, want ErrInvalidSetting", bad, err)
		}
	}
	if err := f.bank.SetParameter(f.ctx, core.ParameterSetting{ProductID: "nope", Key: "rate_bps", Value: "1", Effective: later.Add(time.Hour), By: "alice"}); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("unknown product: %v, want ErrNotFound", err)
	}
}

// The markets the bank knows: the pound's, with the rate in force at the
// clock's instant and the next decision day on its calendar.
func TestMarketsNameTheNextDecision(t *testing.T) {
	f := open(t)
	f.now = time.Date(2020, 3, 5, 9, 0, 0, 0, time.UTC)
	markets, err := f.bank.Markets(f.ctx)
	if err != nil || len(markets) != 1 {
		t.Fatalf("Markets = %+v, %v", markets, err)
	}
	m := markets[0]
	london, _ := time.LoadLocation("Europe/London")
	if m.Currency != "GBP" || m.Name != "Bank of England" || m.RateBps != 500 || !m.NextDecision.Equal(time.Date(2020, 3, 11, 12, 0, 0, 0, london)) {
		t.Errorf("GBP market = %+v; want 500 bps in force and the special meeting of 11 Mar 2020 next", m)
	}
}

// Every command is a root event on record and what it writes names it
// as the cause (ADR-0011): an opening and a payment are done when they
// commit, with their payments and settings under them; a publication and
// a staff setting are pending until the clock reaches them and StartDay
// settles their cascade; the day's root is done when its pass is, and a
// rule's posting names it. The console reads the cascades by root.
func TestCommandsWriteRootsAndTheirEffectsCarryTheCause(t *testing.T) {
	f := open(t)
	fixed := core.NewCustomer{PII: core.PII{Name: "F"}, Accounts: []core.NewAccount{{ProductID: "fixed-term", Opening: 500_00}, {ProductID: "easy-access", Opening: 100_00}}}
	for _, n := range []core.NewCustomer{saver("S", 1000_00), fixed} {
		if _, err := f.bank.OpenCustomer(f.ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.bank.Transfer(f.ctx, core.Transfer{From: "cust-001", To: "cust-002", Amount: 10_00}); err != nil {
		t.Fatal(err)
	}
	noon := opening.Add(12 * time.Hour)
	if err := f.bank.Publish(f.ctx, core.Publication{Currency: "GBP", Bps: 525, At: noon}); err != nil {
		t.Fatal(err)
	}
	if err := f.bank.Publish(f.ctx, core.Publication{Currency: "GBP", Bps: 525, At: noon.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	later := opening.Add(36 * time.Hour)
	if err := f.bank.SetParameter(f.ctx, core.ParameterSetting{ProductID: "easy-access", Key: "rate_bps", Value: "175", Effective: later, By: "alice"}); err != nil {
		t.Fatal(err)
	}
	byKind := func() map[string][]core.Cascade {
		t.Helper()
		cs, err := f.bank.Cascades(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string][]core.Cascade{}
		for _, c := range cs {
			out[c.Event.Kind] = append(out[c.Event.Kind], c)
		}
		return out
	}
	cs := byKind()
	if n := len(cs["opening"]); n != 2 {
		t.Fatalf("%d opening roots, want 2: %+v", n, cs)
	}
	for _, c := range cs["opening"] {
		if c.Event.Status != "done" || c.Payments == 0 || c.Event.DoneAt.IsZero() {
			t.Errorf("opening %+v; want done with its funding payments under it", c)
		}
	}
	if got := cs["opening"][0].Settings + cs["opening"][1].Settings; got == 0 {
		t.Error("the fixed term's open wrote no setting under its opening")
	}
	if n := len(cs["payment"]); n != 1 || cs["payment"][0].Event.Status != "done" || cs["payment"][0].Payments != 1 || cs["payment"][0].Event.By != "cust-001" {
		t.Errorf("payment roots %+v; want one, done, by cust-001, with its payment", cs["payment"])
	}
	// The fixture's opening publication is done; noon's is pending; the
	// one that changed nothing raised nothing.
	if n := len(cs["publication"]); n != 2 || cs["publication"][0].Event.Status != "pending" || cs["publication"][0].Settings != 1 || !cs["publication"][0].Event.At.Equal(noon) || cs["publication"][1].Event.Status != "done" {
		t.Errorf("publication roots %+v; want noon's pending with its setting, the opening's done, and no third", cs["publication"])
	}
	if n := len(cs["setting"]); n != 1 || cs["setting"][0].Event.Status != "pending" || cs["setting"][0].Settings != 1 || !cs["setting"][0].Event.At.Equal(later) || cs["setting"][0].Event.By != "alice" {
		t.Errorf("setting roots %+v; want one at the effective instant, pending, by alice, with its setting", cs["setting"])
	}
	pays, _ := f.bank.PaymentsOf(f.ctx, "cust-001")
	if len(pays) == 0 {
		t.Fatal("no payments for cust-001")
	}

	// The clock reaches noon: the publication is settled; the setting
	// at 36h the day after.
	f.now = noon
	if _, err := f.bank.StartDay(f.ctx); err != nil {
		t.Fatal(err)
	}
	cs = byKind()
	if cs["publication"][0].Event.Status != "done" || cs["setting"][0].Event.Status != "pending" {
		t.Errorf("after noon: publication %s, setting %s; want done and pending", cs["publication"][0].Event.Status, cs["setting"][0].Event.Status)
	}
	f.now = later
	if _, err := f.bank.StartDay(f.ctx); err != nil {
		t.Fatal(err)
	}
	cs = byKind()
	if cs["setting"][0].Event.Status != "done" || len(cs["start of day"]) != 1 || cs["start of day"][0].Event.Status != "done" {
		t.Errorf("after the setting's instant: %+v; want the setting and the day done", cs)
	}

	// A month on, the cycle end's application names the day's root.
	f.now = opening.AddDate(0, 1, 1)
	if _, err := f.bank.StartDay(f.ctx); err != nil {
		t.Fatal(err)
	}
	var movementID string
	if err := f.db.QueryRow(`SELECT id FROM contract_ledger_movements WHERE code = $1 AND from_path = $2 ORDER BY value_time LIMIT 1`, luca.CodeInterestAccrual, ledger.ExpenseInterest).Scan(&movementID); err != nil {
		t.Fatalf("no savings application after a month: %v", err)
	}
	m, err := f.bank.Movement(f.ctx, movementID)
	if err != nil || m.Cause == nil || m.Cause.Kind != "start of day" || m.Cause.Status != "done" || m.Step != "apply" {
		t.Errorf("the application's movement %+v, %v; want its cause the day's root, done, step apply", m, err)
	}
	if !m.Cause.At.Equal(opening.AddDate(0, 1, 0)) {
		t.Errorf("the application's cause is the day %s, want 1 Feb", m.Cause.At)
	}
	funding, _ := f.bank.Movement(f.ctx, fundingMovement(t, f))
	if funding.Cause != nil || funding.Step != "" {
		t.Errorf("a funding movement, not a rule's posting, names a cause: %+v", funding)
	}
	if owed, _ := f.bank.Owed(f.ctx); !owed.AtRest() {
		t.Errorf("at the end %+v is owed", owed)
	}
}

func fundingMovement(t *testing.T, f *fixture) string {
	t.Helper()
	var id string
	if err := f.db.QueryRow(`SELECT id FROM contract_ledger_movements WHERE code = $1 ORDER BY value_time LIMIT 1`, luca.CodeBookTransfer).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// A command whose transaction fails leaves nothing: no root, no payment,
// no movement; the caller sees the error and may try again, and the
// retry is a root of its own (ADR-0011: a payment's cascade is one
// transaction, so its root is written with its effects).
func TestACommandThatFailsLeavesNothing(t *testing.T) {
	f := open(t)
	for _, n := range []core.NewCustomer{saver("A", 100_00), saver("B", 100_00)} {
		if _, err := f.bank.OpenCustomer(f.ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	before := facts(t, f.db)
	f.bank.Catalogue().SetEffectHook(func(account, effect string) error {
		if effect == "position" {
			return errors.New("killed")
		}
		return nil
	})
	if _, err := f.bank.Transfer(f.ctx, core.Transfer{From: "cust-001", To: "cust-002", Amount: 10_00}); err == nil || !strings.Contains(err.Error(), "killed") {
		t.Fatalf("a killed transfer: %v; want the kill", err)
	}
	if _, err := f.bank.OpenCustomer(f.ctx, saver("C", 100_00)); err == nil || !strings.Contains(err.Error(), "killed") {
		t.Fatalf("a killed opening: %v; want the kill", err)
	}
	if after := facts(t, f.db); after != before {
		t.Errorf("a failed command left facts:\n%s", diff(before, after))
	}
	f.bank.Catalogue().SetEffectHook(nil)
	pay, err := f.bank.Transfer(f.ctx, core.Transfer{From: "cust-001", To: "cust-002", Amount: 10_00})
	if err != nil {
		t.Fatal(err)
	}
	cs, _ := f.bank.Cascades(f.ctx)
	if len(cs) != 4 || cs[0].Event.Kind != "payment" || cs[0].Event.Status != "done" || cs[0].Payments != 1 {
		t.Errorf("after the retry the cascades are %+v; want the payment's root newest, done, with %s under it, and no root for what failed", cs, pay.Reference)
	}
}

// Every effect is proven by a fact written in its transaction, so a
// process killed after any effect, and the root replayed, leaves the
// same facts as a run never interrupted (ADR-0011). The scenario — a
// week of passes over a month end, a staff setting and a publication —
// runs once uninterrupted to count its effects and record its facts,
// then once per effect with the hook failing that effect's transaction;
// StartDay replays what the kill left pending, and the facts must match.
func TestKillAfterAnyEffectAndReplayLeavesTheSameFacts(t *testing.T) {
	jan25 := time.Date(2020, 1, 25, 0, 0, 0, 0, time.UTC)
	feb1 := time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC)
	scenario := func(t *testing.T, failAt int) (string, int) {
		t.Helper()
		db := openDB(t)
		f := reopenSilent(t, db, jan25)
		if err := f.bank.Publish(f.ctx, core.Publication{Currency: "GBP", Bps: 500, At: jan25}); err != nil {
			t.Fatal(err)
		}
		for _, n := range []core.NewCustomer{saver("A", 1000_00), saver("B", 2500_00), ada} {
			if _, err := f.bank.OpenCustomer(f.ctx, n); err != nil {
				t.Fatal(err)
			}
		}
		calls := 0
		f.bank.Catalogue().SetEffectHook(func(account, effect string) error {
			calls++
			if calls == failAt {
				return errors.New("killed")
			}
			return nil
		})
		if err := f.bank.SetParameter(f.ctx, core.ParameterSetting{ProductID: "easy-access", Key: "rate_bps", Value: "175", Effective: feb1.Add(9 * time.Hour), By: "alice"}); err != nil {
			t.Fatal(err)
		}
		for day := jan25.AddDate(0, 0, 1); !day.After(feb1.AddDate(0, 0, 1)); day = day.AddDate(0, 0, 1) {
			f.now = day
			f.bank.StartDay(f.ctx) //nolint:errcheck — a killed account is logged and left for the replay
			if day.Equal(feb1) {
				noon := feb1.Add(12 * time.Hour)
				if err := f.bank.Publish(f.ctx, core.Publication{Currency: "GBP", Bps: 525, At: noon}); err != nil {
					t.Fatal(err)
				}
				f.now = noon
				f.bank.StartDay(f.ctx) //nolint:errcheck
			}
		}
		// The replay: a new process would do the same from the record.
		f.bank.Catalogue().SetEffectHook(nil)
		if _, err := f.bank.StartDay(f.ctx); err != nil {
			t.Fatal(err)
		}
		if owed, _ := f.bank.Owed(f.ctx); !owed.AtRest() {
			t.Errorf("after the replay %+v is owed", owed)
		}
		return facts(t, db), calls
	}
	want, effects := scenario(t, 0)
	if effects < 20 {
		t.Fatalf("the scenario has %d effects; too few to prove anything", effects)
	}
	for n := 1; n <= effects; n++ {
		if got, _ := scenario(t, n); got != want {
			t.Errorf("killed after effect %d, the facts differ:\n%s", n, diff(want, got))
		}
	}
}

// facts is everything the bank holds that a replay must leave the same,
// as text: positions, movements, rule postings, settings, payments,
// events and the instants accounts' parameters are applied to, without
// the IDs and wall times that differ between runs.
func facts(t *testing.T, db *sql.DB) string {
	t.Helper()
	var b strings.Builder
	for _, q := range []string{
		`SELECT full_path, day, balance, accrued FROM contract_ledger_eod_positions ORDER BY full_path, day`,
		`SELECT from_path, to_path, amount, code, value_time, description FROM contract_ledger_movements ORDER BY value_time, from_path, to_path, amount, description`,
		`SELECT a.full_path, p.event, p.step, p.parameters, p.input_balance, p.input_accrued_num FROM product_postings p JOIN accounts a ON a.id = p.account_id ORDER BY a.full_path, p.event, p.step, p.input_balance`,
		`SELECT s.scope, COALESCE(a.full_path, s.owner), s.key, s.value, s.effective_from, s.decided_by FROM parameter_settings s LEFT JOIN accounts a ON a.id = s.owner ORDER BY s.scope, 2, s.key, s.effective_from, s.value`,
		`SELECT type, from_id, to_id, amount, status FROM payments ORDER BY id`,
		`SELECT kind, at, status, raised_by FROM events ORDER BY at, kind, raised_by`,
		`SELECT a.full_path, ap.applied_to FROM product_account_parameters ap JOIN accounts a ON a.id = ap.account_id ORDER BY a.full_path`,
	} {
		rows, err := db.Query(q)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			for i, v := range vals {
				if bs, ok := v.([]byte); ok {
					vals[i] = string(bs)
				}
			}
			fmt.Fprintf(&b, "%v\n", vals)
		}
		rows.Close()
		b.WriteString("--\n")
	}
	return b.String()
}

// diff is the lines in one text and not the other.
func diff(want, got string) string {
	w, g := map[string]bool{}, map[string]bool{}
	for l := range strings.SplitSeq(want, "\n") {
		w[l] = true
	}
	for l := range strings.SplitSeq(got, "\n") {
		g[l] = true
	}
	var out []string
	for l := range strings.SplitSeq(want, "\n") {
		if !g[l] {
			out = append(out, "- "+l)
		}
	}
	for l := range strings.SplitSeq(got, "\n") {
		if !w[l] {
			out = append(out, "+ "+l)
		}
	}
	return strings.Join(out, "\n")
}
