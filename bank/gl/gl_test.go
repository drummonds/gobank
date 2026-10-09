package gl

import (
	"context"
	"database/sql"
	"testing"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	_ "git.bytestone.uk/hum3/go-postgres"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/products"
)

var day = time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, m := range Schema.Migrations {
		for _, stmt := range m.Statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("schema v%d: %v", m.Version, err)
			}
		}
	}
	return db
}

func open(t *testing.T, db *sql.DB, adopting bool) *Ledger {
	t.Helper()
	g, err := Open(db, products.Catalogue(), day, adopting)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// Opening the general ledger resolves its chart: the bank's own accounts
// and one control account per product, under the savings or the loans
// root by the product's family. Opening the same database again finds the
// same accounts, and the ledger's tables sit apart from the sub-ledger's.
func TestOpenResolvesTheChartAndAControlPerProduct(t *testing.T) {
	db := openDB(t)
	first := open(t, db, false)
	for path, id := range map[string]string{
		EquityCapital: first.Chart.EquityCapital, ExpenseInterest: first.Chart.ExpenseInterest, IncomeInterest: first.Chart.IncomeInterest,
	} {
		acct, err := first.GetAccountByID(id)
		if err != nil || acct == nil || acct.FullPath != path {
			t.Errorf("%s: resolved to %q, account %+v, %v", path, id, acct, err)
		}
	}
	for _, p := range products.Catalogue() {
		id, ok := first.Control(p.ID)
		if !ok {
			t.Errorf("%s: no control account", p.ID)
			continue
		}
		acct, _ := first.GetAccountByID(id)
		want := SavingsRoot + ":" + p.ID
		if p.Family == gbp.FamilyLending {
			want = LoansRoot + ":" + p.ID
		}
		if acct == nil || acct.FullPath != want {
			t.Errorf("%s: control at %+v, want %s", p.ID, acct, want)
		}
	}
	second := open(t, db, false)
	if second.Chart != first.Chart {
		t.Errorf("reopened chart %+v, want the same accounts %+v", second.Chart, first.Chart)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gl_accounts`).Scan(&n); err != nil || n != 3+len(products.Catalogue()) {
		t.Errorf("gl_accounts: %d rows, %v; want the chart and a control per product", n, err)
	}
}

// What posts to the general ledger per sub-ledger event (ADR-0005):
// funding is equity to the product's control, a transfer across products
// is control to control, a transfer within one product is nothing. Each
// is a movement insert only: no position moves until the close.
func TestWhatEachEventPosts(t *testing.T) {
	g := open(t, openDB(t), false)
	easy, isa, loan := gbp.EasyAccess().ID, gbp.ISA().ID, gbp.PersonalLoan().ID
	if err := g.Funding(day, easy, 1000_00, "PAY-000001"); err != nil {
		t.Fatal(err)
	}
	if err := g.Funding(day, loan, 850_00, "PAY-000002"); err != nil {
		t.Fatal(err)
	}
	if err := g.Transfer(day, easy, isa, 100_00, "PAY-000003"); err != nil {
		t.Fatal(err)
	}
	if err := g.Transfer(day, easy, easy, 50_00, "PAY-000004"); err != nil {
		t.Fatal(err)
	}
	balances := map[string]luca.Amount{}
	for _, id := range []string{easy, isa, loan} {
		c, _ := g.Control(id)
		balances[id], _ = g.Balance(c)
	}
	if balances[easy] != 900_00 || balances[isa] != 100_00 || balances[loan] != 850_00 {
		t.Errorf("controls %v; want easy 90000, isa 10000, loan 85000", balances)
	}
	if equity, _ := g.Balance(g.Chart.EquityCapital); equity != -1850_00 {
		t.Errorf("equity %d, want -185000: both fundings came from it", equity)
	}
	var movements, positions int
	g.db.QueryRow(`SELECT COUNT(*) FROM gl_movements`).Scan(&movements)
	g.db.QueryRow(`SELECT COUNT(*) FROM gl_balances_live`).Scan(&positions)
	if movements != 3 || positions != 0 {
		t.Errorf("%d movements, %d positions; want 3 inserts and no projection", movements, positions)
	}
	if err := g.Funding(day, "no-such-product", 1, "PAY-000005"); err == nil {
		t.Error("funding an unknown product posted")
	}
}

// Before any close the general ledger is posted through no day, and its
// opening is on record once: the day it opened and whether it adopted a
// bank already running, which decides how its first close is made.
func TestTheOpeningIsOnRecordOnce(t *testing.T) {
	db := openDB(t)
	g := open(t, db, true)
	s, err := g.Status(context.Background())
	if err != nil || !s.PostedThrough.IsZero() || len(s.Breaks) != 0 {
		t.Errorf("status %+v, %v; want posted through nothing, no breaks", s, err)
	}
	if !g.opening.day.Equal(day) || !g.opening.adopted {
		t.Errorf("opening %+v, want adopted on %s", g.opening, day.Format(time.DateOnly))
	}
	later := open(t, db, false) // a restart: the record stands
	if !later.opening.day.Equal(day) || !later.opening.adopted {
		t.Errorf("reopened: opening %+v, want the first record kept", later.opening)
	}
}
