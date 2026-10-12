package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/gobank/bff/staff"
	"git.bytestone.uk/hum3/gobank/cmd/demo/sim"
	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/gobank/internal/view"
)

// The simulation moves by events (ADR-0010): the clock stands at an
// event's instant while the bank processes it, and a console step raises
// every event up to and including the next start of day. On the opening
// day the first event is the market's noon publication, which the bank
// takes as the day's rate and whose cascade reaches every account; the
// step ends at the next midnight with the bank at rest.
func TestTheClockMovesByEvents(t *testing.T) {
	ds := NewDemoState()
	twoFundedCustomers(ds)
	ctx := context.Background()
	if pos := ds.position(); pos.RiskFreeBps != 0 || !ds.clock.Now().Equal(sim.OpeningDay) {
		t.Fatalf("a fresh bank at %v has rate %d; want the opening instant, no rate until the market publishes", ds.clock.Now(), pos.RiskFreeBps)
	}
	if owed, _ := ds.Owed(ctx); !owed.AtRest() {
		t.Fatalf("a fresh bank owes %+v", owed)
	}

	ds.AdvanceDay()
	if now := ds.clock.Now(); !now.Equal(sim.OpeningDay.AddDate(0, 0, 1)) {
		t.Errorf("after a step the clock stands at %v, want the next start of day", now)
	}
	pos := ds.position()
	if want := sim.BoERates[0].Bps; pos.DayCount != 1 || pos.RiskFreeBps != want {
		t.Errorf("after the step: day %d at %d bps, want day 1 at the series' %d (published at noon on the way)", pos.DayCount, pos.RiskFreeBps, want)
	}
	if owed, _ := ds.Owed(ctx); !owed.AtRest() {
		t.Errorf("after the step the bank owes %+v", owed)
	}
	// The cascade stamped every account at the publication's instant.
	var n int
	noon := sim.BoERates[0].At.UTC()
	if err := ds.db.QueryRow(`SELECT COUNT(*) FROM product_account_parameters WHERE applied_to = $1`, noon).Scan(&n); err != nil || n == 0 {
		t.Errorf("%d accounts had their parameters applied at the publication's instant %v (%v); want all", n, noon, err)
	}
	if run, _ := loadRun(ds.db); !run.At.Equal(ds.clock.Now()) {
		t.Errorf("the run row holds %v, want the clock's instant %v", run.At, ds.clock.Now())
	}
}

// The generator's payments are events on the timeline: PaymentsPerDay a
// day, at instants inside the day drawn from the seed, raised when the
// clock reaches each, so they are stamped inside the day in order and
// the bank is at rest between them. Stopped, none are raised.
func TestPaymentsAreEventsOnTheTimeline(t *testing.T) {
	ds := NewDemoState()
	for range 4 {
		addFundedCustomer(ds)
	}
	ds.AdvanceDay()
	before := ds.paymentCount()
	ds.AdvanceDay()
	if n := ds.paymentCount(); n != before {
		t.Fatalf("payments stopped, yet %d were raised", n-before)
	}

	ds.UpdateSettings(1000, 3)
	ds.StartPayments()
	ds.AdvanceDay()
	day := ds.position().Day.AddDate(0, 0, -1) // the day just stepped through
	page, _ := ds.paymentPage(1)
	var stamps []time.Time
	for _, p := range page {
		if p.ID > before && p.CreatedAt.After(day) && p.CreatedAt.Before(day.AddDate(0, 0, 1)) {
			stamps = append(stamps, p.CreatedAt)
		}
	}
	if len(stamps) != 3 {
		t.Fatalf("%d payments raised on %s, want the 3 a day set: %v", len(stamps), day.Format(time.DateOnly), stamps)
	}
	for i := 1; i < len(stamps); i++ {
		if !stamps[i].Before(stamps[i-1]) { // newest first
			t.Errorf("payments not at distinct instants in order: %v", stamps)
		}
	}
	if !ds.IsPaymentsRunning() {
		t.Error("the generator stopped on its own")
	}
	ds.StopPayments()
	if ds.IsPaymentsRunning() {
		t.Error("the generator did not stop")
	}
}

