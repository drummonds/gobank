// Package routestats counts and times the requests a mux serves, route by
// route, for the API pages and about.json to show: how many, how long
// (the last, and the p50 and p95 of the last 1,024; the maximum ever),
// how many at once, and how many a second over the last minute. The
// figures live in the process and start again with it.
package routestats

import (
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// Window is how many of a route's latest requests the percentiles are
// over.
const Window = 1024

// Stats is the figures of every route a wrapped mux has served.
type Stats struct {
	Now func() time.Time // nil is time.Now

	mu     sync.Mutex
	routes map[string]*route
}

type sample struct {
	at   time.Time // completed
	took time.Duration
}

type route struct {
	count          int64
	inFlight, peak int
	last, max      time.Duration
	ring           [Window]sample
	n, next        int // samples held, and where the next goes
}

// RouteStat is one route's figures at a moment.
type RouteStat struct {
	Pattern      string // as registered: "GET /v1/health", "/customers/"
	Method, Path string // the pattern split; Method "" when it has none
	Count        int64  // requests completed since the process started
	InFlight     int    // requests being served now
	PeakInFlight int    // the most served at once
	Last         time.Duration
	P50, P95     time.Duration // over the last Window requests
	Max          time.Duration // over every request
	PerSecond    float64       // completed in the last minute, a second
}

// New is an empty Stats.
func New() *Stats { return &Stats{routes: map[string]*route{}} }

// Mux is a mux that can say which pattern a request matches before it
// serves it: http.ServeMux and what embeds it.
type Mux interface {
	http.Handler
	Handler(r *http.Request) (http.Handler, string)
}

// Wrap serves mux, timing every request against the pattern it matched;
// a request no pattern matches is served and not counted. A nil Stats
// serves the mux as it is.
func (s *Stats) Wrap(mux Mux) http.Handler {
	if s == nil {
		return mux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := mux.Handler(r)
		if pattern == "" {
			mux.ServeHTTP(w, r)
			return
		}
		defer s.Observe(pattern)()
		mux.ServeHTTP(w, r)
	})
}

// Observe marks a request of the pattern as begun and returns what ends
// it: a mux that knows its pattern calls the pair around the handler
// (internal/servemux's Observe hook). A nil Stats observes nothing.
func (s *Stats) Observe(pattern string) (done func()) {
	if s == nil {
		return func() {}
	}
	started := s.now()
	s.enter(pattern)
	return func() { s.leave(pattern, s.now().Sub(started)) }
}

// Record counts one completed request of the pattern that took took.
func (s *Stats) Record(pattern string, took time.Duration) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.record(s.route(pattern), took)
}

func (s *Stats) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// route is the pattern's figures, made on first sight; s.mu held.
func (s *Stats) route(pattern string) *route {
	rt := s.routes[pattern]
	if rt == nil {
		rt = &route{}
		s.routes[pattern] = rt
	}
	return rt
}

func (s *Stats) enter(pattern string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rt := s.route(pattern)
	rt.inFlight++
	rt.peak = max(rt.peak, rt.inFlight)
}

func (s *Stats) leave(pattern string, took time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rt := s.route(pattern)
	rt.inFlight--
	s.record(rt, took)
}

// record counts a completed request; s.mu held.
func (s *Stats) record(rt *route, took time.Duration) {
	rt.count++
	rt.last = took
	rt.max = max(rt.max, took)
	rt.ring[rt.next] = sample{at: s.now(), took: took}
	rt.next = (rt.next + 1) % Window
	rt.n = min(rt.n+1, Window)
}

// Snapshot is every route's figures now, by pattern.
func (s *Stats) Snapshot() []RouteStat {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	out := make([]RouteStat, 0, len(s.routes))
	for pattern, rt := range s.routes {
		st := RouteStat{Pattern: pattern, Path: pattern, Count: rt.count, InFlight: rt.inFlight, PeakInFlight: rt.peak, Last: rt.last, Max: rt.max}
		if method, path, ok := strings.Cut(pattern, " "); ok {
			st.Method, st.Path = method, path
		}
		if rt.n > 0 {
			tooks := make([]time.Duration, 0, rt.n)
			inMinute, oldest := 0, now
			for _, smp := range rt.ring[:rt.n] {
				tooks = append(tooks, smp.took)
				if !smp.at.Before(now.Add(-time.Minute)) {
					inMinute++
				}
				if smp.at.Before(oldest) {
					oldest = smp.at
				}
			}
			slices.Sort(tooks)
			st.P50, st.P95 = percentile(tooks, 0.50), percentile(tooks, 0.95)
			if span := now.Sub(oldest); rt.n == Window && span > 0 && span < time.Minute {
				// A route served the whole window inside the minute: the
				// window is what we can see of the minute.
				st.PerSecond = float64(rt.n) / span.Seconds()
			} else {
				st.PerSecond = float64(inMinute) / 60
			}
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pattern < out[j].Pattern })
	return out
}

// percentile is the nearest-rank percentile of sorted durations.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(p*float64(len(sorted))+0.999999) - 1
	return sorted[max(0, min(rank, len(sorted)-1))]
}
