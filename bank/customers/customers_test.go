package customers

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	_ "git.bytestone.uk/hum3/go-postgres"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/bank/products"
	"git.bytestone.uk/hum3/gobank/core"
	store "git.bytestone.uk/hum3/gobanks-customers"
)

var key = store.FixedKeyProvider{Key: []byte("gobank-test-pii-key-32bytes!!!!!")}

var jan2 = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

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

type fixture struct {
	db       *sql.DB
	books    *ledger.Ledger
	products *products.Products
	c        *Customers
}

func open(t *testing.T) fixture {
	t.Helper()
	db := openDB(t)
	return reopen(t, db)
}

func reopen(t *testing.T, db *sql.DB) fixture {
	t.Helper()
	books, err := ledger.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	catalogue := products.New(db)
	c, err := Open(db, key, books, catalogue, 42)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{db: db, books: books, products: catalogue, c: c}
}

// openCustomer plans and persists a customer in one transaction, funding
// each account from equity on jan2 with the opening asked for.
func (f fixture) openCustomer(t *testing.T, n core.NewCustomer) Record {
	t.Helper()
	ctx := context.Background()
	plan, err := f.c.Plan(jan2, n)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.c.Persist(ctx, tx, tx, f.books.WithTx(tx), &plan); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	for i, a := range n.Accounts {
		if a.Opening > 0 {
			if err := f.books.WithTx(tx).Post(f.books.Chart.EquityCapital, plan.Record.Accounts[i].LedgerAccountID, a.Opening, luca.CodeBookTransfer, jan2, "PAY-00000"+string(rune('1'+i))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return plan.Record
}

var ada = core.NewCustomer{
	KYC:      core.KYC{Verified: true, RiskRating: "Standard"},
	PII:      core.PII{Name: "Ada Lovelace", NI: "AB123456C", DOB: "1815-12-10", Address: "12 St James's Square", Email: "ada@example.com", Phone: "07000 000001"},
	Accounts: []core.NewAccount{{ProductID: gbp.EasyAccess().ID, Opening: 1000_00}, {ProductID: gbp.PersonalLoan().ID, Opening: 500_00}},
}

// A planned customer gets the next number, is dated on the day, and each
// account its bank details; persisted, they are on the books with their
// ledger accounts registered, their PII readable and their figures from
// the ledger.
func TestOpenACustomer(t *testing.T) {
	f := open(t)
	ctx := context.Background()
	if n, _ := f.c.Count(ctx); n != 0 {
		t.Fatalf("%d customers on a fresh database", n)
	}
	rec := f.openCustomer(t, ada)
	if rec.ID != core.CustomerID(1) || !rec.JoinDate.Equal(jan2) || !rec.KYC.LastCheck.Equal(jan2) || !rec.KYC.Verified {
		t.Errorf("record %+v; want cust-001 joined and checked on 2 Jan", rec)
	}
	got, err := f.c.ByID(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Accounts) != 2 {
		t.Fatalf("accounts %+v, want 2", got.Accounts)
	}
	num := regexp.MustCompile(`^\d{8}$`)
	for i, a := range got.Accounts {
		if a.SortCode != SortCode || !num.MatchString(a.AccountNum) || a.LedgerAccountID == "" || !a.OpenDate.Equal(jan2) {
			t.Errorf("account %d = %+v; want the sort code, an 8-digit number, a ledger account, opened 2 Jan", i, a)
		}
	}
	if got.Accounts[0].Family != gbp.FamilySavings || got.Accounts[0].Balance != 1000_00 || got.Accounts[1].Family != gbp.FamilyLending || got.Accounts[1].Balance != 500_00 {
		t.Errorf("accounts %+v; want savings 100000 and lending 50000 from the ledger", got.Accounts)
	}
	if !f.c.Exists(ctx, rec.ID) || f.c.Exists(ctx, "cust-999") {
		t.Error("Exists is wrong about the register")
	}
	if name := f.c.Name(ctx, rec.ID); name != "Ada Lovelace" {
		t.Errorf("Name = %q", name)
	}
	if name := f.c.Name(ctx, "cust-999"); name != "cust-999" {
		t.Errorf("an unknown customer's name = %q, want the ID", name)
	}
	pii, err := f.c.PII(ctx, rec.ID)
	if err != nil || pii != ada.PII {
		t.Errorf("PII = %+v, %v; want %+v", pii, err, ada.PII)
	}
	if _, err := f.c.ByID(ctx, "cust-999"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("unknown customer: %v, want ErrNotFound", err)
	}
	if _, err := f.c.PII(ctx, "cust-999"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("unknown customer's PII: %v, want ErrNotFound", err)
	}
	second := f.openCustomer(t, core.NewCustomer{PII: core.PII{Name: "Second"}, Accounts: []core.NewAccount{{ProductID: gbp.ISA().ID}}})
	if second.ID != core.CustomerID(2) {
		t.Errorf("second customer is %s", second.ID)
	}
	page, total, err := f.c.Page(ctx, 1, 50)
	if err != nil || total != 2 || len(page) != 2 || page[0].ID != rec.ID || page[1].ID != second.ID {
		t.Errorf("Page = %d of %d, %v; want both, oldest first", len(page), total, err)
	}
}

// A plan is refused for no accounts, a negative opening or an unknown
// product, before a number is used.
func TestPlanRefusesBadRequests(t *testing.T) {
	f := open(t)
	for _, c := range []struct {
		n    core.NewCustomer
		want error
	}{
		{core.NewCustomer{}, core.ErrInvalidAmount},
		{core.NewCustomer{Accounts: []core.NewAccount{{ProductID: gbp.ISA().ID, Opening: -1}}}, core.ErrInvalidAmount},
		{core.NewCustomer{Accounts: []core.NewAccount{{ProductID: "no-such-product"}}}, core.ErrNotFound},
	} {
		if _, err := f.c.Plan(jan2, c.n); !errors.Is(err, c.want) {
			t.Errorf("Plan(%+v): %v, want %v", c.n, err, c.want)
		}
	}
	if plan, err := f.c.Plan(jan2, ada); err != nil || plan.Record.ID != core.CustomerID(1) {
		t.Errorf("after refusals the first number is still free: %+v, %v", plan.Record, err)
	}
}

// A customer the store refuses is not written at all, and the numbering
// carries on past the number they used.
func TestRefusedCustomerLeavesNoTrace(t *testing.T) {
	f := open(t)
	ctx := context.Background()
	// Occupy cust-001 so the first planned customer cannot be stored.
	if err := f.c.Store().Create(ctx, store.CustomerRecord{ID: "cust-001", Ref: "cust-001", JoinDate: jan2}, store.PIIInput{Name: "Squatter"}); err != nil {
		t.Fatal(err)
	}
	plan, err := f.c.Plan(jan2, ada)
	if err != nil {
		t.Fatal(err)
	}
	tx, _ := f.db.Begin()
	if err := f.c.Persist(ctx, tx, tx, f.books.WithTx(tx), &plan); err == nil {
		t.Fatal("persisting over an occupied number succeeded")
	}
	tx.Rollback()
	var registered int
	f.db.QueryRow(`SELECT COUNT(*) FROM customer_accounts`).Scan(&registered)
	if registered != 0 {
		t.Errorf("%d accounts registered for a refused customer", registered)
	}
	next := f.openCustomer(t, core.NewCustomer{PII: core.PII{Name: "Next"}, Accounts: []core.NewAccount{{ProductID: gbp.ISA().ID}}})
	if next.ID != core.CustomerID(2) {
		t.Errorf("the customer after a refusal is %s, want cust-002", next.ID)
	}
}

// Reopened over the same database, the numbering continues after the
// newest customer on record.
func TestReopenContinuesTheNumbering(t *testing.T) {
	f := open(t)
	for range 3 {
		f.openCustomer(t, core.NewCustomer{PII: core.PII{Name: "N"}, Accounts: []core.NewAccount{{ProductID: gbp.ISA().ID}}})
	}
	g := reopen(t, f.db)
	rec := g.openCustomer(t, core.NewCustomer{PII: core.PII{Name: "Fourth"}, Accounts: []core.NewAccount{{ProductID: gbp.ISA().ID}}})
	if rec.ID != core.CustomerID(4) {
		t.Errorf("after reopening the next customer is %s, want cust-004", rec.ID)
	}
}

// The statement reads each movement on a customer's accounts as a line
// with the balance it left, newest first, across all accounts or one.
func TestStatementLines(t *testing.T) {
	f := open(t)
	ctx := context.Background()
	rec := f.openCustomer(t, ada)
	savings, loan := rec.Accounts[0], rec.Accounts[1]
	other := f.openCustomer(t, core.NewCustomer{PII: core.PII{Name: "Other"}, Accounts: []core.NewAccount{{ProductID: gbp.EasyAccess().ID, Opening: 10_00}}})
	jan3 := jan2.AddDate(0, 0, 1)
	if err := f.books.Post(savings.LedgerAccountID, other.Accounts[0].LedgerAccountID, 200_00, luca.CodeBookTransfer, jan3, "PAY-000003"); err != nil {
		t.Fatal(err)
	}
	interest, _ := f.books.Account(ledger.ExpenseInterest)
	if _, err := f.books.RecordMovement(interest, savings.LedgerAccountID, 1_23, luca.CodeInterestAccrual, jan3.Add(time.Hour), "Interest for January"); err != nil {
		t.Fatal(err)
	}

	all, err := f.c.Transactions(ctx, rec.ID, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	types := []string{}
	for _, e := range all.Entries {
		types = append(types, e.Type)
	}
	want := []string{"Interest", "Transfer Out", "Loan", "Deposit"}
	if all.Total != 4 || len(types) != 4 || types[0] != want[0] || types[1] != want[1] || (types[2] != want[2] && types[2] != want[3]) {
		t.Errorf("statement types %v, want %v (newest first)", types, want)
	}
	one, err := f.c.AccountTransactions(ctx, rec.ID, 0, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if one.Total != 3 || one.Entries[0].Balance != 1000_00-200_00+1_23 || one.Entries[2].Balance != 1000_00 || one.Entries[0].Date != jan3.Format(time.DateOnly) {
		t.Errorf("savings statement %+v; want deposit, transfer out and interest with running balances", one.Entries)
	}
	if loanOnly, _ := f.c.AccountTransactions(ctx, rec.ID, 1, 1, 20); loanOnly.Total != 1 || loanOnly.Entries[0].Type != "Loan" || loanOnly.Entries[0].ProductName != loan.ProductName {
		t.Errorf("loan statement %+v", loanOnly.Entries)
	}
	if p, _ := f.c.Transactions(ctx, rec.ID, 2, 3); p.Total != 4 || len(p.Entries) != 1 || p.HasMore() {
		t.Errorf("page 2 of 3 per page: %+v", p)
	}
	if _, err := f.c.AccountTransactions(ctx, rec.ID, 5, 1, 20); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("an index the customer does not hold: %v", err)
	}
	if _, err := f.c.Transactions(ctx, "cust-999", 1, 20); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("an unknown customer's statement: %v", err)
	}
}

// The decision table for how a movement reads on the statement.
func TestTxTypeOf(t *testing.T) {
	cases := []struct {
		code, counterparty string
		in                 bool
		family             gbp.ProductFamily
		want               TxType
	}{
		{luca.CodeInterestAccrual, ledger.ExpenseInterest, true, gbp.FamilySavings, TxInterestCredit},
		{luca.CodeInterestAccrual, ledger.IncomeInterest, true, gbp.FamilyLending, TxInterestDebit},
		{luca.CodeBookTransfer, ledger.EquityCapital, true, gbp.FamilySavings, TxDepositIn},
		{luca.CodeBookTransfer, ledger.EquityCapital, true, gbp.FamilyLending, TxLoanDisbursement},
		{luca.CodeBookTransfer, ledger.SavingsRoot + ":C000002:easy-access", true, gbp.FamilySavings, TxTransferIn},
		{luca.CodeBookTransfer, ledger.SavingsRoot + ":C000002:easy-access", false, gbp.FamilySavings, TxTransferOut},
	}
	for _, c := range cases {
		if got := TxTypeOf(c.code, c.counterparty, c.in, c.family); got != c.want {
			t.Errorf("TxTypeOf(%s, %s, in=%v, %s) = %s, want %s", c.code, c.counterparty, c.in, c.family, got, c.want)
		}
	}
}

// Savings interest to date per customer: applied plus accrued whole
// pence on savings accounts only, customers with none left out.
func TestSavingsInterestByCustomer(t *testing.T) {
	f := open(t)
	ctx := context.Background()
	rec := f.openCustomer(t, ada)
	f.openCustomer(t, core.NewCustomer{PII: core.PII{Name: "Borrower"}, Accounts: []core.NewAccount{{ProductID: gbp.PersonalLoan().ID, Opening: 100_00}}})
	savings := rec.Accounts[0]
	interest, _ := f.books.Account(ledger.ExpenseInterest)
	if _, err := f.books.RecordMovement(interest, savings.LedgerAccountID, 2_50, luca.CodeInterestAccrual, jan2.Add(time.Hour), "Interest"); err != nil {
		t.Fatal(err)
	}
	// 1.7 pence accrued: 1 whole penny counts.
	if _, err := f.books.Project(savings.LedgerAccountID, jan2, luca.Fraction{Num: 17 * gbp.AccrualDenominator / 10, Den: gbp.AccrualDenominator}); err != nil {
		t.Fatal(err)
	}
	got, err := f.c.SavingsInterest(ctx)
	if err != nil || len(got) != 1 || got[0].CustomerID != rec.ID || got[0].Interest != 2_51 {
		t.Errorf("SavingsInterest = %+v, %v; want %s at 251", got, err, rec.ID)
	}
}

// The schema is the component's own: versions in order from 1.
func TestSchemaVersionsRunFromOne(t *testing.T) {
	if Schema.Name != "customers" {
		t.Errorf("Schema.Name = %q", Schema.Name)
	}
	for i, m := range Schema.Migrations {
		if m.Version != i+1 {
			t.Errorf("migration %d listed where version %d is due", m.Version, i+1)
		}
	}
}