// A staff setting of a product's parameter is an event: upcoming on the
// products page and on the parameter's own page from the moment it is
// decided, in the simulation's timeline, and fired to the product's
// accounts when the clock reaches its instant; after that it is in force
// and no longer upcoming.
func TestAStaffSettingIsAnEvent(t *testing.T) {
	ds := NewDemoState()
	twoFundedCustomers(ds)
	ctx := context.Background()
	ds.AdvanceDay()
	effective := ds.clock.Now().Add(10 * time.Hour)
	if err := ds.SetParameter(ctx, core.ParameterSetting{ProductID: "easy-access", Key: "rate_bps", Value: "175", Effective: effective, By: "admin"}); err != nil {
		t.Fatal(err)
	}
	page := staff.BuildProductsHTML(ds.Bank, "Savings", view.Bank, false)
	for _, want := range []string{"Upcoming", "175", effective.Format("2 Jan 2006 15:04"), "admin", `hx-get="products/easy-access/parameters/rate_bps"`, `id="parameter-detail"`} {
		if !strings.Contains(page, want) {
			t.Errorf("products page lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "Next decision day") || strings.Contains(page, `action="products/set"`) {
		t.Error("the products page still carries the market or the old set form")
	}
	d, err := ds.Parameter(ctx, "easy-access", "rate_bps")
	if err != nil {
		t.Fatal(err)
	}
	detail := staff.BuildParameterHTML(d, ds.clock.Now(), true, false, "")
	for _, want := range []string{"In force", "150", "175", "admin", `hx-post="products/easy-access/parameters/rate_bps"`} {
		if !strings.Contains(detail, want) {
			t.Errorf("parameter page lacks %q:\n%s", want, detail)
		}
	}
	if ro := staff.BuildParameterHTML(d, ds.clock.Now(), false, false, ""); strings.Contains(ro, "hx-post=") {
		t.Error("a role that may not set parameters sees the form")
	}

	ds.AdvanceDay()
	if now := ds.clock.Now(); !now.After(effective) {
		t.Errorf("the clock stands at %v, want past the setting's %v", now, effective)
	}
	if up, _ := ds.UpcomingSettings(ctx); len(up) != 0 {
		t.Errorf("in force but still upcoming: %+v", up)
	}
	if owed, _ := ds.Owed(ctx); !owed.AtRest() {
		t.Errorf("after the step the bank owes %+v", owed)
	}
	var applied int
	if err := ds.db.QueryRow(`SELECT COUNT(*) FROM product_account_parameters WHERE applied_to = $1`, effective.UTC()).Scan(&applied); err != nil || applied == 0 {
		t.Errorf("%d accounts had the setting applied at its instant (%v); want easy-access's", applied, err)
	}
	if page := staff.BuildProductsHTML(ds.Bank, "Savings", view.Bank, false); strings.Contains(page, "Upcoming") {
		t.Error("the products page still lists the setting as upcoming")
	}
	if d, _ := ds.Parameter(ctx, "easy-access", "rate_bps"); d.InForce != "175" || len(d.Upcoming) != 0 {
		t.Errorf("after the instant the parameter is %+v; want 175 in force", d)
	}
}

// The parameter page is the master-detail's detail (lofigui's HTMX
// multipage pattern): an HTMX request gets the fragment alone, a browser
// the whole page, so a parameter deep-links. The set form posts the
// command as the signed-in user and gets the detail back, a refused
// setting with the reason; a plain browser is sent back to the page.
func TestParameterPageAndSetRoute(t *testing.T) {
	t.Setenv("GOBANK_ADMIN_PASSWORD", testAdminPassword)
	ds := NewDemoState()
	addFundedCustomer(ds)
	h := newHandler(ds, "test", "/")
	admin := staffLogin(t, h)
	const route = "/products/easy-access/parameters/rate_bps"
	do := func(method, form string, htmx bool) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(method, route, strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		req.AddCookie(admin)
		h.ServeHTTP(rr, req)
		return rr
	}
	full := do("GET", "", false)
	if full.Code != http.StatusOK || !strings.Contains(full.Body.String(), "<base href=") || !strings.Contains(full.Body.String(), `href="products/savings"`) {
		t.Errorf("a browser's GET: %d, want the whole page with a way back to Savings:\n%.300s", full.Code, full.Body.String())
	}
	frag := do("GET", "", true)
	if frag.Code != http.StatusOK || strings.Contains(frag.Body.String(), "<base href=") || !strings.HasPrefix(strings.TrimSpace(frag.Body.String()), `<div id="parameter-detail"`) {
		t.Errorf("HTMX's GET: %d, want the detail alone:\n%.300s", frag.Code, frag.Body.String())
	}
	effective := ds.clock.Now().Add(26 * time.Hour)
	rr := do("POST", "value=180&effective="+effective.Format("2006-01-02T15:04"), true)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "180") || !strings.Contains(rr.Body.String(), "Upcoming") {
		t.Fatalf("set parameter over HTMX: %d, want the detail with 180 upcoming:\n%.500s", rr.Code, rr.Body.String())
	}
	up, _ := ds.UpcomingSettings(context.Background())
	if len(up) != 1 || up[0].Value != "180" || up[0].By != "admin" || !up[0].Effective.Equal(effective) {
		t.Errorf("upcoming after the post = %+v", up)
	}
	rr = do("POST", "value=nonsense&effective="+effective.Format("2006-01-02T15:04"), true)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "is-danger") || !strings.Contains(rr.Body.String(), "nonsense") {
		t.Errorf("a bad value over HTMX: %d, want the detail naming the refusal:\n%.500s", rr.Code, rr.Body.String())
	}
	rr = do("POST", "value=190&effective="+effective.Add(time.Hour).Format("2006-01-02T15:04"), false)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != route {
		t.Errorf("a browser's post: %d %s; want a redirect back to the page", rr.Code, rr.Header().Get("Location"))
	}
	if rr := do("GET", "", false); !strings.Contains(rr.Body.String(), "190") {
		t.Error("the page does not show the setting just made")
	}
	rr = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/products/no-such/parameters/rate_bps", nil)
	req.AddCookie(admin)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("an unknown parameter: %d, want 404", rr.Code)
	}
}

