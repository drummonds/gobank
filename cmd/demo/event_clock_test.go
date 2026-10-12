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
// products page from the moment it is decided, in the simulation's
// timeline, and fired to the product's accounts when the clock reaches
// its instant; after that it is in force and no longer upcoming.
func TestAStaffSettingIsAnEvent(t *testing.T) {
	ds := NewDemoState()
	twoFundedCustomers(ds)
	ctx := context.Background()
	ds.AdvanceDay()
	effective := ds.clock.Now().Add(10 * time.Hour)
	if err := ds.SetParameter(ctx, core.ParameterSetting{ProductID: "easy-access", Key: "rate_bps", Value: "175", Effective: effective, By: "admin"}); err != nil {
		t.Fatal(err)
	}
	page := staff.BuildProductsHTML(ds.Bank, "Savings", true)
	for _, want := range []string{"Upcoming", "175", effective.Format("2 Jan 2006 15:04"), "admin", `name="key"`, `action="products/set"`, "Next decision day: 30 Jan 2020 12:00 UTC"} {
		if !strings.Contains(page, want) {
			t.Errorf("products page lacks %q:\n%s", want, page)
		}
	}
	if ro := staff.BuildProductsHTML(ds.Bank, "Savings", false); strings.Contains(ro, `action="products/set"`) {
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
	if page := staff.BuildProductsHTML(ds.Bank, "Savings", false); strings.Contains(page, "Upcoming") {
		t.Error("the products page still lists the setting as upcoming")
	}
}

// The set-parameter form posts the command as the signed-in user; a
// refused setting sends the operator back with the reason.
func TestSetParameterRoute(t *testing.T) {
	t.Setenv("GOBANK_ADMIN_PASSWORD", testAdminPassword)
	ds := NewDemoState()
	addFundedCustomer(ds)
	h := newHandler(ds, "test", "/")
	admin := staffLogin(t, h)
	post := func(form string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/products/set", strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(admin)
		h.ServeHTTP(rr, req)
		return rr
	}
	effective := ds.clock.Now().Add(26 * time.Hour)
	rr := post("product=easy-access&family=Savings&key=rate_bps&value=180&effective=" + effective.Format("2006-01-02T15:04"))
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/products/savings" {
		t.Fatalf("set parameter: %d %s", rr.Code, rr.Header().Get("Location"))
	}
	up, _ := ds.UpcomingSettings(context.Background())
	if len(up) != 1 || up[0].Value != "180" || up[0].By != "admin" || !up[0].Effective.Equal(effective) {
		t.Errorf("upcoming after the post = %+v", up)
	}
	rr = post("product=easy-access&family=Savings&key=rate_bps&value=nonsense&effective=" + effective.Format("2006-01-02T15:04"))
	if rr.Code != http.StatusSeeOther || !strings.Contains(rr.Header().Get("Location"), "refused=") {
		t.Errorf("a bad value: %d %s; want a redirect naming the refusal", rr.Code, rr.Header().Get("Location"))
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
