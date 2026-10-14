package routestats

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func serve(h http.Handler, method, path string) int {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(method, path, nil))
	return rr.Code
}

func find(t *testing.T, stats []RouteStat, pattern string) RouteStat {
	t.Helper()
	for _, s := range stats {
		if s.Pattern == pattern {
			return s
		}
	}
	t.Fatalf("no stats for %q in %+v", pattern, stats)
	return RouteStat{}
}

// Every request a mux serves is counted against the pattern it matched,
// split into method and path; a request no pattern matches is nobody's.
func TestWrapCountsEachRouteByItsPattern(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/customers/", func(w http.ResponseWriter, r *http.Request) {})
	st := New()
	h := st.Wrap(mux)
	for range 3 {
		serve(h, "GET", "/v1/health")
	}
	serve(h, "GET", "/customers/cust-001")
	if code := serve(h, "GET", "/nowhere"); code != http.StatusNotFound {
		t.Fatalf("unmatched: %d", code)
	}
	snap := st.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("snapshot = %+v; want the two routes served", snap)
	}
	health := find(t, snap, "GET /v1/health")
	if health.Method != "GET" || health.Path != "/v1/health" || health.Count != 3 {
		t.Errorf("health = %+v", health)
	}
	customers := find(t, snap, "/customers/")
	if customers.Method != "" || customers.Path != "/customers/" || customers.Count != 1 {
		t.Errorf("customers = %+v", customers)
	}
}

// The latency figures are over the last 1,024 requests of the route,
// the maximum over all of them, and the rate is the requests completed
// in the last minute.
func TestRecordKeepsPercentilesMaxAndTheMinuteRate(t *testing.T) {
	now := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	st := New()
	st.Now = func() time.Time { return now }
	for i := 1; i <= 100; i++ {
		st.Record("GET /x", time.Duration(i)*time.Millisecond)
	}
	s := find(t, st.Snapshot(), "GET /x")
	if s.Count != 100 || s.P50 != 50*time.Millisecond || s.P95 != 95*time.Millisecond || s.Max != 100*time.Millisecond || s.Last != 100*time.Millisecond {
		t.Errorf("after 100: %+v", s)
	}
	if s.PerSecond < 1.6 || s.PerSecond > 1.7 {
		t.Errorf("rate = %.2f/s; want 100 in the last minute, 1.67/s", s.PerSecond)
	}

	// A minute on, the window is empty but the figures remain.
	now = now.Add(2 * time.Minute)
	if s = find(t, st.Snapshot(), "GET /x"); s.PerSecond != 0 || s.Count != 100 || s.P95 != 95*time.Millisecond {
		t.Errorf("two minutes on: %+v", s)
	}

	// Beyond the window, the oldest requests leave the percentiles but
	// not the maximum.
	for i := 1; i <= 2000; i++ {
		st.Record("GET /x", time.Duration(i)*time.Millisecond)
	}
	s = find(t, st.Snapshot(), "GET /x")
	if s.Count != 2100 || s.Max != 2000*time.Millisecond || s.P50 < 1400*time.Millisecond || s.P50 > 1600*time.Millisecond {
		t.Errorf("after 2100: %+v; want p50 over the last 1,024 only", s)
	}
}

// Requests in flight are counted per route as they run, and the peak is
// kept: how parallel the route has been asked to be.
func TestWrapKeepsThePeakInFlight(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
	})
	st := New()
	h := st.Wrap(mux)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { serve(h, "GET", "/slow") })
	}
	for range 4 {
		<-entered
	}
	if s := find(t, st.Snapshot(), "GET /slow"); s.InFlight != 4 || s.PeakInFlight != 4 || s.Count != 0 {
		t.Errorf("while running: %+v; want 4 in flight, none complete", s)
	}
	close(release)
	wg.Wait()
	if s := find(t, st.Snapshot(), "GET /slow"); s.InFlight != 0 || s.PeakInFlight != 4 || s.Count != 4 {
		t.Errorf("after: %+v; want none in flight, peak 4, 4 complete", s)
	}
}

// A nil Stats wraps nothing: the mux is served as it is.
func TestNilStatsServesTheMuxAsItIs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {})
	var st *Stats
	if code := serve(st.Wrap(mux), "GET", "/v1/health"); code != http.StatusOK {
		t.Errorf("nil stats: %d", code)
	}
	if snap := st.Snapshot(); snap != nil {
		t.Errorf("nil stats snapshot = %+v", snap)
	}
}
