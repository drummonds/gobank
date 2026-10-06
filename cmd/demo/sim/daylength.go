package sim

import (
	"fmt"
	"strings"
	"time"
)

// minDayGap is the pause between simulated days when the simulation runs
// flat out. It is a self-pacing wait after each day completes rather than a
// fixed-rate ticker: in WASM a ticker that cannot keep up starves the JS
// event loop and freezes the page.
const minDayGap = 200 * time.Millisecond

// ParseDayLength reads a simulated day's wall-clock length, as
// GOBANK_DAY_LENGTH or the settings page give it: a Go duration such as
// "2h" or "90m". Empty or "0" is flat out.
func ParseDayLength(s string) (time.Duration, error) {
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

// nextDayDelay is how long the run loop waits after a day's work before
// starting the next day: whatever is left of the day length, never less
// than minDayGap.
func nextDayDelay(dayLength, elapsed time.Duration) time.Duration {
	return max(dayLength-elapsed, minDayGap)
}
