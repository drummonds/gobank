package sim

import (
	"testing"
	"time"
)

// The day length is spread between events in proportion to the simulated
// gap between them, less what the wall has already spent; flat out, only
// a start of day waits, and only for the browser's sake.
func TestPaceSpreadsTheDayLengthBetweenEvents(t *testing.T) {
	for _, c := range []struct {
		name       string
		dayLength  time.Duration
		gap, spent time.Duration
		startOfDay bool
		want       time.Duration
	}{
		{"a whole day of a 2h day", 2 * time.Hour, 24 * time.Hour, 0, true, 2 * time.Hour},
		{"noon to midnight of a 2h day", 2 * time.Hour, 12 * time.Hour, 0, true, time.Hour},
		{"an hour of a 24h day, half spent", 24 * time.Hour, time.Hour, 30 * time.Minute, false, 30 * time.Minute},
		{"a share the work overran", 2 * time.Hour, time.Hour, time.Hour, false, 0},
		{"flat out before a day", 0, 24 * time.Hour, 0, true, minDayGap},
		{"flat out before a payment", 0, time.Hour, 0, false, 0},
	} {
		if got := pace(c.dayLength, c.gap, c.spent, c.startOfDay); got != c.want {
			t.Errorf("%s: pace = %v, want %v", c.name, got, c.want)
		}
	}
}
