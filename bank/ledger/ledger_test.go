package ledger

import (
	"context"
	"database/sql"
	"testing"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	_ "git.bytestone.uk/hum3/go-postgres"
	gbp "git.bytestone.uk/hum3/gobank-products"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func open(t *testing.T) *Ledger {
	t.Helper()
	l, err := Open(openDB(t))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

var today = time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)

// Opening the ledger resolves the chart of accounts: every account the
// bank posts against exists at its path, and opening the same database
// again finds the same accounts.
func TestOpenResolvesTheChartOnce(t *testing.T) {
	db := openDB(t)
	first, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	for path, id := range map[string]string{
		EquityCapital: first.Chart.EquityCapital, ExpenseInterest: first.Chart.ExpenseInterest,
		IncomeInterest: first.Chart.IncomeInterest, IncomeBoE: first.Chart.IncomeBoE,
		AccruedBoE: first.Chart.AccruedBoE, BoEReserves: first.Chart.BoEReserves,
	} {
		acct, err := first.GetAccountByID(id)
		if err != nil || acct == nil || acct.FullPath != path {
			t.Errorf("%s: resolved to %q, account %+v, %v", path, id, acct, err)
		}
	}
	second, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if second.Chart != first.Chart {
		t.Errorf("reopened chart %+v, want the same accounts %+v", second.Chart, first.Chart)
	}
}

// Account is the account at a path, opened on first use and found after.
func TestAccountOpensOnFirstUse(t *testing.T) {
	l := open(t)
	id, err := l.Account("Expense:Fees")
	if err != nil || id == "" {
		t.Fatalf("Account = %q, %v", id, err)
	}
	again, err := l.Account("Expense:Fees")
	if err != nil || again != id {
		t.Errorf("second Account = %q, %v; want %q", again, err, id)
	}
	if got, _ := l.Account(BoEReserves); got != l.Chart.BoEReserves {
		t.Errorf("Account(BoEReserves) = %q, want the chart's %q", got, l.Chart.BoEReserves)
	}
}

// A customer's account hangs under the chart's savings or loans root by
// the product's family, named by customer and product.
func TestCustomerAccountsHangUnderTheRoots(t *testing.T) {
	l := open(t)
	cases := []struct {
		family gbp.ProductFamily
		want   string
	}{
		{gbp.FamilySavings, SavingsRoot + ":cust-001:easy-access"},
		{gbp.FamilyLending, LoansRoot + ":cust-001:easy-access"},
	}
	for _, c := range cases {
		id, err := l.OpenCustomerAccount("cust-001", "easy-access", c.family)
		if err != nil {
			t.Fatal(err)
		}
		acct, err := l.GetAccountByID(id)
		if err != nil || acct == nil || acct.FullPath != c.want || acct.Commodity != Currency || acct.Exponent != Exponent {
			t.Errorf("%s account = %+v, %v; want %s in %s at %d", c.family, acct, err, c.want, Currency, Exponent)
		}
	}
}

// Post writes an event with projections: the movement is on the books
// and both accounts' positions for the day move at once.
func TestPostMovesBothPositionsAtOnce(t *testing.T) {
	l := open(t)
	acct, err := l.OpenCustomerAccount("cust-001", "easy-access", gbp.FamilySavings)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Post(l.Chart.EquityCapital, acct, 1000_00, luca.CodeBookTransfer, today, "PAY-000001"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		id   string
		want luca.Amount
	}{{acct, 1000_00}, {l.Chart.EquityCapital, -1000_00}} {
		p, err := l.PositionAt(c.id, today)
		if err != nil || p == nil || !p.Day.Equal(today) || p.Balance != c.want {
			t.Errorf("position %s = %+v, %v; want today at %d", c.id, p, err, c.want)
		}
	}
	if err := l.Post(l.Chart.EquityCapital, "", 1, luca.CodeBookTransfer, today, "no account"); err == nil {
		t.Error("posting to no account succeeded")
	}
}

// The live position is the latest projection plus the movements since,
// read exactly: the balance in minor units and the accrual at 7dp.
func TestLivePositionIsExact(t *testing.T) {
	l := open(t)
	acct, err := l.OpenCustomerAccount("cust-001", "easy-access", gbp.FamilySavings)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Post(l.Chart.EquityCapital, acct, 1001_27, luca.CodeBookTransfer, today, "PAY-000001"); err != nil {
		t.Fatal(err)
	}
	// 1001.27 at 1.5% for a day: 1001.27 * 0.015 / 365 = 0.0411480... pounds.
	if _, err := l.Project(acct, today, luca.Fraction{Num: 1001_27 * 150, Den: gbp.AccrualDenominator}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.RecordMovement(l.Chart.EquityCapital, acct, 50, luca.CodeBookTransfer, today.AddDate(0, 0, 1), "tomorrow's top-up"); err != nil {
		t.Fatal(err)
	}
	p, err := l.LivePosition(context.Background(), acct)
	if err != nil {
		t.Fatal(err)
	}
	if p.Balance != 1001_77 {
		t.Errorf("live balance %d, want 100177 (the projection plus the movement valued after its day)", p.Balance)
	}
	if p.AccruedE7 <= 0 || p.AccruedE7 > 0_4200000 {
		t.Errorf("live accrual %d e7, want about 0.0411 pounds", p.AccruedE7)
	}
	if _, err := l.LivePosition(context.Background(), "no-such-account"); err == nil {
		t.Error("an unknown account has a live position")
	}
}

// A NUMERIC rendered as text is read as an integer at a scale, exactly.
func TestParseScaled(t *testing.T) {
	cases := []struct {
		s     string
		scale int
		want  int64
		bad   bool
	}{
		{"1001.27", 2, 100127, false},
		{"-0.4109589", 7, -4109589, false},
		{"0", 7, 0, false},
		{"12", 2, 1200, false},
		{"1.2300000000", 2, 123, false},
		{"1.234", 2, 0, true},
		{"abc", 2, 0, true},
	}
	for _, c := range cases {
		got, err := parseScaled(c.s, c.scale)
		if (err != nil) != c.bad || got != c.want {
			t.Errorf("parseScaled(%q, %d) = %d, %v; want %d, bad %v", c.s, c.scale, got, err, c.want, c.bad)
		}
	}
}

// The lock on an account is held by whoever is rewriting its position;
// several are taken in one order, the same account twice is one lock, and
// a ledger bound to a transaction shares the locks.
func TestAccountLocksTakeTurns(t *testing.T) {
	l := open(t)
	unlock := l.Lock("b", "a", "b", "")
	done := make(chan struct{})
	go func() {
		tx, err := l.db.Begin()
		if err != nil {
			panic(err)
		}
		defer tx.Rollback()
		u := l.WithTx(tx).Lock("a", "c")
		u()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("a second holder got account a while the first held it")
	case <-time.After(50 * time.Millisecond):
	}
	unlock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("releasing the locks did not let the second holder in")
	}
}

// A ledger bound to a transaction keeps the chart and writes inside it:
// nothing is on the books until the transaction commits.
func TestWithTxKeepsTheChart(t *testing.T) {
	l := open(t)
	tx, err := l.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	bound := l.WithTx(tx)
	if bound.Chart != l.Chart {
		t.Errorf("bound chart %+v, want %+v", bound.Chart, l.Chart)
	}
	acct, err := bound.OpenCustomerAccount("cust-002", "isa", gbp.FamilySavings)
	if err != nil {
		t.Fatal(err)
	}
	if err := bound.Post(bound.Chart.EquityCapital, acct, 5_00, luca.CodeBookTransfer, today, "PAY-000002"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if a, err := l.GetAccount(SavingsRoot + ":cust-002:isa"); err != nil || a != nil {
		t.Errorf("account survived the rollback: %+v, %v", a, err)
	}
}
