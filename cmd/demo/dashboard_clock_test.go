package main

import (
	"git.bytestone.uk/hum3/gobank/bff/staff"
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
)

// The dashboard shows both clocks side by side, the wall clock and the
// simulated one, with the warp between them, so a watcher gets a feel for
// the rate at which simulated time passes.
func TestDashboardShowsBothClocks(t *testing.T) {
	ds := NewDemoState()
	defer ds.db.Close()
	wall := time.Date(2026, 10, 6, 14, 5, 9, 0, time.UTC)
	ds.sim.SetWall(func() time.Time { return wall })
	ds.clock = core.ClockFunc(func() time.Time { return time.Date(2020, 3, 4, 11, 30, 0, 0, time.UTC) })
	ds.SetDayLength(2 * time.Hour)

	html := staff.BuildDashboardHTML(dashboardData(ds.Bank, ds))
	for _, want := range []string{
		`<p class="heading">Wall clock</p><p class="title is-5">14:05:09</p>`,
		`<p class="heading">Sim clock</p><p class="title is-5">4 Mar 2020 11:30:00</p><p class="heading">&times;12 wall pace</p>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("dashboard should show %s:\n%s", want, html)
		}
	}

	ds.SetDayLength(0)
	html = staff.BuildDashboardHTML(dashboardData(ds.Bank, ds))
	if want := `<p class="heading">flat out</p>`; !strings.Contains(html, want) {
		t.Errorf("flat out, the sim clock's rate should read %s:\n%s", want, html)
	}
}

// about.json carries the two clocks and the warp too.
func TestAboutJSONCarriesBothClocks(t *testing.T) {
	ds := NewDemoState()
	defer ds.db.Close()
	ds.sim.SetWall(func() time.Time { return time.Date(2026, 10, 6, 14, 5, 9, 0, time.UTC) })
	ds.clock = core.ClockFunc(func() time.Time { return time.Date(2020, 3, 4, 11, 30, 0, 0, time.UTC) })
	ds.SetDayLength(2 * time.Hour)
	got := aboutStatus(ds.Bank, ds)
	if got.Sim.Wall != "2026-10-06T14:05:09Z" || got.Sim.Clock != "2020-03-04T11:30:00Z" || got.Sim.Warp != 12 {
		t.Errorf("about.json sim clocks = wall %q clock %q warp %v; want 2026-10-06T14:05:09Z, 2020-03-04T11:30:00Z, 12", got.Sim.Wall, got.Sim.Clock, got.Sim.Warp)
	}
}
