package main

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// TestChartAxisPreview writes sample customer charts to CHART_PREVIEW_DIR for
// visual inspection. Skipped unless the env var is set.
func TestChartAxisPreview(t *testing.T) {
	dir := os.Getenv("CHART_PREVIEW_DIR")
	if dir == "" {
		t.Skip("CHART_PREVIEW_DIR not set")
	}
	start := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	spans := []int{1, 9, 30, 120, 400, 1100}
	for _, days := range spans {
		var hist []CustomerPoint
		count := 20
		for i := 0; i <= days; i++ {
			if i%3 == 0 {
				count += i % 7
			}
			hist = append(hist, CustomerPoint{Date: start.AddDate(0, 0, i), Count: count})
		}
		svg := buildCustomerChartSVG(hist)
		path := fmt.Sprintf("%s/customer-%dd.svg", dir, days)
		if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d points, final count %d)", path, len(hist), count)
	}
}
