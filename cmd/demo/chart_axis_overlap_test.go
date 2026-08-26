package main

import (
	"regexp"
	"strconv"
	"testing"
	"time"
)

// TestChartAxisLabelOverlapScan renders the customer chart for every span from
// 1 to 1500 days and fails if any two x-axis date labels overlap horizontally.
var svgTextRe = regexp.MustCompile(`<text x="(-?\d+)" y="(\d+)"[^>]*>(\d{4}-\d{2}-\d{2})</text>`)

func TestChartAxisLabelOverlapScan(t *testing.T) {
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	const approxCharW = 7 // ~font-size 12 digit width in px
	seen := 0
	for days := 1; days <= 1500; days++ {
		hist := []CustomerPoint{}
		for i := 0; i <= days; i++ {
			hist = append(hist, CustomerPoint{Date: start.AddDate(0, 0, i), Count: 20 + i/3})
		}
		svg := buildCustomerChartSVG(hist)
		type lbl struct {
			x, y int
			text string
		}
		var labels []lbl
		for _, m := range svgTextRe.FindAllStringSubmatch(svg, -1) {
			x, _ := strconv.Atoi(m[1])
			y, _ := strconv.Atoi(m[2])
			labels = append(labels, lbl{x, y, m[3]})
		}
		for i := 0; i < len(labels); i++ {
			for j := i + 1; j < len(labels); j++ {
				a, b := labels[i], labels[j]
				if a.y != b.y {
					continue // different rows can't collide
				}
				aw := len(a.text) * approxCharW
				bw := len(b.text) * approxCharW
				if a.x < b.x+bw && b.x < a.x+aw {
					seen++
					if seen <= 10 {
						t.Errorf("span %dd: %q@x=%d overlaps %q@x=%d", days, a.text, a.x, b.text, b.x)
					}
				}
			}
		}
	}
	if seen > 10 {
		t.Errorf("... and %d more overlaps", seen-10)
	}
}
