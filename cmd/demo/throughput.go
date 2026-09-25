package main

import "time"

// interestWindow is the batch window the interest throughput is quoted
// against: a bank's overnight run. "Movements per 12h" says whether this
// engine, at its measured rate, could post a day's interest overnight.
const interestWindow = 12 * time.Hour

// throughputDays is how many recent simulated days the interest rate is
// averaged over.
const throughputDays = 10

type daySample struct {
	movements int
	elapsed   time.Duration
}

// interestThroughput is a rolling record of how many interest movements
// each simulated day produced and how long the day's accrual phase took.
type interestThroughput struct {
	samples []daySample
}

func (t *interestThroughput) record(movements int, elapsed time.Duration) {
	t.samples = append(t.samples, daySample{movements, elapsed})
	if len(t.samples) > throughputDays {
		t.samples = t.samples[len(t.samples)-throughputDays:]
	}
}

// per returns the movements the engine would post in window at the rate
// measured over the recent days, or 0 with nothing measured.
func (t *interestThroughput) per(window time.Duration) int64 {
	var movements int
	var elapsed time.Duration
	for _, s := range t.samples {
		movements += s.movements
		elapsed += s.elapsed
	}
	if elapsed <= 0 {
		return 0
	}
	return int64(float64(movements) * float64(window) / float64(elapsed))
}

// perSecond is a plain rate for a count over an elapsed time, 0 if no time
// has passed.
func perSecond(count int, elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	return float64(count) / elapsed.Seconds()
}
