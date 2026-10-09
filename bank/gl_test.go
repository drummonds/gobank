package bank

import (
	"testing"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
)

// The general ledger in shadow (ADR-0005, story 1.6a.2): nothing reads
// it yet, but it is posted per event, closed behind each day's pass and
// reconciled to the sub-ledger, and the position says how far it is
// posted and whether the close broke.

func (f *fixture) control(productID string) luca.Position {
	f.t.Helper()
	id, ok := f.bank.GeneralLedger().Control(productID)
	if !ok {
		f.t.Fatalf("no control account for %s", productID)
	}
	pos, err := f.bank.GeneralLedger().PositionAt(id, f.position().GL.PostedThrough)
	if err != nil || pos == nil {
		f.t.Fatalf("%s control position: %v %v", productID, pos, err)
	}
	return *pos
}

// The pass of day D closes day D-1 on the GL: once every account has
// its position for D, the controls carry D-1's funding, the journal
// carries D-1's interest, and the close reconciles with no break. After
// downtime the days close one by one, none skipped.
func TestTheGeneralLedgerClosesYesterdayBehindThePass(t *testing.T) {
	f := open(t)
	if _, err := f.bank.OpenCustomer(f.ctx, ada); err != nil {
		t.Fatal(err)
	}
	if gl := f.position().GL; !gl.PostedThrough.IsZero() || gl.Breaks != 0 {
		t.Errorf("GL %+v before any close; want posted through nothing", gl)
	}
	f.nextDay()
	gl := f.position().GL
	if !gl.PostedThrough.Equal(opening) || gl.Breaks != 0 {
		t.Fatalf("GL %+v after the first pass; want posted through the opening day with no break", gl)
	}
	const deposit, loan = luca.Amount(1000_00), luca.Amount(850_00)
	if easy, loans := f.control(gbp.EasyAccess().ID), f.control(gbp.PersonalLoan().ID); easy.Balance != deposit || loans.Balance != loan {
		t.Errorf("controls easy %+v, loans %+v; want %d and %d", easy, loans, deposit, loan)
	}
	// Through the month end: the applications of 31 Jan are journalled
	// to the GL by 1 Feb's pass, and both ledgers' P&L accounts agree.
	for f.now.Month() == time.January {
		f.nextDay()
	}
	gl = f.position().GL
	if want := time.Date(2020, 1, 31, 0, 0, 0, 0, time.UTC); !gl.PostedThrough.Equal(want) || gl.Breaks != 0 {
		t.Fatalf("GL %+v on 1 Feb; want posted through 31 Jan with no break", gl)
	}
	sub, g := f.bank.Ledger(), f.bank.GeneralLedger()
	for _, pl := range []struct {
		name    string
		sub, gl string
	}{
		{"interest expense", sub.Chart.ExpenseInterest, g.Chart.ExpenseInterest},
		{"interest income", sub.Chart.IncomeInterest, g.Chart.IncomeInterest},
	} {
		subBal, _ := sub.Balance(pl.sub)
		glBal, _ := g.Balance(pl.gl)
		if subBal == 0 || subBal != glBal {
			t.Errorf("%s: sub-ledger %d, GL %d; want equal and non-zero", pl.name, subBal, glBal)
		}
	}
	var journalled int
	f.db.QueryRow(`SELECT COUNT(*) FROM gl_journal WHERE day = $1`, gl.PostedThrough).Scan(&journalled)
	if journalled != 2 {
		t.Errorf("%d journal lines for 31 Jan, want one per product with an application (easy access, personal loan)", journalled)
	}
	easy := f.control(gbp.EasyAccess().ID)
	if easy.Accrued.Num == 0 {
		t.Errorf("easy access control %+v; want the accrued interest carried from the sub-ledger", easy)
	}
	// Three days down: the catch-up closes each day in turn.
	f.now = f.now.AddDate(0, 0, 3)
	if _, err := f.bank.StartDay(f.ctx); err != nil {
		t.Fatal(err)
	}
	var closes int
	f.db.QueryRow(`SELECT COUNT(*) FROM gl_closes`).Scan(&closes)
	if gl = f.position().GL; !gl.PostedThrough.Equal(core.BusinessDay(f.now).AddDate(0, 0, -1)) || gl.Breaks != 0 || closes != 34 {
		t.Errorf("GL %+v, %d closes after the catch-up; want posted through yesterday, 34 days closed, no break", gl, closes)
	}
}

