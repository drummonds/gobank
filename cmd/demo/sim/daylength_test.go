package sim

import (
	"testing"
	"time"
)

// nextDayDelay is the wait after a day completes, given the day length and
// how long the day's work took.
func TestNextDayDelay(t *testing.T) {
	cases := []struct {
		name      string
		dayLength time.Duration
		elapsed   time.Duration
		want      time.Duration
	}{
		{"flat out", 0, 50 * time.Millisecond, minDayGap},
		{"two hour day, work took a minute", 2 * time.Hour, time.Minute, 2*time.Hour - time.Minute},
		{"work took longer than the day", time.Second, 2 * time.Second, minDayGap},
		{"work nearly filled the day", time.Second, time.Second - time.Millisecond, minDayGap},
	}
	for _, c := range cases {
		if got := nextDayDelay(c.dayLength, c.elapsed); got != c.want {
			t.Errorf("%s: nextDayDelay(%v, %v) = %v; want %v", c.name, c.dayLength, c.elapsed, got, c.want)
		}
	}
}
