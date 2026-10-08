package sim

import "time"

// minDayGap is the pause between simulated days when the simulation runs
// flat out. It is a self-pacing wait after each day completes rather than a
// fixed-rate ticker: in WASM a ticker that cannot keep up starves the JS
// event loop and freezes the page.
const minDayGap = 200 * time.Millisecond

// nextDayDelay is how long the run loop waits after a day's work before
// starting the next day: whatever is left of the day length, never less
// than minDayGap.
func nextDayDelay(dayLength, elapsed time.Duration) time.Duration {
	return max(dayLength-elapsed, minDayGap)
}
