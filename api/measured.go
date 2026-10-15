package api

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
)

// MeasuredJSON is the last full performance scan gobank-deploy made of a
// running environment, written here as api/measured.json (`gobank-deploy
// scan`): a row per route and rung of the concurrency ladder. Empty
// until a scan has been committed; the pages then show nothing of it.
//
//go:embed measured.json
var MeasuredJSON []byte

// Measured is the scan: where and when, and what each route measured.
type Measured struct {
	Date        string          `json:"date"`
	Version     string          `json:"version"` // gobank's, as scanned
	Environment string          `json:"environment"`
	Server      string          `json:"server"`
	MemoryGB    float64         `json:"memory_gb"`
	Customers   int             `json:"customers"`
	Ladder      []int           `json:"ladder"`
	Samples     int             `json:"samples"`
	Routes      []MeasuredRoute `json:"routes"`
	NotScanned  []NotScanned    `json:"not_scanned"`
}

// MeasuredRoute is one route's rungs.
type MeasuredRoute struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Rungs  []Rung `json:"rungs"`
}

// Rung is one route at one concurrency: the calls made, the latency and
// the throughput achieved.
type Rung struct {
	Concurrency int     `json:"concurrency"`
	N           int     `json:"n"`
	P50ms       float64 `json:"p50_ms"`
	P95ms       float64 `json:"p95_ms"`
	PerSecond   float64 `json:"per_second"`
}

// NotScanned is a route the scan could not drive, with why.
type NotScanned struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Why    string `json:"why"`
}

// ParseMeasured reads a measured document; nil for an empty one.
func ParseMeasured(b []byte) (*Measured, error) {
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, nil
	}
	var m Measured
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("measured.json: %w", err)
	}
	return &m, nil
}

// LoadMeasured is the embedded document; nil when there is none or it
// does not parse, since the pages are not the place to fail.
func LoadMeasured() *Measured {
	m, err := ParseMeasured(MeasuredJSON)
	if err != nil {
		return nil
	}
	return m
}

// Route is the rungs the scan measured for r, by method and path.
func (m *Measured) Route(r Route) ([]Rung, bool) {
	if m == nil {
		return nil, false
	}
	for _, mr := range m.Routes {
		if mr.Method == r.Method && mr.Path == r.Path {
			return mr.Rungs, true
		}
	}
	return nil, false
}

// Why is why the scan did not drive r; "" when it did or never saw it.
func (m *Measured) Why(r Route) string {
	if m == nil {
		return ""
	}
	for _, n := range m.NotScanned {
		if n.Method == r.Method && n.Path == r.Path {
			return n.Why
		}
	}
	return ""
}

// Top is the highest rung of the ladder, 0 without one.
func (m *Measured) Top() int {
	if m == nil || len(m.Ladder) == 0 {
		return 0
	}
	return m.Ladder[len(m.Ladder)-1]
}

// measuredView is a route's scan as the pages show it.
type measuredView struct {
	Date, Version, Environment, Server string
	MemoryGB                           float64
	Customers                          string
	Samples                            int
	Rungs                              []rungView
	Top                                string // the top rung in a phrase: "p95 240ms, 150/s"
	Why                                string // not scanned, and why
}

type rungView struct {
	Concurrency, N      int
	P50, P95, PerSecond string
}

func viewMeasured(m *Measured, r Route) *measuredView {
	if m == nil {
		return nil
	}
	v := &measuredView{Date: m.Date, Version: m.Version, Environment: m.Environment, Server: m.Server, MemoryGB: m.MemoryGB,
		Customers: commas(int64(m.Customers)), Samples: m.Samples, Why: m.Why(r)}
	rungs, _ := m.Route(r)
	for _, rg := range rungs {
		v.Rungs = append(v.Rungs, rungView{Concurrency: rg.Concurrency, N: rg.N, P50: msText(rg.P50ms), P95: msText(rg.P95ms), PerSecond: fmt.Sprintf("%.1f", rg.PerSecond)})
	}
	if n := len(rungs); n > 0 {
		top := rungs[n-1]
		v.Top = fmt.Sprintf("p95 %s, %.0f/s", msText(top.P95ms), top.PerSecond)
	}
	return v
}

// msText shows milliseconds whole when they are, else to a tenth.
func msText(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%.0fms", v)
	}
	return fmt.Sprintf("%.1fms", v)
}

// commas writes n with thousands separators.
func commas(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
