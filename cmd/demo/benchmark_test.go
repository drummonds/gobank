package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

var benchDayCounts = []int{7, 30, 60, 180, 365}
var benchCustomerCounts = []int{1, 10, 100}
var benchAccountCounts = []int{1_000, 10_000, 100_000, 1_000_000}

// --- helpers ---

// benchAddCustomers adds n customers via the real generateCustomer pipeline.
// Must not hold ds.mu.
func benchAddCustomers(ds *DemoState, n int) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	for range n {
		cust, pii := generateCustomer(ds.rng, ds.nextCustSeq, ds.products, ds.currentDay)
		ds.nextCustSeq++
		_ = ds.piiStore.Store(cust.ID, pii)
		ds.customers = append(ds.customers, cust)
		ds.addCustomerToLedger(&ds.customers[len(ds.customers)-1])
		ds.fundCustomer(len(ds.customers) - 1)
	}
}

// benchCountAccounts returns total accounts across all customers.
func benchCountAccounts(ds *DemoState) int {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	n := 0
	for _, c := range ds.customers {
		n += len(c.Accounts)
	}
	return n
}

// benchSimulateDays advances n days, clearing txLog each day to bound memory.
func benchSimulateDays(ds *DemoState, n int) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	for range n {
		ds.advanceDay()
		ds.txLog = ds.txLog[:0]
	}
}

// benchSimulateYear advances 365 days.
func benchSimulateYear(ds *DemoState) {
	benchSimulateDays(ds, 365)
}

func newBenchState(n int) *DemoState {
	ds := NewDemoState()
	ds.settings.MaxCustomers = n
	return ds
}

func newBenchStateWithDSN(maxCust int, dsn string) *DemoState {
	ds := NewDemoStateWithDSN(dsn)
	ds.settings.MaxCustomers = maxCust
	return ds
}

