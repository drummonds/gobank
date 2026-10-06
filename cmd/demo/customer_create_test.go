package main

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"git.bytestone.uk/hum3/gobank/bank/payments"
	store "git.bytestone.uk/hum3/gobanks-customers"
)

// TestCreateCustomerRefusedLeavesBookUnchanged: a customer the database
// refuses leaves no trace — not on the books, not in the tx log, no payments —
// and the next customer is created normally.
func TestCreateCustomerRefusedLeavesBookUnchanged(t *testing.T) {
	ds := NewDemoState()
	defer ds.db.Close()
	// Occupy cust-001 so persisting the first generated customer fails.
	if err := ds.Customers().Store().Create(context.Background(),
		store.CustomerRecord{ID: "cust-001", Ref: "cust-001", JoinDate: ds.position().Day},
		store.PIIInput{Name: "Squatter"}); err != nil {
		t.Fatal(err)
	}

	ds.createCustomer()

	if n, p := ds.customerCount(), ds.position(); n != 1 || p.Savings != 0 || p.Lending != 0 { // the squatter alone
		t.Fatalf("refused customer left %d customers, position %+v", n, p)
	}
	if p := ds.paymentCount(); p != 0 {
		t.Fatalf("refused customer left %d payments", p)
	}

	ds.createCustomer()
	if _, ok := ds.customerByID("cust-002"); !ok {
		t.Fatal("cust-002 not created after a refused customer")
	}
}

// TestCreateCustomerReleasesLockDuringPersist: the bank answers reads
// while a customer's database writes are in flight, so the dashboard and
// other creators are not stuck behind its round trips.
// Set GOBANK_PG_DSN to run.
func TestCreateCustomerReleasesLockDuringPersist(t *testing.T) {
	dsn := os.Getenv("GOBANK_PG_DSN")
	if dsn == "" {
		t.Skip("GOBANK_PG_DSN not set")
	}
	ds := NewDemoStateWithDSN(dsn)
	defer ds.db.Close()

	// Block every customer write behind a table lock held by another session.
	blocker, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	btx, err := blocker.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := btx.Exec(`LOCK TABLE customer_accounts IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() { ds.createCustomer(); close(done) }()

	time.Sleep(100 * time.Millisecond) // long enough for the creator to be blocked on the table lock
	answered := make(chan struct{})
	go func() { ds.position(); close(answered) }()
	select {
	case <-answered:
	case <-time.After(2 * time.Second):
		_ = btx.Rollback()
		t.Fatal("the bank did not answer a read while the customer's writes are blocked")
	}
	select {
	case <-done:
		t.Fatal("customer finished despite its writes being blocked")
	default:
	}

	if err := btx.Commit(); err != nil {
		t.Fatal(err)
	}
	<-done
	if _, ok := ds.customerByID("cust-001"); !ok {
		t.Fatal("cust-001 not persisted once unblocked")
	}
}

// TestAddCustomersBatchParallelMatchesBook: customers added by concurrent
// workers are each persisted once, and the in-memory book agrees with the
// funding payments in the database.
// Set GOBANK_PG_DSN to run.
func TestAddCustomersBatchParallelMatchesBook(t *testing.T) {
	dsn := os.Getenv("GOBANK_PG_DSN")
	if dsn == "" {
		t.Skip("GOBANK_PG_DSN not set")
	}
	ds := NewDemoStateWithDSN(dsn)
	defer ds.db.Close()
	if ds.dbWriters() < 2 {
		t.Skip("needs more than one CPU")
	}
	const n = 300
	ds.AddCustomersBatch(n)
	deadline := time.Now().Add(60 * time.Second)
	for ds.IsAddingCustomers() {
		if time.Now().After(deadline) {
			t.Fatal("batch did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := ds.customerCount(); got != n {
		t.Fatalf("customers in database %d, want %d", got, n)
	}
	book := ds.position()
	if book.Customers != n {
		t.Errorf("customers on the position %d, want %d", book.Customers, n)
	}
	var deposits, loans int64
	if err := ds.db.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN type = $1 THEN amount END), 0),
		COALESCE(SUM(CASE WHEN type = $2 THEN amount END), 0) FROM contract_payments`,
		int(payments.Deposit), int(payments.LoanDisbursement)).Scan(&deposits, &loans); err != nil {
		t.Fatal(err)
	}
	if int64(book.Savings) != deposits || int64(book.Lending) != loans {
		t.Errorf("book savings=%d lending=%d, payments deposits=%d loans=%d",
			book.Savings, book.Lending, deposits, loans)
	}
}
