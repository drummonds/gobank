package main

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	customers "git.bytestone.uk/hum3/gobanks-customers"
)

// TestCreateCustomerRefusedLeavesBookUnchanged: a customer the database
// refuses leaves no trace — not on the books, not in the tx log, no payments —
// and the next customer is created normally.
func TestCreateCustomerRefusedLeavesBookUnchanged(t *testing.T) {
	ds := NewDemoState()
	defer ds.db.Close()
	// Occupy cust-001 so persisting the first generated customer fails.
	if err := ds.custStore.Create(context.Background(),
		customers.CustomerRecord{ID: "cust-001", Ref: "cust-001", JoinDate: ds.currentDay},
		customers.PIIInput{Name: "Squatter"}); err != nil {
		t.Fatal(err)
	}

	ds.createCustomer()

	ds.mu.Lock()
	n, book := ds.nCustomers, ds.book
	ds.mu.Unlock()
	if n != 0 || book != (bookTotals{}) {
		t.Fatalf("refused customer left nCustomers=%d book=%+v", n, book)
	}
	if p := ds.paymentCount(); p != 0 {
		t.Fatalf("refused customer left %d payments", p)
	}

	ds.createCustomer()
	if _, ok := ds.customerByID("cust-002"); !ok {
		t.Fatal("cust-002 not created after a refused customer")
	}
}

// TestCreateCustomerReleasesLockDuringPersist: ds.mu is free while a
// customer's database writes are in flight, so the day loop, dashboard and
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

	deadline := time.Now().Add(2 * time.Second)
	for !ds.mu.TryLock() {
		if time.Now().After(deadline) {
			_ = btx.Rollback()
			t.Fatal("ds.mu held while the customer's writes are blocked")
		}
		time.Sleep(5 * time.Millisecond)
	}
	ds.mu.Unlock()
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
	ds.mu.Lock()
	nCust, book := ds.nCustomers, ds.book
	ds.mu.Unlock()
	if nCust != n {
		t.Errorf("nCustomers %d, want %d", nCust, n)
	}
	var deposits, loans int64
	if err := ds.db.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN type = $1 THEN amount END), 0),
		COALESCE(SUM(CASE WHEN type = $2 THEN amount END), 0) FROM contract_payments`,
		int(PayDeposit), int(PayLoanDisbursement)).Scan(&deposits, &loans); err != nil {
		t.Fatal(err)
	}
	if int64(book.Savings) != deposits || int64(book.Lending) != loans {
		t.Errorf("book savings=%d lending=%d, payments deposits=%d loans=%d",
			book.Savings, book.Lending, deposits, loans)
	}
}
