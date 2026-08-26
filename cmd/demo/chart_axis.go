package main

import (
	"time"

	"github.com/go-analyze/charts"
)

// chartDateFormat is the ISO date format used on time axis labels.
const chartDateFormat = "2006-01-02"

// integerYAxis returns axis bounds and label count so that every y-axis label
// lands on a whole number: the range is snapped to multiples of a 1/2/5-style
// integer step with one step of headroom above the data.
func integerYAxis(minVal, maxVal int) (yMin, yMax float64, labelCount int) {
	if maxVal < minVal {
		minVal, maxVal = maxVal, minVal
	}
	step := niceIntStep(maxVal - minVal)
	lo := (minVal / step) * step
	if minVal < 0 && minVal%step != 0 {
		lo -= step
	}
	hi := ((maxVal + step) / step) * step // headroom so the line doesn't touch the frame
	return float64(lo), float64(hi), (hi-lo)/step + 1
}

// niceIntStep picks the smallest step from 1/2/5 times a power of ten that
// divides r into at most 4 intervals.
func niceIntStep(r int) int {
	if r <= 4 {
		return 1
	}
	for mag := 1; ; mag *= 10 {
		for _, m := range []int{1, 2, 5} {
			if step := m * mag; 4*step >= r {
				return step
			}
		}
	}
}

// timeAxisTicks builds x-axis ticks for the span [t0, t1]: labeled major ticks
// pinned at the first and last date plus calendar-boundary dates between them,
// and short minor ticks at day/week/month boundaries depending on the span.
func timeAxisTicks(t0, t1 time.Time) []charts.CustomTick {
	if !t1.After(t0) {
		return []charts.CustomTick{{Position: 0, Label: t0.Format(chartDateFormat)}}
	}
	span := t1.Sub(t0)
	frac := func(t time.Time) float64 { return float64(t.Sub(t0)) / float64(span) }
	spanDays := int(span.Hours() / 24)

	var labelUnit calUnit
	switch {
	case spanDays <= 4:
		labelUnit = calDay
	case spanDays <= 35:
		labelUnit = calWeek
	case spanDays <= 180:
		labelUnit = calMonth
	case spanDays <= 540:
		labelUnit = calQuarter
	default:
		labelUnit = calYear
	}

	// Mid labels at calendar boundaries, thinned to at most 4.
	var mids []time.Time
	for t := nextBoundary(t0, labelUnit); t.Before(t1); t = nextBoundary(t, labelUnit) {
		mids = append(mids, t)
	}
	labelStep := (len(mids) + 3) / 4

	// Collision handling in approximate pixel space. End labels are preferred,
	// but when a calendar boundary lands essentially on top of an end the
	// boundary label wins and the end label is dropped (the renderer clamps
	// the boundary label to the axis edge).
	const (
		axisPx      = 520.0           // approximate plot width of the 660px-wide chart
		labelPx     = 70.0            // ~10-char YYYY-mm-dd label width
		crowdPx     = labelPx*1.5 + 6 // mid centered this close to an end overlaps its label
		veryClosePx = labelPx * 0.6   // effectively coincident; the boundary label wins
	)
	showStart, showEnd := true, true
	var midTicks []charts.CustomTick
	for i, t := range mids {
		if i%labelStep != 0 {
			continue
		}
		f := frac(t)
		fromStart := f * axisPx
		fromEnd := (1 - f) * axisPx
		switch {
		case fromStart < veryClosePx:
			showStart = false
		case fromEnd < veryClosePx:
			showEnd = false
		case fromStart < crowdPx || fromEnd < crowdPx:
			continue // would crowd a pinned end label
		}
		midTicks = append(midTicks, charts.CustomTick{Position: f, Label: t.Format(chartDateFormat)})
	}

	startTick := charts.CustomTick{Position: 0}
	if showStart {
		startTick.Label = t0.Format(chartDateFormat)
	}
	endTick := charts.CustomTick{Position: 1}
	if showEnd {
		endTick.Label = t1.Format(chartDateFormat)
	}
	ticks := make([]charts.CustomTick, 0, len(midTicks)+2)
	ticks = append(ticks, startTick)
	ticks = append(ticks, midTicks...)
	ticks = append(ticks, endTick)
	labeled := make([]float64, 0, len(ticks))
	for _, t := range ticks {
		labeled = append(labeled, t.Position)
	}

	// Minor ticks at the finest calendar unit that stays legible (~5px apart
	// on the ~560px axis).
	minorUnit := calDay
	for ; minorUnit < calYear; minorUnit++ {
		if countBoundaries(t0, t1, minorUnit) <= 100 {
			break
		}
	}
	for t := nextBoundary(t0, minorUnit); t.Before(t1); t = nextBoundary(t, minorUnit) {
		f := frac(t)
		tooClose := false
		for _, lf := range labeled {
			if diff := f - lf; diff > -0.005 && diff < 0.005 {
				tooClose = true
				break
			}
		}
		if !tooClose {
			ticks = append(ticks, charts.CustomTick{Position: f, Minor: true})
		}
	}
	return ticks
}

type calUnit int

const (
	calDay calUnit = iota
	calWeek
	calMonth
	calQuarter
	calYear
)

// nextBoundary returns the first calendar boundary of the given unit strictly
// after t (weeks start on Monday).
func nextBoundary(t time.Time, unit calUnit) time.Time {
	y, m, d := t.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	switch unit {
	case calDay:
		return midnight.AddDate(0, 0, 1)
	case calWeek:
		next := midnight.AddDate(0, 0, (8-int(midnight.Weekday()))%7)
		if !next.After(t) {
			next = next.AddDate(0, 0, 7)
		}
		return next
	case calMonth:
		return time.Date(y, m, 1, 0, 0, 0, 0, t.Location()).AddDate(0, 1, 0)
	case calQuarter:
		qStartMonth := time.Month(((int(m)-1)/3)*3 + 1)
		return time.Date(y, qStartMonth, 1, 0, 0, 0, 0, t.Location()).AddDate(0, 3, 0)
	default: // calYear
		return time.Date(y+1, time.January, 1, 0, 0, 0, 0, t.Location())
	}
}

// countBoundaries counts unit boundaries strictly inside (t0, t1).
func countBoundaries(t0, t1 time.Time, unit calUnit) int {
	n := 0
	for t := nextBoundary(t0, unit); t.Before(t1); t = nextBoundary(t, unit) {
		n++
	}
	return n
}
