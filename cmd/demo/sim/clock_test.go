package sim

import (
	"testing"
	"time"
)

// The simulation's clock is stepped (ADR-0010): it stands at an instant,
// whatever the wall does, until it is set to the next; its day is the
// instant's UTC date.
func TestSimulatedClockStandsAtAnInstant(t *testing.T) {
	c := NewClock()
	noon := time.Date(2020, 3, 4, 12, 0, 0, 0, time.UTC)
	c.Set(noon)
	time.Sleep(5 * time.Millisecond)
	if got := c.Now(); !got.Equal(noon) {
		t.Errorf("Now = %v after the wall moved, want %v: the clock stands", got, noon)
	}
	if got := c.Day(); !got.Equal(time.Date(2020, 3, 4, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Day = %v, want the instant's date", got)
	}
	next := noon.Add(13 * time.Hour)
	c.Set(next)
	if got := c.Now(); !got.Equal(next) {
		t.Errorf("Now after the tick = %v, want %v", got, next)
	}
	if got := c.Day(); !got.Equal(time.Date(2020, 3, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Day after the tick = %v, want the next date", got)
	}
}
