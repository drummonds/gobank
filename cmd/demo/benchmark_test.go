package main

import (
	"fmt"
	"testing"
	"time"
)

var benchAccountCounts = []int{1_000, 10_000, 100_000, 1_000_000}

// benchAddCustomers adds n customers synchronously. Must not hold ds.mu.
func benchAddCustomers(ds *DemoState, n int) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	for i := 0; i < n; i++ {
		cust, pii := generateCustomer(ds.rng, ds.nextCustSeq, ds.products, ds.currentDay)
		ds.nextCustSeq++
		_ = ds.piiStore.Store(cust.ID, pii)
		ds.customers = append(ds.customers, cust)
		ds.addCustomerToLedger(&ds.customers[len(ds.customers)-1])
		ds.fundCustomer(len(ds.customers) - 1)
	}
}

// benchSimulateYear advances 365 days, clearing txLog each day to bound memory.
func benchSimulateYear(ds *DemoState) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	for d := 0; d < 365; d++ {
		ds.advanceDay()
		ds.txLog = ds.txLog[:0]
	}
}

// newBenchState creates a DemoState configured for benchmarking with n accounts max.
func newBenchState(n int) *DemoState {
	ds := NewDemoState()
	ds.settings.MaxCustomers = n
	return ds
}

// BenchmarkCreateAccounts measures account creation time at various scales.
func BenchmarkCreateAccounts(b *testing.B) {
	for _, n := range benchAccountCounts {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			for range b.N {
				b.StopTimer()
				ds := newBenchState(n)
				b.StartTimer()

				benchAddCustomers(ds, n)

				b.StopTimer()
				ds.db.Close()
			}
		})
	}
}

// BenchmarkSimulateYear measures 365-day simulation with pre-created accounts.
func BenchmarkSimulateYear(b *testing.B) {
	for _, n := range benchAccountCounts {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			for range b.N {
				b.StopTimer()
				ds := newBenchState(n)
				benchAddCustomers(ds, n)
				ds.mu.Lock()
				ds.txLog = ds.txLog[:0]
				ds.mu.Unlock()
				b.StartTimer()

				benchSimulateYear(ds)

				b.StopTimer()
				ds.db.Close()
			}
		})
	}
}

// BenchmarkFullYear measures account creation + 365-day simulation combined.
// Reports create-ms and sim-ms as custom metrics.
func BenchmarkFullYear(b *testing.B) {
	for _, n := range benchAccountCounts {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			for range b.N {
				ds := newBenchState(n)

				createStart := time.Now()
				benchAddCustomers(ds, n)
				createDur := time.Since(createStart)

				ds.mu.Lock()
				ds.txLog = ds.txLog[:0]
				ds.mu.Unlock()

				simStart := time.Now()
				benchSimulateYear(ds)
				simDur := time.Since(simStart)

				b.ReportMetric(float64(createDur.Milliseconds()), "create-ms")
				b.ReportMetric(float64(simDur.Milliseconds()), "sim-ms")

				ds.db.Close()
			}
		})
	}
}
