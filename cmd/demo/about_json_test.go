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