// A transfer across products moves the controls in the GL; one within a
// product posts nothing there. Either way the close reconciles.
func TestATransferAcrossProductsMovesTheControls(t *testing.T) {
	f := open(t)
	from, _ := f.bank.OpenCustomer(f.ctx, saver("Easy", 1000_00))
	isa, _ := f.bank.OpenCustomer(f.ctx, core.NewCustomer{PII: core.PII{Name: "Isa"}, Accounts: []core.NewAccount{{ProductID: gbp.ISA().ID, Opening: 500_00}}})
	easy, _ := f.bank.OpenCustomer(f.ctx, saver("Easy Too", 200_00))
	if _, err := f.bank.Transfer(f.ctx, core.Transfer{From: from.ID, To: isa.ID, Amount: 100_00}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.bank.Transfer(f.ctx, core.Transfer{From: from.ID, To: easy.ID, Amount: 50_00}); err != nil {
		t.Fatal(err)
	}
	g := f.bank.GeneralLedger()
	easyControl, _ := g.Control(gbp.EasyAccess().ID)
	isaControl, _ := g.Control(gbp.ISA().ID)
	easyBal, _ := g.Balance(easyControl)
	isaBal, _ := g.Balance(isaControl)
	if easyBal != 1100_00 || isaBal != 600_00 {
		t.Errorf("controls easy %d, isa %d; want 110000 (the within-product transfer nets out) and 60000", easyBal, isaBal)
	}
	var movements int
	f.db.QueryRow(`SELECT COUNT(*) FROM gl_movements`).Scan(&movements)
	if movements != 4 {
		t.Errorf("%d GL movements, want 3 fundings and one cross-product transfer", movements)
	}
	f.nextDay()
	if gl := f.position().GL; gl.Breaks != 0 {
		t.Errorf("GL %+v; want no break", gl)
	}
}

// A bank that ran before the GL existed is adopted: the GL's first
// close takes each control's balance from the sub-ledger instead of
// journalling, so the reconciliation holds from the first day, and a
// close owed when the process starts is made then.
func TestTheGeneralLedgerAdoptsARunningBank(t *testing.T) {
	f := open(t)
	if _, err := f.bank.OpenCustomer(f.ctx, ada); err != nil {
		t.Fatal(err)
	}
	f.nextDay()
	f.nextDay()
	// The database as a bank from before the GL leaves it: the GL's
	// tables exist (go-luca creates them) and hold nothing.
	for _, table := range []string{"gl_movements", "gl_balances_live", "gl_ledger_day", "gl_closes", "gl_journal", "gl_reconciliations", "gl_opening"} {
		if _, err := f.db.Exec(`DELETE FROM ` + table); err != nil {
			t.Fatal(err)
		}
	}
	f = reopen(t, f.db, f.now)
	if gl := f.position().GL; !gl.PostedThrough.IsZero() {
		t.Errorf("GL %+v at open; want nothing closed yet", gl)
	}
	if _, err := f.bank.StartDay(f.ctx); err != nil {
		t.Fatal(err)
	}
	gl := f.position().GL
	if want := opening.AddDate(0, 0, 1); !gl.PostedThrough.Equal(want) || gl.Breaks != 0 {
		t.Fatalf("GL %+v; want yesterday closed with no break: the opening balances taken from the sub-ledger", gl)
	}
	if easy := f.control(gbp.EasyAccess().ID); easy.Balance != 1000_00 {
		t.Errorf("easy access control %+v, want the sub-ledger's 100000", easy)
	}
	var kinds int
	f.db.QueryRow(`SELECT COUNT(*) FROM gl_movements WHERE code = $1`, luca.CodeOpeningBalance).Scan(&kinds)
	if kinds != 2 {
		t.Errorf("%d opening-balance movements, want one per funded product", kinds)
	}
	f.nextDay()
	if gl := f.position().GL; !gl.PostedThrough.Equal(opening.AddDate(0, 0, 2)) || gl.Breaks != 0 {
		t.Errorf("GL %+v; want the next day closed normally", gl)
	}
}
