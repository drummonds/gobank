package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Another program reads the demo's state at /about.json: gobank-deploy's
// upgrade drill compares the position and the restart record across a
// redeploy, so the fields it needs are there under stable names.
func TestAboutJSONReportsTheProcessForAnotherProgram(t *testing.T) {
	defer func(v string) { version = v }(version)
	version = "v1"
	first := NewDemoState()
	for range 3 {
		first.createCustomer()
	}
	for range 5 {
		first.advanceDay()
	}
	first.RecordStop()
	version = "v2"
	second := newDemoStateOn(first.db, "")
	second.SetDayLength(2 * time.Hour)

	got := aboutStatus(newCoreAdapter(second, ""), second)
	if got.Version != "v2" || got.Settings.DayLength != "2h0m0s" {
		t.Errorf("version %q day length %q", got.Version, got.Settings.DayLength)
	}
	if got.Position.DayCount != 5 || got.Position.Customers != 3 || !strings.HasPrefix(got.Position.Savings, "£") {
		t.Errorf("position = %+v", got.Position)
	}
	if len(got.Restarts) != 2 {
		t.Fatalf("restarts = %d, want 2 newest first", len(got.Restarts))
	}
	latest := got.Restarts[0]
	if latest.Version != "v2" || latest.PreviousVersion != "v1" || latest.Downtime == "" || latest.Intact == nil || !*latest.Intact {
		t.Errorf("latest restart = %+v", latest)
	}
	if got.Restarts[1].Intact != nil {
		t.Error("the first start over a database has nothing to be intact against")
	}
	found := false
	for _, s := range got.Schema {
		found = found || (s.Component == "simulation" && s.Version >= 2)
	}
	if !found {
		t.Errorf("schema = %+v; want the simulation component at its version", got.Schema)
	}

	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"previous_version":"v1"`, `"day_count":5`, `"day_length":"2h0m0s"`, `"day_ends_in"`, `"intact":true`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("json missing %s:\n%s", want, b)
		}
	}
}

// The two rates a performance run reads: customers added per second (the
// live rate while a batch add runs, else the last batch's) and account
// days per 12h at the pass's measured whole-day rate, with the last day's
// duration and accounts so the reader can see what the rate is made of.
func TestAboutJSONReportsTheRates(t *testing.T) {
	ds := NewDemoState()
	clock := steppingClock(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC))
	ds.sim.SetWall(clock)
	ds.progress.now = clock
	ds.clock = fixedBankClock(ds) // banking stamps must not step the wall clock

	got := aboutStatus(newCoreAdapter(ds, ""), ds)
	if got.Sim.CustomersPerSec != 0 || got.Sim.AccountDaysPer12h != 0 || got.Sim.AddingCustomers {
		t.Fatalf("before anything: %+v", got.Sim)
	}

	ds.AddCustomersBatch(5)
	deadline := time.Now().Add(10 * time.Second)
	for ds.IsAddingCustomers() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	ds.clock = ds.sim.Clock() // the day follows the simulation's clock again
	ds.AdvanceDay()

	got = aboutStatus(newCoreAdapter(ds, ""), ds)
	if got.Sim.AddingCustomers || got.Sim.CustomersPerSec != 5 { // batch start and end are the only clock readings: 5 customers in 1s
		t.Errorf("after a batch add: adding %v, %v customers/s; want not adding at 5/s", got.Sim.AddingCustomers, got.Sim.CustomersPerSec)
	}
	last := ds.progress.snapshot()
	want := int64(float64(last.LastAccounts) * float64(passWindow) / float64(last.LastDuration))
	if got.Sim.AccountDaysPer12h != want || want == 0 {
		t.Errorf("account days per 12h = %d, want %d", got.Sim.AccountDaysPer12h, want)
	}
	if got.Sim.LastDayAccounts != last.LastAccounts || got.Sim.LastDayDuration != last.LastDuration.String() {
		t.Errorf("last day = %d accounts in %s, want %d in %s", got.Sim.LastDayAccounts, got.Sim.LastDayDuration, last.LastAccounts, last.LastDuration)
	}

	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"adding_customers":false`, `"customers_per_sec":5`, `"account_days_per_12h":`, `"last_day_duration":"`, `"last_day_accounts":`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("json missing %s:\n%s", want, b)
		}
	}
}