// dropAllPublicTables removes all tables from a postgres database.
func dropAllPublicTables(db *sql.DB) {
	rows, err := db.Query("SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
	if err != nil {
		return
	}
	var tables []string
	for rows.Next() {
		var t string
		rows.Scan(&t)
		tables = append(tables, t)
	}
	rows.Close()
	for _, t := range tables {
		db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %q CASCADE", t))
	}
}

// pgDSN returns the postgres DSN from env, or empty string if unavailable.
func pgDSN() string {
	return os.Getenv("GOBANK_PG_DSN")
}

// --- Day-scaling benchmarks ---

// BenchmarkDayScale measures per-day cost as simulation length and customer count increase.
// Uses real generateCustomer pipeline (1-3 accounts per customer).
// Runs on pglike and optionally postgres (set GOBANK_PG_DSN).
func BenchmarkDayScale(b *testing.B) {
	type backend struct {
		name string
		dsn  string
	}
	backends := []backend{{"pglike", ""}}
	if dsn := pgDSN(); dsn != "" {
		backends = append(backends, backend{"postgres", dsn})
	}

	for _, be := range backends {
		for _, nCust := range benchCustomerCounts {
			for _, days := range benchDayCounts {
				name := fmt.Sprintf("%s/c%d/d%d", be.name, nCust, days)
				be, nCust, days := be, nCust, days // capture
				b.Run(name, func(b *testing.B) {
					var cleanDB *sql.DB
					if be.dsn != "" {
						var err error
						cleanDB, err = sql.Open("pgx", be.dsn)
						if err != nil {
							b.Fatalf("postgres connect: %v", err)
						}
						defer cleanDB.Close()
					}

					for range b.N {
						b.StopTimer()
						if cleanDB != nil {
							dropAllPublicTables(cleanDB)
						}
						ds := newBenchStateWithDSN(nCust, be.dsn)
						benchAddCustomers(ds, nCust)
						nAcct := benchCountAccounts(ds)
						ds.mu.Lock()
						ds.txLog = ds.txLog[:0]
						ds.mu.Unlock()
						b.StartTimer()

						start := time.Now()
						benchSimulateDays(ds, days)
						dur := time.Since(start)

						b.StopTimer()
						acctDays := float64(nAcct) * float64(days)
						b.ReportMetric(float64(dur.Milliseconds()), "total-ms")
						b.ReportMetric(float64(dur.Microseconds())/float64(days), "us/day")
						b.ReportMetric(acctDays/dur.Seconds(), "acct-days/sec")
						b.ReportMetric(float64(nAcct), "accounts")
						ds.db.Close()
					}
				})
			}
		}
	}
}

// --- Baseline benchmarks (quick, fixed 60 days) ---

// BenchmarkBaseline measures 1 customer, real pipeline, 60 days (pglike, direct call).
func BenchmarkBaseline(b *testing.B) {
	for range b.N {
		ds := newBenchState(1)
		benchAddCustomers(ds, 1)
		nAcct := benchCountAccounts(ds)
		ds.mu.Lock()
		ds.txLog = ds.txLog[:0]
		ds.mu.Unlock()

		start := time.Now()
		benchSimulateDays(ds, 60)
		dur := time.Since(start)

		b.ReportMetric(float64(dur.Milliseconds()), "total-ms")
		b.ReportMetric(float64(dur.Microseconds())/60.0, "us/day")
		b.ReportMetric(float64(nAcct), "accounts")
		ds.db.Close()
	}
}

// BenchmarkBaselineHTTP measures 60 days advanced via HTTP POST /advance.
func BenchmarkBaselineHTTP(b *testing.B) {
	for range b.N {
		b.StopTimer()
		ds := newBenchState(1)
		benchAddCustomers(ds, 1)
		nAcct := benchCountAccounts(ds)
		ds.mu.Lock()
		ds.txLog = ds.txLog[:0]
		ds.mu.Unlock()

		mux := http.NewServeMux()
		mux.HandleFunc("POST /advance", func(w http.ResponseWriter, r *http.Request) {
			ds.AdvanceDay()
		})
		ts := httptest.NewServer(mux)
		client := ts.Client()
		b.StartTimer()

		start := time.Now()
		for range 60 {
			req, _ := http.NewRequest("POST", ts.URL+"/advance", nil)
			resp, err := client.Do(req)
			if err != nil {
				b.Fatal(err)
			}
			resp.Body.Close()
		}
		dur := time.Since(start)

		b.StopTimer()
		b.ReportMetric(float64(dur.Milliseconds()), "total-ms")
		b.ReportMetric(float64(dur.Microseconds())/60.0, "us/day")
		b.ReportMetric(float64(nAcct), "accounts")
		ts.Close()
		ds.db.Close()
	}
}

// BenchmarkBaselineDashboardRender measures rendering the dashboard HTML after 60 days.
func BenchmarkBaselineDashboardRender(b *testing.B) {
	ds := newBenchState(1)
	benchAddCustomers(ds, 1)
	benchSimulateDays(ds, 60)

	b.ResetTimer()
	for b.Loop() {
		_ = ds.BuildDashboardHTML()
	}
	b.StopTimer()
	ds.db.Close()
}

// --- Scale benchmarks (existing, larger account counts) ---

// BenchmarkCreateAccounts measures account creation time at various scales.
func BenchmarkCreateAccounts(b *testing.B) {
	for _, n := range benchAccountCounts {
		b.Run(fmt.Sprintf("%d", n), func(b *testing.B) {
			for range b.N {
				b.StopTimer()
				ds := newBenchState(n)
				b.StartTimer()

				start := time.Now()
				benchAddCustomers(ds, n)
				dur := time.Since(start)

				b.StopTimer()
				b.ReportMetric(float64(n)/dur.Seconds(), "accounts/sec")
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

				start := time.Now()
				benchSimulateYear(ds)
				dur := time.Since(start)

				b.StopTimer()
				accountDays := float64(n) * 365
				b.ReportMetric(accountDays/dur.Seconds(), "account-days/sec")
				b.ReportMetric(float64(n)/dur.Seconds(), "eod-accounts/sec")
				ds.db.Close()
			}
		})
	}
}

// BenchmarkFullYear measures account creation + 365-day simulation combined.
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
				b.ReportMetric(float64(n)/createDur.Seconds(), "accounts/sec")
				accountDays := float64(n) * 365
				b.ReportMetric(accountDays/simDur.Seconds(), "account-days/sec")

				ds.db.Close()
			}
		})
	}
}
