package main

import (
	"math"
	"time"
)

// Point is a single-value time series point.
type Point struct {
	Date  time.Time
	Value float64
}

// DualPoint is a two-value time series point.
type DualPoint struct {
	Date time.Time
	A, B float64
}

var startDate = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

// customerData generates ~50 points of growing customer count (5→55).
func customerData() []Point {
	pts := make([]Point, 50)
	for i := range pts {
		pts[i] = Point{
			Date:  startDate.AddDate(0, 0, i),
			Value: 5 + float64(i) + 2*math.Sin(float64(i)/5),
		}
	}
	return pts
}

// balanceData generates ~50 points of dual-line savings/lending.
func balanceData() []DualPoint {
	pts := make([]DualPoint, 50)
	for i := range pts {
		day := float64(i)
		pts[i] = DualPoint{
			Date: startDate.AddDate(0, 0, i),
			A:    10000 + day*200 + 500*math.Sin(day/7), // savings
			B:    5000 + day*100 + 300*math.Cos(day/10), // lending
		}
	}
	return pts
}

// nimData generates ~50 points of volatile NIM in basis points (100-300).
func nimData() []Point {
	pts := make([]Point, 50)
	for i := range pts {
		day := float64(i)
		pts[i] = Point{
			Date:  startDate.AddDate(0, 0, i),
			Value: 200 + 80*math.Sin(day/4) + 30*math.Cos(day/7),
		}
	}
	return pts
}

// boeRateData generates ~50 points of step-function BoE rate (0.10%→5.25%).
func boeRateData() []Point {
	steps := []struct {
		dayOffset int
		rate      float64
	}{
		{0, 0.0010},
		{8, 0.0025},
		{16, 0.0050},
		{22, 0.0100},
		{28, 0.0175},
		{32, 0.0250},
		{36, 0.0350},
		{40, 0.0425},
		{44, 0.0500},
		{47, 0.0525},
	}

	pts := make([]Point, 50)
	stepIdx := 0
	for i := range pts {
		if stepIdx+1 < len(steps) && i >= steps[stepIdx+1].dayOffset {
			stepIdx++
		}
		pts[i] = Point{
			Date:  startDate.AddDate(0, 0, i),
			Value: steps[stepIdx].rate,
		}
	}
	return pts
}
