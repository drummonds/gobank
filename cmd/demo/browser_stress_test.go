package main

import (
	"testing"
	"time"
)

// TestBrowserScenarioAddCustomersDuringSim replicates the browser workload
// that crashed the WASM demo: the simulation ticker running while a batch of
// 100 customers is added and dashboard/explorer reads poll concurrently.
// Run it compiled to js/wasm with an unusable TMPDIR to force go-postgres's
// single-shared-connection fallback (the browser code path); it also guards
// the temp-file path natively. Regression targets: interleaved statements
// corrupting the shared SQLite connection, and deadlocks from the
// shared-connection lock.
func TestBrowserScenarioAddCustomersDuringSim(t *testing.T) {
	ds := NewDemoState()
	defer ds.Stop()

	ds.Start()
	ds.AddCustomersBatch(100)

	stop := make(chan struct{})
	pollErrs := make(chan error, 64)
	for range 3 {
		go func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = ds.BuildDashboardHTML()
				var count int
				if err := ds.db.QueryRow(`SELECT COUNT(*) FROM movements`).Scan(&count); err != nil {
					select {
					case pollErrs <- err:
					default:
					}
				}
				time.Sleep(20 * time.Millisecond)
			}
		}()
	}

	deadline := time.After(90 * time.Second)
	for ds.IsAddingCustomers() {
		select {
		case <-deadline:
			close(stop)
			t.Fatal("batch add never completed — likely deadlock on the shared DB connection")
		case <-time.After(100 * time.Millisecond):
		}
	}
	// Let the sim tick a few more days against the full customer set.
	time.Sleep(2 * time.Second)
	close(stop)
	ds.Stop()

	select {
	case err := <-pollErrs:
		t.Fatalf("polling query failed: %v", err)
	default:
	}

	ds.mu.Lock()
	customers := len(ds.customers)
	ds.mu.Unlock()
	if customers < 100 {
		t.Errorf("only %d customers added, want >= 100", customers)
	}
	var count int
	if err := ds.db.QueryRow(`SELECT COUNT(*) FROM movements`).Scan(&count); err != nil {
		t.Fatalf("final movements query: %v", err)
	}
	if count == 0 {
		t.Error("no movements recorded")
	}
}
