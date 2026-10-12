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
}
