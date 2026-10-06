package main

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// With a day length set, the running simulation knows when the current
// day ends, and the dashboard's day tile counts down to it. Flat out or
// stopped, there is nothing to count down.
func TestDayEndsInCountsDownWhileRunning(t *testing.T) {
	ds := NewDemoState()
	var clock sync.Mutex // the run loop reads the clock while the test moves it
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	ds.sim.SetWall(func() time.Time { clock.Lock(); defer clock.Unlock(); return now })
	advance := func(d time.Duration) { clock.Lock(); defer clock.Unlock(); now = now.Add(d) }
	if got := ds.SimStatus().DayEndsIn; got != 0 {
		t.Fatalf("stopped: DayEndsIn = %v; want 0", got)
	}

	ds.SetDayLength(2 * time.Hour)
	ds.Start()
	defer ds.Stop()
	var got time.Duration
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if got = ds.SimStatus().DayEndsIn; got > 0 {
			break
		}
	}
	if got != 2*time.Hour {
		t.Fatalf("running with a 2h day: DayEndsIn = %v; want 2h (the clock is frozen)", got)
	}
	advance(90 * time.Minute)
	if got := ds.SimStatus().DayEndsIn; got != 30*time.Minute {
		t.Errorf("ninety minutes on: DayEndsIn = %v; want 30m", got)
	}
	ds.Stop()
	if got := ds.SimStatus().DayEndsIn; got != 0 {
		t.Errorf("stopped again: DayEndsIn = %v; want 0", got)
	}
}

func TestDayTileShowsCountdown(t *testing.T) {
	d := DashData{Sim: SimStatus{Running: true, DayLength: 2 * time.Hour, DayEndsIn: 62*time.Minute + 3*time.Second}}
	html := renderDashContent(d)
	if !strings.Contains(html, "ends in 1h2m3s") {
		t.Errorf("day tile should count down to the end of the day:\n%s", html)
	}
	if html := renderDashContent(DashData{Sim: SimStatus{Running: true}}); strings.Contains(html, "ends in") {
		t.Errorf("flat out there is no end of day to count down to")
	}
}
