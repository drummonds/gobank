package products

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	_ "git.bytestone.uk/hum3/go-postgres"
	gbp "git.bytestone.uk/hum3/gobank-products"
	easyaccess "git.bytestone.uk/hum3/gobank-products/easyaccess/v1"
	fixedterm "git.bytestone.uk/hum3/gobank-products/fixedterm/v1"
	isa "git.bytestone.uk/hum3/gobank-products/isa/v1"
	mortgage "git.bytestone.uk/hum3/gobank-products/mortgage/v1"
	overdraft "git.bytestone.uk/hum3/gobank-products/overdraft/v1"
	personalloan "git.bytestone.uk/hum3/gobank-products/personalloan/v1"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/bank/parameters"
)

// listings is a definition's products: one on each of the six v1 rules
// versions, named after its rules.
func listings() []Listing {
	var out []Listing
	for _, v := range []gbp.Version{easyaccess.Version, fixedterm.Version, isa.Version, personalloan.Version, mortgage.Version, overdraft.Version} {
		out = append(out, Listing{ID: v.Product(), Name: v.Name(), Rules: v, Description: v.Name()})
	}
	return out
}

var jan2 = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

// open is the catalogue, adopted on jan2, over a database with the ledger
// and the settings open and a register of accounts: the contract view the
// pass reads, over the test's own table. What is registered is the
// customers' business; the pass needs only the view.
func open(t *testing.T) (*Products, *ledger.Ledger) {
	t.Helper()
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, m := range append(Schema.Migrations, parameters.Schema.Migrations...) {
		for _, stmt := range m.Statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatal(err)
			}
		}
	}
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
	p := New(db, parameters.Open(db), listings())
	// A clock that steps a second per reading: settings decided in one
	// test are told apart by when they were decided.
	tick := 0
	p.now = func() time.Time { tick++; return jan2.Add(9*time.Hour + time.Duration(tick)*time.Second) }
	if err := p.Adopt(context.Background(), jan2); err != nil {
		t.Fatal(err)
	}
	return p, books
}

