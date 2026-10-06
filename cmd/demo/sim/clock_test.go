package sim

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
	c := NewClock(func() time.Time { return wall })
	day := time.Date(2020, 3, 4, 0, 0, 0, 0, time.UTC)
	c.SetDayLength(2 * time.Hour)
	c.BeginDay(day)

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
	c.SetDayLength(4 * time.Hour)
	if got := c.Now(); !got.Equal(day.Add(6 * time.Hour)) {
		t.Errorf("after the day length doubles an hour in: Now = %v, want 06:00", got)
	}

	c.SetDayLength(0) // flat out: wall pace, inside the day
	c.BeginDay(day.AddDate(0, 0, 1))
	wall = wall.Add(5 * time.Second)
	if got := c.Now(); !got.Equal(day.AddDate(0, 0, 1).Add(5 * time.Second)) {
		t.Errorf("flat out, 5s in: Now = %v, want 00:00:05 the next day", got)
	}
	wall = wall.Add(48 * time.Hour)
	if got := c.Now(); !got.Equal(day.AddDate(0, 0, 1).Add(24*time.Hour - time.Second)) {
		t.Errorf("flat out, long after: Now = %v, want the last second of the day", got)
	}
}
