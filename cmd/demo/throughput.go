package main

import "time"

// passWindow is the batch window the pass's throughput is quoted against:
// a bank's overnight run. "Account days per 12h" says whether the pass, at
// its measured rate, could project every account's day overnight. The
// rate is measured over the whole simulated day — closing yesterday's
// books and visiting every account — the span the runtime page reports as
// the last day's duration.
const passWindow = 12 * time.Hour

// throughputDays is how many recent simulated days the rate is averaged
// over.
const throughputDays = 10

type daySample struct {
	accounts int
	elapsed  time.Duration
}

// passThroughput is a rolling record of how many accounts each simulated
// day's pass visited and how long the whole day took.
type passThroughput struct {
	samples []daySample
}

func (t *passThroughput) record(accounts int, elapsed time.Duration) {
	t.samples = append(t.samples, daySample{accounts, elapsed})
	if len(t.samples) > throughputDays {
		t.samples = t.samples[len(t.samples)-throughputDays:]
	}
}

// per returns the accounts the pass would visit in window at the rate
// measured over the recent days, or 0 with nothing measured.
func (t *passThroughput) per(window time.Duration) int64 {
	var accounts int
	var elapsed time.Duration
	for _, s := range t.samples {
		accounts += s.accounts
		elapsed += s.elapsed
	}
	if elapsed <= 0 {
		return 0
	}
	return int64(float64(accounts) * float64(window) / float64(elapsed))
}

// perSecond is a plain rate for a count over an elapsed time, 0 if no time
// has passed.
func perSecond(count int, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	return float64(count) / elapsed.Seconds()
}
