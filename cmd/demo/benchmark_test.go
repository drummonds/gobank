package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
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
	for range n {
		ds.advanceDay()
		ds.mu.Lock()
		ds.txLog = ds.txLog[:0]
		ds.mu.Unlock()
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

// --- API load benchmarks ---

// benchBuildMux creates a realistic HTTP mux for load testing against ds.
// Includes a mix of JSON API, HTML renders, and write endpoints.
func benchBuildMux(ds *DemoState) *http.ServeMux {
	var renderMu sync.Mutex
	mux := http.NewServeMux()

	// JSON API — lightweight reads
	mux.HandleFunc("GET /api/customers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ds.bankAppCustomerList())
	})
	mux.HandleFunc("GET /api/customer/{id}/accounts", func(w http.ResponseWriter, r *http.Request) {
		resp := ds.bankAppAccounts(r.PathValue("id"))
		w.Header().Set("Content-Type", "application/json")
		if resp == nil {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("GET /api/customer/{id}/transactions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ds.bankAppTransactions(r.PathValue("id"), 1))
	})

	// HTML renders — heavier, hold renderMu
	mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		renderMu.Lock()
		html := ds.BuildDashboardHTML()
		renderMu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, html)
	})
	mux.HandleFunc("GET /accounting/pnl", func(w http.ResponseWriter, r *http.Request) {
		renderMu.Lock()
		html := ds.BuildPnLHTML()
		renderMu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, html)
	})
	mux.HandleFunc("GET /customers", func(w http.ResponseWriter, r *http.Request) {
		renderMu.Lock()
		html := ds.BuildCustomersHTML(1)
		renderMu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, html)
	})

	// Write endpoints — mutate state under lock
	mux.HandleFunc("POST /advance", func(w http.ResponseWriter, r *http.Request) {
		ds.AdvanceDay()
	})
	mux.HandleFunc("POST /payments/send", func(w http.ResponseWriter, r *http.Request) {
		ds.SendPayment()
	})

	return mux
}

// benchFirstCustomerID returns the ID of the first customer (for API calls).
func benchFirstCustomerID(ds *DemoState) string {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if len(ds.customers) == 0 {
		return ""
	}
	return ds.customers[0].ID
}

// benchLoadEndpoints returns endpoint lists for load testing.
type benchEndpoint struct {
	method string
	path   string
}

func benchReadEndpoints(custID string) []benchEndpoint {
	return []benchEndpoint{
		{"GET", "/api/customers"},
		{"GET", "/api/customer/" + custID + "/accounts"},
		{"GET", "/api/customer/" + custID + "/transactions"},
		{"GET", "/api/customers"},
		{"GET", "/api/customer/" + custID + "/accounts"},
		{"GET", "/api/customer/" + custID + "/transactions"},
		{"GET", "/dashboard"},
		{"GET", "/accounting/pnl"},
		{"GET", "/customers"},
		{"GET", "/api/customer/" + custID + "/accounts"},
	}
}

func benchMixedEndpoints(custID string) []benchEndpoint {
	// 70% reads, 20% light writes, 10% heavy writes.
	// In production, advance fires at most once per 200ms from the auto-play
	// ticker, while API reads happen on every page view.
	return []benchEndpoint{
		{"GET", "/api/customers"},
		{"GET", "/api/customer/" + custID + "/accounts"},
		{"GET", "/api/customer/" + custID + "/transactions"},
		{"GET", "/dashboard"},
		{"GET", "/accounting/pnl"},
		{"GET", "/customers"},
		{"GET", "/api/customer/" + custID + "/accounts"},
		{"POST", "/payments/send"},
		{"POST", "/payments/send"},
		{"POST", "/advance"},
	}
}

