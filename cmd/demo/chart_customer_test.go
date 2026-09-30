package main

import (
	"regexp"
	"strconv"
	"testing"
	"time"
)

var svgYLabelRe = regexp.MustCompile(`<text [^>]*>(-?\d+(?:\.\d+)?)</text>`)

// The customer chart's leftmost and rightmost date labels are the first and
// last day of the history, and its count axis shows distinct whole numbers.
func TestCustomerChartAxes(t *testing.T) {
	start := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	for _, days := range []int{1, 9, 30, 120, 400, 1100} {
		var hist []CustomerPoint
		for i := 0; i <= days; i++ {
			hist = append(hist, CustomerPoint{Date: start.AddDate(0, 0, i), Count: 20 + i/3})
		}
		svg := buildCustomerChartSVG(hist)

		first, last := hist[0].Date.Format(chartDateFormat), hist[len(hist)-1].Date.Format(chartDateFormat)
		var leftmost, rightmost string
		minX, maxX := 1e9, -1e9
		for _, m := range svgTextRe.FindAllStringSubmatch(svg, -1) {
			x, _ := strconv.ParseFloat(m[1], 64)
			if x < minX {
				minX, leftmost = x, m[3]
			}
			if x > maxX {
				maxX, rightmost = x, m[3]
			}
		}
		if leftmost != first || rightmost != last {
			t.Errorf("%dd: date labels run %s..%s, want %s..%s", days, leftmost, rightmost, first, last)
		}

		seen := map[string]bool{}
		for _, m := range svgYLabelRe.FindAllStringSubmatch(svg, -1) {
			if _, err := strconv.Atoi(m[1]); err != nil {
				t.Errorf("%dd: y label %q is not a whole number", days, m[1])
			}
			if seen[m[1]] {
				t.Errorf("%dd: y label %q repeated", days, m[1])
			}
			seen[m[1]] = true
		}
		if len(seen) < 2 {
			t.Errorf("%dd: %d y labels, want at least 2", days, len(seen))
		}
	}
}
