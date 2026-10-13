package family

import (
	"strings"
	"testing"
)

// A cut of go mod graph from the demo module (dependent dependency@version
// per line) and of go list -m all (the versions the build selected).
const graph = `git.bytestone.uk/hum3/gobank/cmd/demo git.bytestone.uk/hum3/gobank@v0.3.44
git.bytestone.uk/hum3/gobank/cmd/demo git.bytestone.uk/hum3/go-luca@v0.5.0
git.bytestone.uk/hum3/gobank/cmd/demo git.bytestone.uk/hum3/go-postgres@v0.8.0
git.bytestone.uk/hum3/gobank/cmd/demo github.com/stretchr/testify@v1.9.0
git.bytestone.uk/hum3/gobank@v0.3.44 git.bytestone.uk/hum3/go-luca@v0.5.0
git.bytestone.uk/hum3/gobank@v0.3.44 git.bytestone.uk/hum3/go-postgres@v0.8.0
git.bytestone.uk/hum3/go-luca@v0.5.0 git.bytestone.uk/hum3/go-postgres@v0.7.0
git.bytestone.uk/hum3/go-luca@v0.5.0 golang.org/x/text@v0.20.0
git.bytestone.uk/hum3/go-luca@v0.5.0 git.bytestone.uk/hum3/gotreesitter@v0.6.8
git.bytestone.uk/hum3/gobank@v0.3.44 git.bytestone.uk/hum3/gotreesitter@v0.7.0
`

const buildList = `git.bytestone.uk/hum3/gobank/cmd/demo
git.bytestone.uk/hum3/go-luca v0.5.0
git.bytestone.uk/hum3/go-postgres v0.8.0
git.bytestone.uk/hum3/gobank v0.3.44 => ../..
git.bytestone.uk/hum3/gotreesitter v0.7.0
golang.org/x/text v0.20.0
`

// Parse keeps only our repos, merges a repo's nested modules into one
// node (gobank and gobank/cmd/demo are one repo) and drops what a repo
// asks of itself; each node carries the version the build selected, and
// each edge what the dependent's go.mod asked for.
func TestParseIsTheFamilyAtTheBuildsVersions(t *testing.T) {
	m, err := Parse(graph, buildList)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"gobank": "", "go-luca": "v0.5.0", "go-postgres": "v0.8.0", "gotreesitter": "v0.7.0"}
	if len(m.Repos) != len(want) {
		t.Fatalf("repos %v, want %v", m.Repos, want)
	}
	for _, r := range m.Repos {
		v, ok := want[r.Name]
		if !ok || r.Version != v {
			t.Errorf("repo %+v, want version %q", r, v)
		}
	}
	edges := map[string]bool{}
	for _, e := range m.Requires {
		edges[e.From+"->"+e.To+"@"+e.Version] = true
	}
	for _, e := range []string{"gobank->go-luca@v0.5.0", "gobank->go-postgres@v0.8.0", "go-luca->go-postgres@v0.7.0", "go-luca->gotreesitter@v0.6.8", "gobank->gotreesitter@v0.7.0"} {
		if !edges[e] {
			t.Errorf("edge %s missing from %v", e, m.Requires)
		}
	}
	if len(edges) != 5 {
		t.Errorf("%d edges, want 5 (the demo's duplicates of the root's folded, gobank->gobank dropped, third parties out): %v", len(edges), m.Requires)
	}
}

// Lags reports a requirement for a version older than the one the build
// selected: the dependent's go.mod is behind what it actually runs with.
func TestLags(t *testing.T) {
	m, _ := Parse(graph, buildList)
	lagging := map[string]bool{}
	for _, e := range m.Requires {
		if m.Lags(e) {
			lagging[e.From+"->"+e.To] = true
		}
	}
	if !lagging["go-luca->go-postgres"] || !lagging["go-luca->gotreesitter"] || len(lagging) != 2 {
		t.Errorf("lagging %v; want go-luca's two requirements only", lagging)
	}
}

// The d2 is one node per repo labelled with its selected version, the
// root without one, and one edge per requirement; a lagging requirement
// is drawn apart and labelled with the version it asks for.
func TestD2(t *testing.T) {
	m, _ := Parse(graph, buildList)
	d2 := m.D2()
	for _, want := range []string{
		`go_luca: "go-luca v0.5.0"`,
		`gobank: "gobank"`,
		"gobank -> go_luca\n",
		`go_luca -> go_postgres: "v0.7.0" {class: lags}`,
		"direction: right",
	} {
		if !strings.Contains(d2, want) {
			t.Errorf("d2 lacks %q:\n%s", want, d2)
		}
	}
	if strings.Contains(d2, "testify") || strings.Contains(d2, "x/text") {
		t.Errorf("third parties drawn:\n%s", d2)
	}
}