// benchRunLoad drives concurrent HTTP load for a fixed duration and reports metrics.
func benchRunLoad(b *testing.B, ts *httptest.Server, endpoints []benchEndpoint, conc int, duration time.Duration, nAcct int) {
	var totalReqs atomic.Int64
	var totalErrors atomic.Int64
	var totalLatencyNs atomic.Int64

	b.ResetTimer()
	start := time.Now()
	deadline := start.Add(duration)

	var wg sync.WaitGroup
	for w := range conc {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := ts.Client()
			i := workerID * 7 // offset so workers hit different endpoints
			for time.Now().Before(deadline) {
				ep := endpoints[i%len(endpoints)]
				i++
				t0 := time.Now()
				req, _ := http.NewRequest(ep.method, ts.URL+ep.path, nil)
				resp, err := client.Do(req)
				latency := time.Since(t0)
				if err != nil {
					totalErrors.Add(1)
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if resp.StatusCode >= 500 {
					totalErrors.Add(1)
				}
				totalReqs.Add(1)
				totalLatencyNs.Add(int64(latency))
			}
		}(w)
	}
	wg.Wait()

	dur := time.Since(start)
	b.StopTimer()

	reqs := totalReqs.Load()
	errs := totalErrors.Load()
	reqPerSec := float64(reqs) / dur.Seconds()
	avgLatencyMs := 0.0
	if reqs > 0 {
		avgLatencyMs = float64(totalLatencyNs.Load()) / float64(reqs) / 1e6
	}

	b.ReportMetric(reqPerSec, "req/sec")
	b.ReportMetric(avgLatencyMs, "avg-ms")
	b.ReportMetric(float64(reqs), "total-reqs")
	b.ReportMetric(float64(errs), "errors")
	b.ReportMetric(float64(nAcct), "accounts")
	b.ReportMetric(float64(conc), "goroutines")
	if errs > 0 {
		b.Logf("WARNING: %d errors out of %d requests (%.1f%%)",
			errs, reqs+errs, float64(errs)/float64(reqs+errs)*100)
	}
}

// BenchmarkAPILoad measures API throughput under concurrent load.
// Sets up 1000 customers + 10 days, then drives workloads at increasing
// concurrency. Two sub-suites: "read" (reads only, no mutex contention)
// and "mixed" (70% reads, 20% light writes, 10% advance).
func BenchmarkAPILoad(b *testing.B) {
	const loadDuration = 3 * time.Second
	concurrencyLevels := []int{1, 2, 4, 8, 16, 32, 64}

	type workload struct {
		name      string
		endpoints func(custID string) []benchEndpoint
	}
	workloads := []workload{
		{"read", benchReadEndpoints},
		{"mixed", benchMixedEndpoints},
	}

	for _, wl := range workloads {
		for _, conc := range concurrencyLevels {
			name := fmt.Sprintf("%s/c%d", wl.name, conc)
			wl, conc := wl, conc
			b.Run(name, func(b *testing.B) {
				ds := newBenchState(1000)
				benchAddCustomers(ds, 1000)
				benchSimulateDays(ds, 10)
				nAcct := benchCountAccounts(ds)
				custID := benchFirstCustomerID(ds)

				mux := benchBuildMux(ds)
				ts := httptest.NewServer(mux)
				defer ts.Close()
				defer ds.db.Close()

				endpoints := wl.endpoints(custID)
				benchRunLoad(b, ts, endpoints, conc, loadDuration, nAcct)
			})
		}
	}
}

// BenchmarkAPIEndpoint measures individual endpoint throughput (serial, no contention).
// Useful for identifying which endpoints are slowest.
func BenchmarkAPIEndpoint(b *testing.B) {
	ds := newBenchState(1000)
	benchAddCustomers(ds, 1000)
	benchSimulateDays(ds, 10)
	custID := benchFirstCustomerID(ds)

	mux := benchBuildMux(ds)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	defer ds.db.Close()

	endpoints := []struct {
		name   string
		method string
		path   string
	}{
		{"api/customers", "GET", "/api/customers"},
		{"api/accounts", "GET", "/api/customer/" + custID + "/accounts"},
		{"api/transactions", "GET", "/api/customer/" + custID + "/transactions"},
		{"dashboard", "GET", "/dashboard"},
		{"pnl", "GET", "/accounting/pnl"},
		{"customers", "GET", "/customers"},
		{"advance", "POST", "/advance"},
		{"payments/send", "POST", "/payments/send"},
	}

	client := ts.Client()
	for _, ep := range endpoints {
		ep := ep
		b.Run(ep.name, func(b *testing.B) {
			for b.Loop() {
				req, _ := http.NewRequest(ep.method, ts.URL+ep.path, nil)
				resp, err := client.Do(req)
				if err != nil {
					b.Fatal(err)
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		})
	}
}