// The console shows what the bank owes and the next event, on the
// dashboard and in about.json, so a drill can wait for at_rest.
func TestConsoleShowsOwedAndTheNextEvent(t *testing.T) {
	html := staff.BuildDashboardHTML(staff.DashData{Sim: staff.SimStatus{
		Running: true, Owed: core.Owed{Days: 1, Events: 12},
		Next: staff.Event{Kind: "payment", At: time.Date(2020, 3, 4, 10, 32, 0, 0, time.UTC)},
	}})
	for _, want := range []string{"owed: 1 day, 12 events in progress", "next: payment 4 Mar 10:32"} {
		if !strings.Contains(html, want) {
			t.Errorf("dashboard lacks %q:\n%s", want, html)
		}
	}
	if html := staff.BuildDashboardHTML(staff.DashData{}); strings.Contains(html, "owed:") || strings.Contains(html, "next:") {
		t.Error("at rest and stopped, the dashboard shows owed work or a next event")
	}

	ds := NewDemoState()
	addFundedCustomer(ds)
	ds.AdvanceDay()
	b, err := json.Marshal(staff.AboutStatusOf(ds.Bank, ds, version))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"owed":{"at_rest":true,"days":0,"unprojected":0,"events":0}`) {
		t.Errorf("about.json lacks the owed counts:\n%s", b)
	}
}
