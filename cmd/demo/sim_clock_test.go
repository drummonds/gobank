package main

import (
	"testing"
	"time"
)

// The simulation's clock warps a day: the day begins at slot start and
// the day length sets how fast simulated time passes, so an event an hour
// into a two-hour day is stamped at noon. Flat out, simulated time passes
// at the wall's pace. Either way the clock stays inside its day until the
// next begins.
func TestSimulatedClockWarpsTheDay(t *testing.T) {
	wall := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	c := newSimClock(func() time.Time { return wall })
	day := time.Date(2020, 3, 4, 0, 0, 0, 0, time.UTC)
	c.setDayLength(2 * time.Hour)
	c.beginDay(day)

	for _, tc := range []struct {
		elapsed time.Duration
		want    time.Time
	}{
		{0, day},
		{30 * time.Minute, day.Add(6 * time.Hour)},
		{time.Hour, day.Add(12 * time.Hour)},
		{3 * time.Hour, day.Add(24*time.Hour - time.Second)}, // past the end of the slot: still that day
	} {
		wall = time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC).Add(tc.elapsed)
		if got := c.Now(); !got.Equal(tc.want) {
			t.Errorf("%v into a 2h day: Now = %v, want %v", tc.elapsed, got, tc.want)
		}
	}
	if !c.Day().Equal(day) {
		t.Errorf("Day = %v, want %v", c.Day(), day)
	}

	// A day length set mid-day re-warps what is left of it.
	wall = time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC) // an hour in: noon
	c.setDayLength(4 * time.Hour)
	if got := c.Now(); !got.Equal(day.Add(6 * time.Hour)) {
		t.Errorf("after the day length doubles an hour in: Now = %v, want 06:00", got)
	}

	c.setDayLength(0) // flat out: wall pace, inside the day
	c.beginDay(day.AddDate(0, 0, 1))
	wall = wall.Add(5 * time.Second)
	if got := c.Now(); !got.Equal(day.AddDate(0, 0, 1).Add(5 * time.Second)) {
		t.Errorf("flat out, 5s in: Now = %v, want 00:00:05 the next day", got)
	}
	wall = wall.Add(48 * time.Hour)
	if got := c.Now(); !got.Equal(day.AddDate(0, 0, 1).Add(24*time.Hour - time.Second)) {
		t.Errorf("flat out, long after: Now = %v, want the last second of the day", got)
	}
}

// The clock's day and when it began are on the run row, so the next
// process resumes the same simulated day and the bank is on it too.
func TestSimulatedClockResumesFromTheRunRow(t *testing.T) {
	first := NewDemoState()
	addFundedCustomer(first)
	first.AdvanceDay()
	first.AdvanceDay()
	day := first.simClock.Day()
	if want := time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC); !day.Equal(want) {
		t.Fatalf("clock day after two advances = %v, want %v", day, want)
	}

	second := newDemoStateOn(first.db, "")
	if !second.simClock.Day().Equal(day) {
		t.Errorf("resumed clock day %v, want %v", second.simClock.Day(), day)
	}
	if pos := second.position(); !pos.Day.Equal(day) || pos.DayCount != 2 {
		t.Errorf("resumed bank position %+v, want day %v count 2", pos, day)
	}
	if !second.simClock.Now().After(day) || second.simClock.Now().After(day.Add(24*time.Hour)) {
		t.Errorf("resumed clock Now = %v, want inside %v", second.simClock.Now(), day)
	}
}
