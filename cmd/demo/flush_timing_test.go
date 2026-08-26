package main

import (
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// TestFlushTiming measures the cost of one advanceDay at scale, split into the
// interest/accrual phase and the ledger flush (which holds ds.mu).
// Run explicitly: FLUSH_TIMING=1 go test -run TestFlushTiming -v
func TestFlushTiming(t *testing.T) {
	if os.Getenv("FLUSH_TIMING") == "" {
		t.Skip("FLUSH_TIMING not set")
	}
	for _, n := range []int{100, 1000, 5000} {
		ds := newBenchState(n)
		benchAddCustomers(ds, n)
		accounts := benchCountAccounts(ds)

		// Warm one day so ledger paths are initialized.
		ds.advanceDay()

		// Measure a full day, instrumenting the flush by re-running it manually:
		// run interest phase by calling advanceDay with flush observed via a
		// pre/post movement count is not accessible, so time the whole day and
		// then a no-ledger day for comparison.
		start := time.Now()
		ds.advanceDay()
		fullDay := time.Since(start)

		// Disable ledger to skip movement queue + flush entirely.
		ds.mu.Lock()
		sim := ds.sim
		ds.sim = nil
		ds.mu.Unlock()
		start = time.Now()
		ds.advanceDay()
		noLedgerDay := time.Since(start)
		ds.mu.Lock()
		ds.sim = sim
		ds.mu.Unlock()

		// Measure worst-case reader lock wait while a full day advances:
		// a sampler goroutine repeatedly acquires ds.mu like a dashboard read.
		var maxWait time.Duration
		var stop atomic.Bool
		done := make(chan struct{})
		go func() {
			defer close(done)
			for !stop.Load() {
				s := time.Now()
				ds.mu.Lock()
				ds.mu.Unlock() //nolint:staticcheck // empty critical section is the point
				if w := time.Since(s); w > maxWait {
					maxWait = w
				}
				time.Sleep(time.Millisecond)
			}
		}()
		ds.advanceDay()
		stop.Store(true)
		<-done

		t.Logf("customers=%d accounts=%d fullDay=%v noLedgerDay=%v ledgerOverhead=%v maxReaderLockWait=%v",
			n, accounts, fullDay, noLedgerDay, fullDay-noLedgerDay, maxWait)
	}
}
