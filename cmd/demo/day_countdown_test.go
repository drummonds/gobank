package main

import (
	"git.bytestone.uk/hum3/gobank/bff/staff"
	"strings"
	"sync"
	"testing"
	"time"
)

// With a day length set, the running simulation knows when the next
// start of day is due by the wall (the day length's share of what is
// left of the day, ADR-0010), and the dashboard's day tile counts down
// to it. Flat out or stopped, there is nothing to count down.
func TestDayEndsInCountsDownWhileRunning(t *testing.T) {
	ds := NewDemoState()
	var clock sync.Mutex // the run loop reads the clock while the test moves it
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	ds.sim.SetWall(func() time.Time { clock.Lock(); defer clock.Unlock(); return now })
	advance := func(d time.Duration) { clock.Lock(); defer clock.Unlock(); now = now.Add(d) }
	if got := ds.SimStatus().DayEndsIn; got != 0 {
		t.Fatalf("stopped: DayEndsIn = %v; want 0", got)
	}

	// The opening day's one event is the next start of day: a start just
	// raises it, and a whole 2h day is left to the one after.
	ds.SetDayLength(2 * time.Hour)
	ds.Start()
	defer ds.Stop()
	var got time.Duration
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if got = ds.SimStatus().DayEndsIn; got > 0 {
			break
		}
	}
	if got != 2*time.Hour || !ds.clock.Now().Equal(time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("running with a 2h day from the second day's start: DayEndsIn = %v at %v; want 2h (the wall is frozen)", got, ds.clock.Now())
	}
	advance(45 * time.Minute)
	if got := ds.SimStatus().DayEndsIn; got != 75*time.Minute {
		t.Errorf("forty-five minutes on: DayEndsIn = %v; want 1h15m", got)
	}
	ds.Stop()
	if got := ds.SimStatus().DayEndsIn; got != 0 {
		t.Errorf("stopped again: DayEndsIn = %v; want 0", got)
	}
}

func TestDayTileShowsCountdown(t *testing.T) {
	d := staff.DashData{Sim: staff.SimStatus{Running: true, DayLength: 2 * time.Hour, DayEndsIn: 62*time.Minute + 3*time.Second}}
	html := staff.BuildDashboardHTML(d)
	if !strings.Contains(html, "ends in 1h2m3s") {
		t.Errorf("day tile should count down to the end of the day:\n%s", html)
	}
	if html := staff.BuildDashboardHTML(staff.DashData{Sim: staff.SimStatus{Running: true}}); strings.Contains(html, "ends in") {
		t.Errorf("flat out there is no end of day to count down to")
	}
}
