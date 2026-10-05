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

// SetDayLength sets the wall-clock length of a simulated day, as the
// console does; zero is flat out. A negative value is refused. It applies
// to the day in progress — the run loop's idle wait is recomputed, so zero
// means the next day begins now — and is recorded with the run so the
// next process resumes at it.
func (ds *DemoState) SetDayLength(d time.Duration) {
	if d < 0 {
		return
	}
	ds.settings.Update(func(s *Settings) { s.DayLength = d })
	ds.mu.Lock()
	ds.dayLengthRecorded = true
	db := ds.db
	ds.mu.Unlock()
	select {
	case ds.dayLengthChanged <- struct{}{}:
	default: // one change already pending; the loop reads the setting when it wakes
	}
	saveDayLength(db, d)
}

// DefaultDayLength is the environment's day length (GOBANK_DAY_LENGTH):
// where a run starts until the console sets one. A run resumed with a
// recorded setting keeps it.
func (ds *DemoState) DefaultDayLength(d time.Duration) {
	ds.mu.Lock()
	recorded := ds.dayLengthRecorded
	ds.mu.Unlock()
	if recorded || d < 0 {
		return
	}
	ds.settings.Update(func(s *Settings) { s.DayLength = d })
}
