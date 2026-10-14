package api_test

import (
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/gobank/api"
)

// A route's capacity is the requests a second it completes at its peak
// in flight and its p95: how many clients it carries follows from how
// often a client acts.
func TestPerfCapacityAndClients(t *testing.T) {
	p := api.Perf{Count: 120, PeakInFlight: 4, P50: 20 * time.Millisecond, P95: 50 * time.Millisecond, Max: 300 * time.Millisecond, PerSecond: 2.5}
	if c := p.Capacity(); c != 80 {
		t.Errorf("capacity = %v/s; want 4 in flight / 50ms = 80/s", c)
	}
	if n := p.Clients(10 * time.Second); n != 800 {
		t.Errorf("clients at one action every 10 s = %v; want 800", n)
	}
	if (api.Perf{}).Capacity() != 0 {
		t.Error("an unmeasured route has no capacity figure")
	}
}

// With the running process's figures given, the index shows each
// route's requests and p95 and the detail page its performance: the
// figures, the capacity and the clients at one action every ten seconds.
func TestPagesShowTheRunningProcessFigures(t *testing.T) {
	s := api.Surface{
		Packages: []api.Package{{Path: "example.com/app/bff", Name: "bff"}},
		Routes: []api.Route{
			{Package: "example.com/app/bff", Method: "GET", Path: "/things", Pos: "bff/x.go:10", Doc: "things lists things."},
			{Package: "example.com/app/bff", Method: "GET", Path: "/other", Pos: "bff/x.go:20", Doc: "other is untouched."},
		},
	}
	perf := map[string]api.Perf{"GET /things": {Count: 120, PeakInFlight: 4, P50: 20 * time.Millisecond, P95: 50 * time.Millisecond, Max: 300 * time.Millisecond, PerSecond: 2.5}}
	o := api.Options{Perf: func(r api.Route) (api.Perf, bool) {
		p, ok := perf[r.Method+" "+r.Path]
		return p, ok
	}}

	index := string(api.Index(s, o))
	for _, want := range []string{">Requests</th>", ">p95</th>", "<td>120</td>", "<td>50ms</td>"} {
		if !strings.Contains(index, want) {
			t.Errorf("index lacks %q:\n%s", want, index)
		}
	}
	if strings.Count(index, "<td>50ms</td>") != 1 {
		t.Errorf("the untouched route should show no figures:\n%s", index)
	}

	detail := string(api.Detail(s, s.Routes[0], o))
	for _, want := range []string{"Performance", "120 requests", "p50 20ms", "p95 50ms", "max 300ms", "4 in flight at the peak", "2.5 a second over the last minute", "80 requests a second", "800 clients"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail lacks %q:\n%s", want, detail)
		}
	}
	if untouched := string(api.Detail(s, s.Routes[1], o)); !strings.Contains(untouched, "not been served") {
		t.Errorf("an unserved route should say so:\n%s", untouched)
	}
	if plain := string(api.Detail(s, s.Routes[0], api.Options{})); strings.Contains(plain, "Performance") {
		t.Errorf("without figures the detail page has no performance block:\n%s", plain)
	}
}
