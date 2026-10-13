package staff

import (
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
)

// The parameter page shows the declaration, the value in force and the
// settings upcoming; the set form only for a settable parameter and a
// role that may set; the state re-polls while the simulation runs, the
// form never; a refusal is shown on the page.
func TestBuildParameterHTML(t *testing.T) {
	day := time.Date(2020, 3, 5, 0, 0, 0, 0, time.UTC)
	rate := core.ParameterDetail{Owner: "easy-access", Scope: "version", Key: "rate_bps", Kind: "bps", Description: "annual gross rate", InForce: "150", Settable: true,
		Upcoming: []core.Setting{{Scope: "version", Owner: "easy-access", Key: "rate_bps", Value: "175", Effective: day.Add(36 * time.Hour), Decided: day, By: "alice"}}}
	page := BuildParameterHTML(rate, day, true, false, "")
	for _, want := range []string{`id="parameter-detail"`, "<code>rate_bps</code>", "version", "bps", "annual gross rate", "In force", "150", "Upcoming", "175", "6 Mar 2020 12:00", "alice",
		`hx-post="products/easy-access/parameters/rate_bps"`, `hx-target="#parameter-detail"`, `id="parameter-state"`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "hx-trigger") {
		t.Error("a page that is not polling polls")
	}
	polling := BuildParameterHTML(rate, day, true, true, "")
	if !strings.Contains(polling, `hx-trigger="every 2s"`) || !strings.Contains(polling, `hx-select="#parameter-state"`) {
		t.Errorf("while the simulation runs the state re-polls itself:\n%s", polling)
	}
	if i, j := strings.Index(polling, "hx-trigger"), strings.Index(polling, "<form"); i > j {
		t.Error("the form is inside what polls")
	}
	// A pre-filled tomorrow goes stale while the clock runs and the form
	// does not re-render, so a running bank leaves the instant empty: now.
	if !strings.Contains(page, `name="effective" value="2020-03-06T00:00"`) {
		t.Errorf("a stopped bank does not offer tomorrow:\n%s", page)
	}
	if !strings.Contains(polling, `name="effective" value=""`) {
		t.Errorf("a running bank pre-fills an instant that will be stale:\n%s", polling)
	}
	if ro := BuildParameterHTML(rate, day, false, false, ""); strings.Contains(ro, "<form") {
		t.Error("a role that may not set sees the form")
	}
	refused := BuildParameterHTML(rate, day, true, false, "rate_bps: \"x\" is not a bps")
	if !strings.Contains(refused, "is-danger") || !strings.Contains(refused, "is not a bps") {
		t.Error("a refusal is not shown")
	}

	market := core.ParameterDetail{Owner: "GBP", Scope: "market", Key: "risk_free_rate_bps", Kind: "bps", Description: "Bank Rate, as the Bank of England publishes it", InForce: "75"}
	if page := BuildParameterHTML(market, day, true, false, ""); strings.Contains(page, "<form") || !strings.Contains(page, "Nothing upcoming") || !strings.Contains(page, `href="products/bank"`) {
		t.Errorf("the market's parameter: no form, nothing upcoming, a way back to the bank tab:\n%s", page)
	}
	derived := core.ParameterDetail{Owner: "easy-access-v2", Scope: "version", Key: "rate_bps", Kind: "bps", Derived: true, Derivation: "savings_rate_bps − 15 bps, floor 0"}
	if page := BuildParameterHTML(derived, day, true, false, ""); strings.Contains(page, "<form") || !strings.Contains(page, "savings_rate_bps − 15 bps, floor 0") {
		t.Errorf("a derived parameter: no form, the derivation in words:\n%s", page)
	}
	spread := core.ParameterDetail{Owner: "bank", Scope: "bank", Key: "savings_spread_bps", Kind: "bps", Description: "the savings rate's spread", InForce: "-50", Settable: true}
	if page := BuildParameterHTML(spread, day, true, false, ""); !strings.Contains(page, `hx-post="products/bank/parameters/savings_spread_bps"`) || !strings.Contains(page, `href="products/bank"`) {
		t.Errorf("the policy's spread: a form posting to the bank's own page, a way back to the bank tab:\n%s", page)
	}
}

// The policy's chart draws the risk-free rate against the bank's savings
// and lending rates over the run, as percentages, each series named.
func TestRatesChart(t *testing.T) {
	d := func(n int) time.Time { return time.Date(2020, 1, 1+n, 0, 0, 0, 0, time.UTC) }
	hist := core.History{
		RiskFree:    []core.RatePoint{{Date: d(0), Bps: 75}, {Date: d(1), Bps: 75}, {Date: d(2), Bps: 25}},
		SavingsRate: []core.RatePoint{{Date: d(1), Bps: 25}, {Date: d(2), Bps: 0}},
		LendingRate: []core.RatePoint{{Date: d(1), Bps: 375}, {Date: d(2), Bps: 375}},
	}
	svg := buildRatesChart(hist)
	for _, want := range []string{"<svg", "risk-free", "savings", "lending", "0.50%"} {
		if !strings.Contains(svg, want) {
			t.Errorf("chart lacks %q:\n%s", want, svg)
		}
	}
	if empty := buildRatesChart(core.History{}); strings.Contains(empty, "<svg") {
		t.Errorf("an empty history draws a chart: %s", empty)
	}
}

// A row link loads the parameter's detail whether or not the master it
// sits in is polling: the master's own poll selects the master from the
// page, and a link inside it must not inherit that selection, or its
// reply (the detail) is filtered to nothing.
func TestRowLinkSelectsTheDetailInsideAPollingMaster(t *testing.T) {
	page := masterDetail(parameterTable([]string{parameterRow("easy-access", "rate_bps", "bps", "150")}, "In force"), "products/savings", true)
	row := page[strings.Index(page, "<a "):]
	row = row[:strings.Index(row, ">")+1]
	if !strings.Contains(row, `hx-select="#parameter-detail"`) {
		t.Errorf("a row link in a polling master does not select the detail from its reply:\n%s", row)
	}
}
