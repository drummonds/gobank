package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// minDayGap is the pause between simulated days when the simulation runs
// flat out. It is a self-pacing wait after each day completes rather than a
// fixed-rate ticker: in WASM a ticker that cannot keep up starves the JS
// event loop and freezes the page.
const minDayGap = 200 * time.Millisecond

// parseDayLength reads a simulated day's wall-clock length, as
// GOBANK_DAY_LENGTH or the settings page give it: a Go duration such as
// "2h" or "90m". Empty or "0" is flat out.
func parseDayLength(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("day length %q: want a duration like 2h or 90m, or 0 for flat out", s)
	}
	return d, nil
}

// dayLengthFromEnv is the configured day length, or an error the caller
// should refuse to start on.
func dayLengthFromEnv() (time.Duration, error) {
	return parseDayLength(os.Getenv("GOBANK_DAY_LENGTH"))
}

// nextDayDelay is how long the run loop waits after a day's work before
// starting the next day: whatever is left of the day length, never less
// than minDayGap.
func nextDayDelay(dayLength, elapsed time.Duration) time.Duration {
	return max(dayLength-elapsed, minDayGap)
}

// SetDayLength sets the wall-clock length of a simulated day; zero is flat
// out. A negative value is refused. Takes effect from the next day.
func (ds *DemoState) SetDayLength(d time.Duration) {
	if d < 0 {
		return
	}
	ds.mu.Lock()
	ds.settings.DayLength = d
	ds.mu.Unlock()
}

// DayLength is the wall-clock length of a simulated day; zero is flat out.
func (ds *DemoState) DayLength() time.Duration {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.settings.DayLength
}
