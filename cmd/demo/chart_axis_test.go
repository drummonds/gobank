package main

import (
	"testing"
	"time"
)

func TestIntegerYAxis(t *testing.T) {
	cases := []struct {
		min, max   int
		wantMin    float64
		wantMax    float64
		wantLabels int
	}{
		{20, 20, 20, 21, 2},  // flat data still gets a 1-unit range
		{20, 52, 20, 60, 5},  // step 10
		{20, 142, 0, 150, 4}, // step 50
		{20, 419, 0, 500, 6}, // step 100
		{3, 8, 2, 10, 5},     // step 2
		{0, 3, 0, 4, 5},      // step 1
	}
	for _, c := range cases {
		gotMin, gotMax, gotLabels := integerYAxis(c.min, c.max)
		if gotMin != c.wantMin || gotMax != c.wantMax || gotLabels != c.wantLabels {
			t.Errorf("integerYAxis(%d, %d) = (%v, %v, %d), want (%v, %v, %d)",
				c.min, c.max, gotMin, gotMax, gotLabels, c.wantMin, c.wantMax, c.wantLabels)
		}
	}
}

func TestTimeAxisTicks(t *testing.T) {
	t0 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	for _, days := range []int{1, 9, 30, 120, 400, 1100} {
		t1 := t0.AddDate(0, 0, days)
		ticks := timeAxisTicks(t0, t1)

		var labeled []float64
		for _, tick := range ticks {
			if tick.Position < 0 || tick.Position > 1 {
				t.Errorf("%dd: tick position %v out of range", days, tick.Position)
			}
			if tick.Label != "" {
				labeled = append(labeled, tick.Position)
				if tick.Minor {
					t.Errorf("%dd: labeled tick at %v marked minor", days, tick.Position)
				}
			}
		}
		if ticks[0].Position != 0 {
			t.Errorf("%dd: first tick = %+v, want position 0", days, ticks[0])
		}
		// each end is anchored by a label at or essentially at the edge:
		// either the pinned end date or a near-coincident calendar boundary
		startAnchored, endAnchored := false, false
		for _, f := range labeled {
			if f <= 0.1 {
				startAnchored = true
			}
			if f >= 0.9 {
				endAnchored = true
			}
		}
		if !startAnchored || !endAnchored {
			t.Errorf("%dd: axis ends not anchored by labels (start %v, end %v)", days, startAnchored, endAnchored)
		}
		if len(labeled) > 6 {
			t.Errorf("%dd: %d labels, want <= 6", days, len(labeled))
		}
	}

	// degenerate single-day history
	single := timeAxisTicks(t0, t0)
	if len(single) != 1 || single[0].Label != "2026-01-15" {
		t.Errorf("zero span = %+v, want single start label", single)
	}
}
