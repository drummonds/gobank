package products

import (
	"context"
	"database/sql"
	"testing"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	_ "git.bytestone.uk/hum3/go-postgres"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
)

// open is the catalogue over a database with the ledger open and a
// register of accounts: the contract view the pass reads, over the test's
// own table. What is registered is the customers' business; the pass
// needs only the view.
func open(t *testing.T) (*Products, *ledger.Ledger) {
	t.Helper()
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	books, err := ledger.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE registered (ledger_account_id VARCHAR(64) PRIMARY KEY, product_id VARCHAR(50) NOT NULL)`,
		`CREATE VIEW contract_customer_accounts AS SELECT ledger_account_id, product_id FROM registered`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return New(db), books
}

// register opens a customer account on the ledger, funds it from equity on
// day and registers it for the pass.
func register(t *testing.T, p *Products, books *ledger.Ledger, product Product, customer string, amount luca.Amount, day time.Time) Account {
	t.Helper()
	id, err := books.OpenCustomerAccount(customer, product.ID, product.Family)
	if err != nil {
		t.Fatal(err)
	}
	if amount > 0 { // a movement alone: the day's position is the pass's to write
		if _, err := books.RecordMovement(books.Chart.EquityCapital, id, amount, luca.CodeBookTransfer, day, "funding"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.db.Exec(`INSERT INTO registered (ledger_account_id, product_id) VALUES ($1, $2)`, id, product.ID); err != nil {
		t.Fatal(err)
	}
	return Account{ID: id, Product: product}
}

var jan2 = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

// The catalogue is six GBP products, each with a family and a rate, found
// by ID.
func TestCatalogue(t *testing.T) {
	p, _ := open(t)
	if n := len(p.All()); n != 6 {
		t.Fatalf("%d products, want 6", n)
	}
	for _, prod := range p.All() {
		if prod.ID == "" || prod.Currency != "GBP" || prod.Rate <= 0 || (prod.Family != gbp.FamilySavings && prod.Family != gbp.FamilyLending) {
			t.Errorf("product %+v needs an ID, GBP, a positive rate and a family", prod)
		}
		if got, ok := p.ByID(prod.ID); !ok || got.ID != prod.ID {
			t.Errorf("ByID(%s) = %+v, %v", prod.ID, got, ok)
		}
	}
	if _, ok := p.ByID("no-such-product"); ok {
		t.Error("an unknown product was found")
	}
}

// A day's work on an account projects its position for the day: the
// balance as it stands and the day's interest on it, over yesterday's.
func TestAccountDayProjectsTheDay(t *testing.T) {
	p, books := open(t)
	easy, _ := p.ByID(gbp.EasyAccess().ID)
	a := register(t, p, books, easy, "cust-001", 1000_00, jan2)
	res, err := p.AccountDay(books, a, jan2)
	if err != nil {
		t.Fatal(err)
	}
	want := int64(1000_00) * gbp.RateBps(easy.Rate)
	if res.Family != gbp.FamilySavings || res.Applied != 0 || res.Accrued != want {
		t.Errorf("result %+v, want savings, nothing applied, accrued %d", res, want)
	}
	pos, err := books.PositionAt(a.ID, jan2)
	if err != nil || pos == nil || pos.Balance != 1000_00 || pos.Accrued.Num != want || pos.Accrued.Den != gbp.AccrualDenominator {
		t.Errorf("position %+v, %v; want 100000 accruing %d/%d", pos, err, want, gbp.AccrualDenominator)
	}
	// Again is the same answer: the day's work is idempotent.
	if _, err := p.AccountDay(books, a, jan2); err != nil {
		t.Fatal(err)
	}
	if again, _ := books.PositionAt(a.ID, jan2); again.Accrued.Num != want {
		t.Errorf("a second visit changed the accrual to %d", again.Accrued.Num)
	}
}

// When the product's cycle ended yesterday, the whole pence accrued are
// applied to the balance as a movement from the P&L account, valued at
// yesterday's last second, and yesterday's position keeps the remainder.
func TestAccountDayAppliesAtTheCycleEnd(t *testing.T) {
	p, books := open(t)
	easy, _ := p.ByID(gbp.EasyAccess().ID)
	jan31 := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
	a := register(t, p, books, easy, "cust-001", 1000_00, jan31)
	// A month's accrual on the 31 Jan position: 1000.00 at 1.5% for 30 days
	// is 123.3p, so 123p apply and 0.3p remain.
	accrued := luca.Fraction{Num: 1000_00 * gbp.RateBps(easy.Rate) * 30, Den: gbp.AccrualDenominator}
	if _, err := books.Project(a.ID, jan31, accrued); err != nil {
		t.Fatal(err)
	}
	feb1 := jan31.AddDate(0, 0, 1)
	res, err := p.AccountDay(books, a, feb1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 123 {
		t.Errorf("applied %d, want 123 pence", res.Applied)
	}
	var movements int
	if err := p.db.QueryRow(`SELECT COUNT(*) FROM contract_ledger_movements WHERE to_account_id = $1 AND code = $2 AND from_path = $3`,
		a.ID, luca.CodeInterestAccrual, ledger.ExpenseInterest).Scan(&movements); err != nil || movements != 1 {
		t.Errorf("application movements from %s: %d, %v; want 1", ledger.ExpenseInterest, movements, err)
	}
	closed, _ := books.PositionAt(a.ID, jan31)
	if closed.Accrued.Num/gbp.AccrualDenominator != 0 {
		t.Errorf("31 Jan still holds %d/%d, want under a penny", closed.Accrued.Num, closed.Accrued.Den)
	}
	today, _ := books.PositionAt(a.ID, feb1)
	if today.Balance != 1000_00+123 || today.Accrued.Num != closed.Accrued.Num+int64(today.Balance)*gbp.RateBps(easy.Rate) {
		t.Errorf("1 Feb position %+v; want the applied balance accruing a day over the remainder", today)
	}
}

// An event moves both positions for the day at once and runs the day's
// rules again on the accounts it touched, so today's accrual is on the
// balance after the event; an event counts no accrual of its own.
func TestPostEventRewritesTheDaysPositions(t *testing.T) {
	p, books := open(t)
	easy, _ := p.ByID(gbp.EasyAccess().ID)
	from := register(t, p, books, easy, "cust-001", 1000_00, jan2)
	to := register(t, p, books, easy, "cust-002", 500_00, jan2)
	for _, a := range []Account{from, to} {
		if _, err := p.AccountDay(books, a, jan2); err != nil {
			t.Fatal(err)
		}
	}
	results, err := p.PostEvent(books, jan2, from.ID, to.ID, 200_00, luca.CodeBookTransfer, "PAY-000001", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Accrued != 0 || results[1].Accrued != 0 {
		t.Errorf("results %+v; want one per account with no accrual counted", results)
	}
	for _, c := range []struct {
		a    Account
		want luca.Amount
	}{{from, 800_00}, {to, 700_00}} {
		pos, err := books.PositionAt(c.a.ID, jan2)
		if err != nil || pos == nil || pos.Balance != c.want || pos.Accrued.Num != int64(c.want)*gbp.RateBps(easy.Rate) {
			t.Errorf("%s: position %+v, %v; want %d accruing a day on it", c.a.ID, pos, err, c.want)
		}
	}
	if _, err := p.PostEvent(books, jan2, from.ID, "", 1, luca.CodeBookTransfer, "nowhere"); err == nil {
		t.Error("an event to no account was posted")
	}
}

type progress struct {
	phase string
	total int
	done  int
}

func (p *progress) Phase(name string, total int) { p.phase, p.total, p.done = name, total, 0 }
func (p *progress) Add(n int)                    { p.done += n }

// The pass visits every registered account without a position for the
// day, under its lock, and totals what the rules changed per family; cut
// short, it leaves the rest for the next pass over the same day.
func TestPassVisitsEveryUnprojectedAccountAndResumes(t *testing.T) {
	p, books := open(t)
	ctx := context.Background()
	easy, _ := p.ByID(gbp.EasyAccess().ID)
	loan, _ := p.ByID(gbp.PersonalLoan().ID)
	var accounts []Account
	for i := range 3 {
		accounts = append(accounts, register(t, p, books, easy, "cust-00"+string(rune('1'+i)), 1000_00, jan2))
	}
	accounts = append(accounts, register(t, p, books, loan, "cust-004", 2000_00, jan2))
	if pending, err := p.AnyUnprojected(ctx, jan2); err != nil || !pending {
		t.Fatalf("AnyUnprojected before the pass = %v, %v; want true", pending, err)
	}

	// Cut short after two accounts.
	ctx1, cancel := context.WithCancel(ctx)
	visited := 0
	var prog progress
	res := p.RunPass(ctx1, books, jan2, &prog, func() {
		if visited++; visited == 2 {
			cancel()
		}
	})
	if res.Visited != 2 || prog.phase != "projecting positions" || prog.total != 4 || prog.done != 2 {
		t.Fatalf("cut-short pass: result %+v, progress %+v; want 2 of 4 visited", res, prog)
	}
	if n, _ := p.CountUnprojected(ctx, jan2); n != 2 {
		t.Fatalf("%d accounts unprojected after the cut-short pass, want 2", n)
	}

	// The next pass over the same day finishes it.
	rest := p.RunPass(ctx, books, jan2, &prog, nil)
	if rest.Visited != 2 {
		t.Errorf("resumed pass visited %d, want the 2 left", rest.Visited)
	}
	if pending, _ := p.AnyUnprojected(ctx, jan2); pending {
		t.Error("accounts still unprojected after the resumed pass")
	}
	wantSavings := 3 * int64(1000_00) * gbp.RateBps(easy.Rate)
	wantLending := int64(2000_00) * gbp.RateBps(loan.Rate)
	if got := res.AccruedSavings + rest.AccruedSavings; got != wantSavings {
		t.Errorf("savings accrued over both passes %d, want %d", got, wantSavings)
	}
	if got := res.AccruedLending + rest.AccruedLending; got != wantLending {
		t.Errorf("lending accrued over both passes %d, want %d", got, wantLending)
	}
	// Nothing left: a further pass visits nobody.
	if again := p.RunPass(ctx, books, jan2, &prog, nil); again.Visited != 0 {
		t.Errorf("a pass over a finished day visited %d", again.Visited)
	}
}

// The schema is the component's own: versions in order from 1.
func TestSchemaVersionsRunFromOne(t *testing.T) {
	if Schema.Name != "products" {
		t.Errorf("Schema.Name = %q", Schema.Name)
	}
	for i, m := range Schema.Migrations {
		if m.Version != i+1 {
			t.Errorf("migration %d listed where version %d is due", m.Version, i+1)
		}
	}
}
