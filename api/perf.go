package api

import (
	"fmt"
	"time"
)

// Perf is what the running process has measured of a route since it
// started, shown beside the route's facts: the count, the latency (p50
// and p95 of the latest requests, the maximum ever), the most served at
// once, and the rate over the last minute. The pages are given it
// through Options.Perf; the figures themselves are kept by
// internal/routestats.
type Perf struct {
	Count        int64
	PeakInFlight int
	P50, P95     time.Duration
	Max          time.Duration
	PerSecond    float64
}

// Capacity is the requests a second the route completes at its peak in
// flight and its p95 latency — concurrency over latency (Little's law) —
// so a reader sees what the route has shown it can carry; zero until it
// has been served.
func (p Perf) Capacity() float64 {
	if p.P95 <= 0 {
		return 0
	}
	return float64(p.PeakInFlight) * float64(time.Second) / float64(p.P95)
}

// Clients is how many clients, each acting once every think, the
// capacity carries.
func (p Perf) Clients(think time.Duration) float64 {
	return p.Capacity() * think.Seconds()
}

// thinkTime is how often a client is taken to act when the detail page
// turns a capacity into clients.
const thinkTime = 10 * time.Second

// perfView is a route's performance as the pages show it.
type perfView struct {
	Count         int64
	P50, P95, Max string
	PeakInFlight  int
	PerSecond     string
	Capacity      string
	Clients       string
	ThinkSeconds  int
}

func viewPerf(p Perf) *perfView {
	return &perfView{
		Count: p.Count, P50: dur(p.P50), P95: dur(p.P95), Max: dur(p.Max), PeakInFlight: p.PeakInFlight,
		PerSecond: fmt.Sprintf("%.1f", p.PerSecond), Capacity: fmt.Sprintf("%.0f", p.Capacity()),
		Clients: fmt.Sprintf("%.0f", p.Clients(thinkTime)), ThinkSeconds: int(thinkTime.Seconds()),
	}
}

// dur shows a latency to the millisecond, or finer below one.
func dur(d time.Duration) string {
	if d >= time.Millisecond {
		return d.Round(time.Millisecond).String()
	}
	return d.Round(time.Microsecond).String()
}
