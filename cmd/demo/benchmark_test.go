package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	for range n {
		ds.createCustomer()
	}
}

// benchCountAccounts returns total accounts across all customers.
func benchCountAccounts(ds *DemoState) int {
	var n int
	if err := ds.db.QueryRow(`SELECT COUNT(*) FROM customer_accounts`).Scan(&n); err != nil {
		panic(err)
	}
	return n
}

// benchSimulateDays advances n days.
func benchSimulateDays(ds *DemoState, n int) {
	for range n {
		ds.advanceDay()
	}
}

// benchSimulateYear advances 365 days.
func benchSimulateYear(ds *DemoState) {
	benchSimulateDays(ds, 365)
}

func newBenchState(n int) *DemoState {
	ds := NewDemoState()
	ds.sim.SetMaxCustomers(n)
	return ds
}

func newBenchStateWithDSN(maxCust int, dsn string) *DemoState {
	ds := NewDemoStateWithDSN(dsn)
	ds.sim.SetMaxCustomers(maxCust)
	return ds
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
		_ = buildDashboardHTML(ds.Bank, ds)
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

	// The customer BFF — lightweight reads of one customer's screens
	mux.Handle("/v1/", newAppBFF(ds.Bank, newAppLogin(ds.Bank, benchAppPassword), ds.DB, "/", "", slog.New(slog.NewTextHandler(io.Discard, nil))))

	// HTML renders — heavier, hold renderMu
	mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		renderMu.Lock()
		html := buildDashboardHTML(ds.Bank, ds)
		renderMu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, html)
	})
	mux.HandleFunc("GET /accounting/pnl", func(w http.ResponseWriter, r *http.Request) {
		renderMu.Lock()
		html := buildPnLHTML(ds.Bank)
		renderMu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, html)
	})
	mux.HandleFunc("GET /customers", func(w http.ResponseWriter, r *http.Request) {
		renderMu.Lock()
		html := buildCustomersHTML(ds.Bank, 1, false)
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

// benchAppPassword is the app password the benchmark's BFF accepts.
const benchAppPassword = "bench"

// benchLogin logs the customer in to the BFF and returns the bearer token
// the customer's reads carry.
func benchLogin(b *testing.B, ts *httptest.Server, custID string) string {
	body := fmt.Sprintf(`{"customer_id":%q,"password":%q}`, custID, benchAppPassword)
	resp, err := ts.Client().Post(ts.URL+"/v1/login", "application/json", strings.NewReader(body))
	if err != nil {
		b.Fatal(err)
	}
	defer resp.Body.Close()
	var m struct{ Token string }
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil || m.Token == "" {
		b.Fatalf("login: %d %v", resp.StatusCode, err)
	}
	return m.Token
}

// benchFirstCustomerID returns the ID of the first customer (for API calls).
func benchFirstCustomerID(ds *DemoState) string {
	page, _ := ds.customerPage(1)
	if len(page) == 0 {
		return ""
	}
	return page[0].ID
}

// benchLoadEndpoints returns endpoint lists for load testing.
type benchEndpoint struct {
	method string
	path   string
}

func benchReadEndpoints(custID string) []benchEndpoint {
	return []benchEndpoint{
		{"GET", "/v1/screen/accounts"},
		{"GET", "/v1/screen/activity"},
		{"GET", "/v1/screen/product/0"},
		{"GET", "/v1/screen/accounts"},
		{"GET", "/v1/screen/activity"},
		{"GET", "/v1/screen/product/0"},
		{"GET", "/dashboard"},
		{"GET", "/accounting/pnl"},
		{"GET", "/customers"},
		{"GET", "/v1/screen/accounts"},
	}
}

func benchMixedEndpoints(custID string) []benchEndpoint {
	// 70% reads, 20% light writes, 10% heavy writes.
	// In production, advance fires at most once per 200ms from the auto-play
	// ticker, while the app's reads happen on every screen.
	return []benchEndpoint{
		{"GET", "/v1/screen/accounts"},
		{"GET", "/v1/screen/activity"},
		{"GET", "/v1/screen/product/0"},
		{"GET", "/dashboard"},
		{"GET", "/accounting/pnl"},
		{"GET", "/customers"},
		{"GET", "/v1/screen/accounts"},
		{"POST", "/payments/send"},
		{"POST", "/payments/send"},
		{"POST", "/advance"},
	}
}

// benchRunLoad drives concurrent HTTP load for a fixed duration and reports metrics.
func benchRunLoad(b *testing.B, ts *httptest.Server, endpoints []benchEndpoint, token string, conc int, duration time.Duration, nAcct int) {
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
				req.Header.Set("Authorization", "Bearer "+token)
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
				benchRunLoad(b, ts, endpoints, benchLogin(b, ts, custID), conc, loadDuration, nAcct)
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
		{"app/accounts", "GET", "/v1/screen/accounts"},
		{"app/activity", "GET", "/v1/screen/activity"},
		{"app/product", "GET", "/v1/screen/product/0"},
		{"dashboard", "GET", "/dashboard"},
		{"pnl", "GET", "/accounting/pnl"},
		{"customers", "GET", "/customers"},
		{"advance", "POST", "/advance"},
		{"payments/send", "POST", "/payments/send"},
	}

	client := ts.Client()
	token := benchLogin(b, ts, custID)
	for _, ep := range endpoints {
		b.Run(ep.name, func(b *testing.B) {
			for b.Loop() {
				req, _ := http.NewRequest(ep.method, ts.URL+ep.path, nil)
				req.Header.Set("Authorization", "Bearer "+token)
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
