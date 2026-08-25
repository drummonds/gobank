package main

import (
	"database/sql"
	"sync"
	"testing"

	_ "git.bytestone.uk/hum3/go-postgres"
)

// TestDemoDSNSharedAcrossConnections uses the demo's ACTUAL DSN
// ("file::memory:?_pragma=temp_store(2)") rather than ":memory:".
// go-postgres only special-cases ":memory:" for pool sharing, so each pool
// connection to file::memory: gets its own private empty database. This is
// invisible while the pool holds a single connection, and surfaces as
// "no such table" the moment concurrent load opens a second one — the
// suspected wasm add-100-customers crash.
func TestDemoDSNSharedAcrossConnections(t *testing.T) {
	db, err := sql.Open("pglike", "file::memory:?_pragma=temp_store(2)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(4)

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS test_accounts (id TEXT PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO test_accounts (id, name) VALUES ('a1', 'Alice')`); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Hold one connection open so concurrent queries force a second one.
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for range 10 {
		wg.Go(func() {
			var name string
			if err := db.QueryRow(`SELECT name FROM test_accounts WHERE id = 'a1'`).Scan(&name); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent read: %v", err)
	}
}