// register opens a customer account on the ledger, funds it from equity on
// day (a movement alone: the day's position is the pass's to write) and
// registers it on a product for the pass.
func register(t *testing.T, p *Products, books *ledger.Ledger, product string, customer string, amount luca.Amount, day time.Time) Account {
	t.Helper()
	family, _ := p.Family(product)
	id, err := books.OpenCustomerAccount(customer, product, family)
	if err != nil {
		t.Fatal(err)
	}
	if amount > 0 {
		if _, err := books.RecordMovement(books.Chart.EquityCapital, id, amount, luca.CodeBookTransfer, day, "funding"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.db.Exec(`INSERT INTO registered (ledger_account_id, product_id) VALUES ($1, $2)`, id, product); err != nil {
		t.Fatal(err)
	}
	return Account{ID: id, Product: product}
}

// The catalogue is the definition's products, each an instance of a rules
// version the build carries (ADR-0006 as amended), adopted on the day the
// bank first ran with them; adopting again changes nothing.
func TestCatalogueIsTheDefinitionsProductsAdopted(t *testing.T) {
	p, _ := open(t)
	if n := len(p.All()); n != 6 {
		t.Fatalf("%d products, want 6", n)
	}
	for _, prod := range p.All() {
		if prod.ID == "" || prod.Name == "" || prod.Currency != "GBP" || prod.RateBps() <= 0 || !prod.Carried() || prod.Rules.Version() != 1 || !prod.Adopted.Equal(jan2) || prod.ModuleVersion == "" {
			t.Errorf("product %+v needs an ID, a name, GBP, a positive rate, v1 rules carried, adopted on jan2 by a module version", prod)
		}
		if got, ok := p.ByID(prod.ID); !ok || got.ID != prod.ID {
			t.Errorf("ByID(%s) = %+v, %v", prod.ID, got, ok)
		}
	}
	if _, ok := p.ByID("no-such-product"); ok {
		t.Error("an unknown product was found")
	}
	var rules string
	var version int
	if err := p.db.QueryRow(`SELECT rules, rules_version FROM products WHERE product_id = 'easy-access'`).Scan(&rules, &version); err != nil || rules != "easy-access" || version != 1 {
		t.Errorf("easy-access runs on %s v%d, %v; want easy-access v1", rules, version, err)
	}
	if err := p.Adopt(context.Background(), jan2.AddDate(0, 0, 30)); err != nil {
		t.Fatal(err)
	}
	if prod, _ := p.ByID("easy-access"); !prod.Adopted.Equal(jan2) {
		t.Errorf("adopting again moved the adopted day to %s", prod.Adopted)
	}
	var n int
	if err := p.db.QueryRow(`SELECT COUNT(*) FROM products`).Scan(&n); err != nil || n != 6 {
		t.Errorf("%d adoption rows, want 6", n)
	}
}

// A product's parameters are its own: defaulted from what its rules
// version publishes, and set per product by a setting at the product's
// scope (the contract's "version" scope is the product's).
func TestProductParametersDefaultFromTheRulesAndAreSetPerProduct(t *testing.T) {
	p, _ := open(t)
	ctx := context.Background()
	easy, _ := p.ByID("easy-access")
	rate, err := p.resolve(ctx, p.db, easy, "", "rate_bps", jan2, p.now())
	if err != nil || rate.Bps() != 150 {
		t.Fatalf("easy-access rate = %v, %v; want the published 150", rate.Bps(), err)
	}
	if err := p.params.Set(ctx, nil, parameters.Setting{Scope: gbp.ScopeVersion, Owner: easy.ID, Key: "rate_bps", Value: "200", Effective: jan2, Decided: p.now(), By: "test"}); err != nil {
		t.Fatal(err)
	}
	if rate, _ := p.resolve(ctx, p.db, easy, "", "rate_bps", jan2, p.now()); rate.Bps() != 200 {
		t.Errorf("easy-access rate after the setting = %d, want 200", rate.Bps())
	}
	if isa, _ := p.ByID("isa"); isa.RateBps() == 200 {
		t.Error("the setting reached another product")
	}
}

// A product on record whose rules this build does not carry stays in the
// catalogue, not carried: its accounts are left alone until a build that
// carries the rules runs.
func TestProductOnRulesTheBuildLacksIsNotCarried(t *testing.T) {
	p, books := open(t)
	if _, err := p.db.Exec(`INSERT INTO products (product_id, name, rules, rules_version, adopted, module_version) VALUES ('fixed-5y', 'Fixed 5 Year', 'fixed-term', 3, $1, 'v9.9.9')`, jan2); err != nil {
		t.Fatal(err)
	}
	q := New(p.db, p.params, listings())
	if err := q.Adopt(context.Background(), jan2); err != nil {
		t.Fatal(err)
	}
	fixed, ok := q.ByID("fixed-5y")
	if !ok || fixed.Carried() || fixed.Name != "Fixed 5 Year" || fixed.RateBps() != 0 || !fixed.OnSale() {
		t.Errorf("fixed-5y = %+v, %v; want on record, named, not carried, no rate", fixed, ok)
	}
	if n := len(q.All()); n != 7 {
		t.Errorf("%d products, want the six and the one on record", n)
	}
	a := register(t, q, books, "fixed-5y", "cust-001", 100_00, jan2)
	if err := q.Open(books, a, jan2); !errors.Is(err, ErrNotCarried) {
		t.Errorf("opening on rules the build lacks: %v, want ErrNotCarried", err)
	}
}

// A day's work on an account projects its position for the day: the
// balance as it stands and the day's interest on it, over yesterday's.
func TestAccountDayProjectsTheDay(t *testing.T) {
	p, books := open(t)
	easy, _ := p.ByID("easy-access")
	a := register(t, p, books, "easy-access", "cust-001", 1000_00, jan2)
	res, err := p.AccountDay(books, a, jan2)
	if err != nil {
		t.Fatal(err)
	}
	want := int64(1000_00) * easy.RateBps()
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
// yesterday's last second, yesterday's position keeps the remainder, and
// the posting is recorded with the version, the event, the parameters
// the rule read and the position it applied them to.
func TestAccountDayAppliesAtTheCycleEndAndRecordsWhy(t *testing.T) {
	p, books := open(t)
	easy, _ := p.ByID("easy-access")
	jan31 := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
	a := register(t, p, books, "easy-access", "cust-001", 1000_00, jan31)
	// A month's accrual on the 31 Jan position: 1000.00 at 1.5% for 30 days
	// is 123.3p, so 123p apply and 0.3p remain.
	accrued := luca.Fraction{Num: 1000_00 * easy.RateBps() * 30, Den: gbp.AccrualDenominator}
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
	if today.Balance != 1000_00+123 || today.Accrued.Num != closed.Accrued.Num+int64(today.Balance)*easy.RateBps() {
		t.Errorf("1 Feb position %+v; want the applied balance accruing a day over the remainder", today)
	}
	if res.Accrued != int64(today.Balance)*easy.RateBps() {
		t.Errorf("accrued counted %d, want the day's %d", res.Accrued, int64(today.Balance)*easy.RateBps())
	}
	recs, err := p.Postings(context.Background(), a.ID)
	if err != nil || len(recs) != 1 {
		t.Fatalf("postings = %+v, %v; want one", recs, err)
	}
	r := recs[0]
	if r.Product != "easy-access" || r.Rules != "easy-access" || r.RulesVersion != 1 || r.Event != gbp.EventDay || r.InputBalance != 1000_00 || r.InputAccruedNum != accrued.Num {
		t.Errorf("record %+v; want easy-access on easy-access v1, day, applied to the 31 Jan position", r)
	}
	if !strings.Contains(r.Parameters, "cycle=monthly") || !strings.Contains(r.Parameters, "rate_bps=150") {
		t.Errorf("record parameters %q; want the cycle and the rate the rule read", r.Parameters)
	}
	var movementID string
	if err := p.db.QueryRow(`SELECT id FROM contract_ledger_movements WHERE to_account_id = $1 AND code = $2`, a.ID, luca.CodeInterestAccrual).Scan(&movementID); err != nil || movementID != r.MovementID {
		t.Errorf("record names movement %s, the ledger has %s (%v)", r.MovementID, movementID, err)
	}
}

// An event moves both positions for the day at once and runs the
// post-posting rule on the accounts it touched, so today's accrual is on
// the balance after the event; an event counts no accrual of its own.
func TestPostEventRewritesTheDaysPositions(t *testing.T) {
	p, books := open(t)
	easy, _ := p.ByID("easy-access")
	from := register(t, p, books, "easy-access", "cust-001", 1000_00, jan2)
	to := register(t, p, books, "easy-access", "cust-002", 500_00, jan2)
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
		if err != nil || pos == nil || pos.Balance != c.want || pos.Accrued.Num != int64(c.want)*easy.RateBps() {
			t.Errorf("%s: position %+v, %v; want %d accruing a day on it", c.a.ID, pos, err, c.want)
		}
	}
	if _, err := p.PostEvent(books, jan2, from.ID, "", 1, luca.CodeBookTransfer, "nowhere"); err == nil {
		t.Error("an event to no account was posted")
	}
}

// The open event writes the account-scoped settings a version needs:
// fixed term fixes the maturity day from its term, and an ISA counts its
// deposits from the funding on.
func TestOpenWritesTheVersionsAccountSettings(t *testing.T) {
	p, books := open(t)
	fixed := register(t, p, books, "fixed-term", "cust-001", 0, jan2)
	if err := p.Open(books, fixed, jan2); err != nil {
		t.Fatal(err)
	}
	v, _ := p.ByID("fixed-term")
	maturity, err := p.resolve(context.Background(), p.db, v, fixed.ID, "maturity_day", jan2, p.now())
	if err != nil || maturity.Day() != jan2.AddDate(0, 24, 0) {
		t.Errorf("maturity = %v, %v; want 24 months on", maturity.Day(), err)
	}
	if pos, _ := books.PositionAt(fixed.ID, jan2); pos == nil {
		t.Error("an opened account has no position for the day")
	}

	saver := register(t, p, books, "isa", "cust-002", 0, jan2)
	if err := p.Open(books, saver, jan2); err != nil {
		t.Fatal(err)
	}
	// A deposit credits the saver and debits cash (ADR-0008).
	if _, err := p.PostEvent(books, jan2, saver.ID, books.Chart.Cash, 5000_00, luca.CodeBookTransfer, "funding", saver); err != nil {
		t.Fatal(err)
	}
	iv, _ := p.ByID("isa")
	deposited, err := p.resolve(context.Background(), p.db, iv, saver.ID, "deposited", jan2, p.now())
	if err != nil || deposited.Money() != 5000_00 {
		t.Errorf("deposited = %v, %v; want the funding counted", deposited.Money(), err)
	}
	if err := p.Open(books, Account{ID: "x", Product: "no-such-product"}, jan2); !errors.Is(err, ErrNotCarried) {
		t.Errorf("opening on a product the catalogue lacks: %v, want ErrNotCarried", err)
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
// short, it leaves the rest for the next pass over the same day; an
// account on a version this build does not carry is left and counted.
func TestPassVisitsEveryUnprojectedAccountAndResumes(t *testing.T) {
	p, books := open(t)
	ctx := context.Background()
	easy, _ := p.ByID("easy-access")
	loan, _ := p.ByID("personal-loan")
	var accounts []Account
	for i := range 3 {
		accounts = append(accounts, register(t, p, books, "easy-access", "cust-00"+string(rune('1'+i)), 1000_00, jan2))
	}
	accounts = append(accounts, register(t, p, books, "personal-loan", "cust-004", 2000_00, jan2))
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
	wantSavings := 3 * int64(1000_00) * easy.RateBps()
	wantLending := int64(2000_00) * loan.RateBps()
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

	// An account on a product whose rules the build does not carry is
	// skipped, not run.
	if _, err := p.db.Exec(`INSERT INTO products (product_id, name, rules, rules_version, adopted, module_version) VALUES ('easy-access-v7', 'Easy Access 7', 'easy-access', 7, $1, 'v9.9.9')`, jan2); err != nil {
		t.Fatal(err)
	}
	if err := p.Adopt(ctx, jan2); err != nil {
		t.Fatal(err)
	}
	register(t, p, books, "easy-access-v7", "cust-005", 100_00, jan2)
	skipped := p.RunPass(ctx, books, jan2, &prog, nil)
	if skipped.Visited != 0 || skipped.Skipped != 1 {
		t.Errorf("pass with a product not carried: %+v; want nothing visited, one skipped", skipped)
	}
	if pending, _ := p.AnyUnprojected(ctx, jan2); !pending {
		t.Error("the skipped account counts as projected")
	}
}

// A product the definition withdraws comes off sale on the day the bank
// first runs with it withdrawn, and stays in the catalogue for the
// accounts on it.
func TestWithdrawnProductIsOffSale(t *testing.T) {
	p, _ := open(t)
	listings := listings()
	for i := range listings {
		if listings[i].ID == "isa" {
			listings[i].Withdrawn = true
		}
	}
	later := jan2.AddDate(0, 1, 0)
	q := New(p.db, p.params, listings)
	if err := q.Adopt(context.Background(), later); err != nil {
		t.Fatal(err)
	}
	got, ok := q.ByID("isa")
	if !ok || got.OnSale() || !got.Withdrawn.Equal(later) || !got.Adopted.Equal(jan2) {
		t.Errorf("isa = %+v, %v; want adopted jan2, withdrawn a month on, still in the catalogue", got, ok)
	}
	if err := q.Adopt(context.Background(), later.AddDate(0, 1, 0)); err != nil {
		t.Fatal(err)
	}
	if got, _ := q.ByID("isa"); !got.Withdrawn.Equal(later) {
		t.Errorf("adopting again moved the withdrawn day to %s", got.Withdrawn)
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

// tracker is a rules version that reads the market (ADR-0009): its rate
// derives from the currency's risk-free rate, a parameter at the market
// scope that the version declares by key alone.
type tracker struct{ gbp.Interest }

func (tracker) Product() string           { return "tracker" }
func (tracker) Version() int              { return 1 }
func (tracker) Name() string              { return "Tracker" }
func (tracker) Family() gbp.ProductFamily { return gbp.FamilySavings }
func (tracker) Parameters() []gbp.Declaration {
	floor := int64(0)
	return []gbp.Declaration{
		{Key: gbp.RiskFreeRateKey, Scope: gbp.ScopeMarket, Kind: gbp.KindBps},
		{Key: "rate_bps", Scope: gbp.ScopeVersion, Kind: gbp.KindBps, Derived: &gbp.Derivation{Source: gbp.RiskFreeRateKey, SpreadBps: -15, Floor: &floor}},
		{Key: "cycle", Scope: gbp.ScopeVersion, Kind: gbp.KindCycle, Published: "monthly"},
	}
}

// A market-scope parameter resolves for the currency of the product whose
// rules read it, from the market's publications; with none published the
// product cannot run.
func TestMarketParametersResolveForTheProductsCurrency(t *testing.T) {
	p, _ := open(t)
	ctx := context.Background()
	p.listings = append(p.listings, Listing{ID: "tracker", Name: "Tracker", Rules: tracker{}, Description: "tracks the market"})
	if err := p.Adopt(ctx, jan2); err != nil {
		t.Fatal(err)
	}
	trk, ok := p.ByID("tracker")
	if !ok || trk.Currency != "GBP" {
		t.Fatalf("tracker = %+v, %v", trk, ok)
	}
	if _, err := p.resolve(ctx, p.db, trk, "", "rate_bps", jan2, p.now()); err == nil || !strings.Contains(err.Error(), "market") {
		t.Errorf("with nothing published: %v, want a refusal naming the market", err)
	}
	noon := jan2.Add(12 * time.Hour)
	if err := p.params.Set(ctx, nil, parameters.Setting{Scope: gbp.ScopeMarket, Owner: "GBP", Key: gbp.RiskFreeRateKey, Value: "525", Effective: noon, Decided: noon, By: "Bank of England"}); err != nil {
		t.Fatal(err)
	}
	rate, err := p.resolve(ctx, p.db, trk, "", "rate_bps", jan2, noon.Add(time.Second))
	if err != nil || rate.Bps() != 510 {
		t.Errorf("tracker rate on the day of a noon publication = %v, %v; want 525 − 15", rate.Bps(), err)
	}
	if rate, _ := p.resolve(ctx, p.db, trk, "", "rate_bps", jan2.AddDate(0, 0, -1), noon.Add(time.Second)); rate.Bps() == 510 {
		t.Error("the publication reached the day before")
	}
}
