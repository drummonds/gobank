package main

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"

	_ "github.com/drummonds/go-postgres"
)

// TestMemoryDBSharedAcrossConnections verifies that multiple connections from
// the database/sql pool all see the same schema and data when using :memory:.
// This was the root cause of #3 (go-postgres): the accounts table vanished
// after ~188 customers because pool connections got separate empty databases.
func TestMemoryDBSharedAcrossConnections(t *testing.T) {
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Allow multiple connections — the whole point of this test.
	db.SetMaxOpenConns(4)

	// Create a table on one connection.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS test_accounts (id TEXT PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	// Insert a row.
	if _, err := db.Exec(`INSERT INTO test_accounts (id, name) VALUES ('a1', 'Alice')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Read from potentially different connections concurrently.
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var name string
			if err := db.QueryRow(`SELECT name FROM test_accounts WHERE id = 'a1'`).Scan(&name); err != nil {
				errs <- err
				return
			}
			if name != "Alice" {
				errs <- fmt.Errorf("expected Alice, got %s", name)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent read: %v", err)
	}
}

// TestLedgerSurvivesConnectionPool creates a DemoState (which sets up the
// go-luca ledger) and adds enough customers to exercise connection pooling.
// Without the go-postgres v0.4.1 fix, this fails around customer 188.
func TestLedgerSurvivesConnectionPool(t *testing.T) {
	ds := NewDemoState()
	defer ds.db.Close()

	// Add 200 customers — must not hit "no such table: accounts".
	ds.mu.Lock()
	defer ds.mu.Unlock()
	for i := 0; i < 200; i++ {
		cust, pii := generateCustomer(ds.rng, ds.nextCustSeq, ds.products, ds.currentDay)
		ds.nextCustSeq++
		_ = ds.piiStore.Store(cust.ID, pii)
		ds.customers = append(ds.customers, cust)
		ds.addCustomerToLedger(&ds.customers[len(ds.customers)-1])
		ds.fundCustomer(len(ds.customers) - 1)
	}

	if len(ds.customers) != 200 {
		t.Fatalf("expected 200 customers, got %d", len(ds.customers))
	}
}
