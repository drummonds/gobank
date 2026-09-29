package main

import (
	"testing"
	"time"
)

// TestDayWaitsForDatabaseWithoutEngineLock: when the day loop has to wait for
// the database (another writer holds pglike's single write lock, e.g. a
// customer mid-transaction), it must not hold simMu while it waits — that
// writer may itself be waiting for simMu, which deadlocks until SQLite's busy
// timeout (or for ever on the WASM shared connection).
func TestDayWaitsForDatabaseWithoutEngineLock(t *testing.T) {
	ds := NewDemoState()
	defer ds.db.Close()
	addFundedCustomer(ds)

	tx, err := ds.db.Begin() // pglike begins IMMEDIATE: takes the write lock
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE payments SET status = status`); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() { ds.advanceDay(); close(done) }()

	deadline := time.Now().Add(3 * time.Second)
	for {
		select {
		case <-done:
			_ = tx.Rollback()
			t.Fatal("day finished while the database write lock was held elsewhere")
		default:
		}
		if ds.simMu.TryLock() {
			ds.simMu.Unlock()
			// Only meaningful once the day is actually waiting on the database.
			if ds.progress.snapshot().Phase == "accrual postings" {
				break
			}
		} else if time.Now().After(deadline) {
			_ = tx.Rollback()
			t.Fatal("simMu held while the day loop waits for the database")
		}
		if time.Now().After(deadline) {
			_ = tx.Rollback()
			t.Fatalf("day never reached accrual postings (phase %q)", ds.progress.snapshot().Phase)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	<-done
}
