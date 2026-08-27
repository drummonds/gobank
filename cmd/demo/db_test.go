package main

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"

	_ "git.bytestone.uk/hum3/go-postgres"
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
	for range 10 {
		wg.Go(func() {
			var name string
			if err := db.QueryRow(`SELECT name FROM test_accounts WHERE id = 'a1'`).Scan(&name); err != nil {
				errs <- err
				return
			}
			if name != "Alice" {
				errs <- fmt.Errorf("expected Alice, got %s", name)
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent read: %v", err)
	}
}

// TestLedgerSurvivesConnectionPool creates a DemoState (which sets up the
// go-luca ledger) and adds enough customers to exercise connection pooling.
// Without the go-postgres shared in-memory DB fix (landed in v0.4.1;
// current pin v0.5.2), this fails around customer 188.
func TestLedgerSurvivesConnectionPool(t *testing.T) {
	ds := NewDemoState()
	defer ds.db.Close()

	// Add 200 customers — must not hit "no such table: accounts".
	ds.mu.Lock()
	defer ds.mu.Unlock()
	for range 200 {
		ds.createCustomerLocked()
	}

	if len(ds.customers) != 200 {
		t.Fatalf("expected 200 customers, got %d", len(ds.customers))
	}
}
