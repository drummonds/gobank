package main

import (
	"testing"
	"time"
)

// TestDayWaitsForDatabaseWithoutStateLock: when the day has to wait for
// the database (another writer holds pglike's single write lock, e.g. a
// customer mid-transaction), it must not hold ds.mu while it waits — the
// dashboard and the customer app read under ds.mu, and that writer may
// itself need ds.mu to finish.
func TestDayWaitsForDatabaseWithoutStateLock(t *testing.T) {
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
	for !ds.progress.snapshot().Active { // the day has begun and is at its first write
		if time.Now().After(deadline) {
			_ = tx.Rollback()
			t.Fatal("the day never began")
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond) // long enough to be blocked on the write lock
	select {
	case <-done:
		_ = tx.Rollback()
		t.Fatal("day finished while the database write lock was held elsewhere")
	default:
	}
	if !ds.mu.TryLock() {
		_ = tx.Rollback()
		t.Fatal("ds.mu held while the day waits for the database")
	}
	ds.mu.Unlock()
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	<-done
}
