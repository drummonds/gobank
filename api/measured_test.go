package api_test

import (
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/api"
)

// measuredDoc is a scan as gobank-deploy writes it into api/measured.json.
const measuredDoc = `{
  "date": "2026-10-15", "version": "v0.51.0", "environment": "preprod", "server": "ccx33", "memory_gb": 32, "customers": 1234,
  "ladder": [1, 4, 16], "samples": 20,
  "routes": [
    {"method": "GET", "path": "/things", "rungs": [
      {"concurrency": 1, "n": 20, "p50_ms": 12.5, "p95_ms": 30, "per_second": 45.2},
      {"concurrency": 4, "n": 20, "p50_ms": 20, "p95_ms": 55, "per_second": 120.7},
      {"concurrency": 16, "n": 20, "p50_ms": 80, "p95_ms": 240, "per_second": 150.1}
    ]}
  ],
  "not_scanned": [{"method": "GET", "path": "/other", "why": "needs a path parameter"}]
}`

// The measured document is the last scan gobank-deploy made: a route's
// rungs by method and path, and where and when the scan was.
func TestMeasuredDocumentIsReadByRoute(t *testing.T) {
	m, err := api.ParseMeasured([]byte(measuredDoc))
	if err != nil {
		t.Fatal(err)
	}
	if m.Date != "2026-10-15" || m.Version != "v0.51.0" || m.Environment != "preprod" || m.Customers != 1234 {
		t.Errorf("header = %+v", m)
	}
	rungs, ok := m.Route(api.Route{Method: "GET", Path: "/things"})
	if !ok || len(rungs) != 3 || rungs[2].Concurrency != 16 || rungs[2].P95ms != 240 || rungs[2].PerSecond != 150.1 {
		t.Errorf("rungs = %+v, %v", rungs, ok)
	}
	if _, ok := m.Route(api.Route{Method: "GET", Path: "/other"}); ok {
		t.Error("a route not scanned has no rungs")
	}
	if empty, err := api.ParseMeasured(nil); err != nil || empty != nil {
		t.Errorf("no document = %+v, %v; want none, no error", empty, err)
	}
}

// With a measured document given, the index shows each route's p95 and
// throughput at the top rung and the detail page the whole ladder with
// where and when it was measured; without one the pages say nothing of
// it.
func TestPagesShowTheMeasuredDocument(t *testing.T) {
	s := api.Surface{
		Packages: []api.Package{{Path: "example.com/app/bff", Name: "bff"}},
		Routes: []api.Route{
			{Package: "example.com/app/bff", Method: "GET", Path: "/things", Pos: "bff/x.go:10", Doc: "things lists things."},
			{Package: "example.com/app/bff", Method: "GET", Path: "/other", Pos: "bff/x.go:20", Doc: "other is untouched."},
		},
	}
	m, _ := api.ParseMeasured([]byte(measuredDoc))
	o := api.Options{Measured: m}

	index := string(api.Index(s, o))
	for _, want := range []string{">Measured at 16</th>", "<td>p95 240ms, 150/s</td>"} {
		if !strings.Contains(index, want) {
			t.Errorf("index lacks %q:\n%s", want, index)
		}
	}
	detail := string(api.Detail(s, s.Routes[0], o))
	for _, want := range []string{"Measured", "2026-10-15", "v0.51.0", "preprod", "ccx33", "1,234 customers",
		"<td>1</td><td>20</td><td>12.5ms</td><td>30ms</td><td>45.2</td>",
		"<td>16</td><td>20</td><td>80ms</td><td>240ms</td><td>150.1</td>"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail lacks %q:\n%s", want, detail)
		}
	}
	if untouched := string(api.Detail(s, s.Routes[1], o)); !strings.Contains(untouched, "not scanned") || !strings.Contains(untouched, "needs a path parameter") {
		t.Errorf("a route not scanned should say so and why:\n%s", untouched)
	}
	if plain := string(api.Index(s, api.Options{})); strings.Contains(plain, "Measured") {
		t.Errorf("without a document the index has no measured column:\n%s", plain)
	}
}
