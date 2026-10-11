package sim

import "time"

// minDayGap is the pause before a start of day when the simulation runs
// flat out. It is a self-pacing wait rather than a fixed-rate ticker: in
// WASM a ticker that cannot keep up starves the JS event loop and
// freezes the page. The events inside a day follow at once.
const minDayGap = 200 * time.Millisecond

// pace is the wall time the simulation spends from one event to the next
// (ADR-0010): the day length's share of the simulated gap between them,
// less the wall time already spent since the last tick, never negative.
// Flat out it is the gap before a start of day alone.
func pace(dayLength, gap, spent time.Duration, startOfDay bool) time.Duration {
	if dayLength <= 0 {
		if startOfDay {
			return minDayGap
		}
		return 0
	}
	share := time.Duration(float64(dayLength) * float64(gap) / float64(24*time.Hour))
	return max(share-spent, 0)
}
