package main

// Spike: could gogal replace go-analyze/charts (and its local fork) for the
// customer chart? Renders the same histories through both and writes them
// side by side, with what each puts on its axes, for a visual and measured
// comparison. Skipped unless CHART_PREVIEW_DIR is set; not part of the
// suite.

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/gogal"
)

// gogalCustomerChartSVG is the customer chart as gogal draws it today, with
// the nearest options to the current chart: same size and padding, ISO
// dates, integer y labels, no legend, no tooltips.
func gogalCustomerChartSVG(history []CustomerPoint) string {
	times := make([]time.Time, len(history))
	values := make([]float64, len(history))
	for i, cp := range history {
		times[i], values[i] = cp.Date, float64(cp.Count)
	}
	svg, err := gogal.NewLineChart(
		gogal.WithSize(660, 180),
		gogal.WithMargins(10, 40, 24, 60),
		gogal.WithLegend(false),
		gogal.WithTooltips(false),
		gogal.WithAccessibility(false),
		gogal.WithTimeFormat(chartDateFormat),
		gogal.WithYFormat("%.0f"),
		gogal.WithLocation(time.UTC),
	).AddTimeSeries("customers", times, values).RenderString()
	if err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">gogal: %v</p>`, err)
	}
	return svg
}

var (
	spikeDateLabel = regexp.MustCompile(`<text x="(-?[\d.]+)" y="[\d.]+"[^>]*>(\d{4}-\d{2}-\d{2})</text>`)
	spikeYLabel    = regexp.MustCompile(`<text x="[\d.]+" y="[\d.]+" text-anchor="end"[^>]*>(-?\d+)</text>`)
)

func spikeHistory(start time.Time, days int) []CustomerPoint {
	var hist []CustomerPoint
	count := 20
	for i := 0; i <= days; i++ {
		if i%3 == 0 {
			count += i % 7
		}
		hist = append(hist, CustomerPoint{Date: start.AddDate(0, 0, i), Count: count})
	}
	return hist
}

func TestChartGogalSpike(t *testing.T) {
	dir := os.Getenv("CHART_PREVIEW_DIR")
	if dir == "" {
		t.Skip("CHART_PREVIEW_DIR not set")
	}
	start := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	var page strings.Builder
	page.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>gogal spike</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bulma@1.0.4/css/bulma.min.css"></head><body><section class="section">
<h1 class="title">Customer chart: go-analyze/charts (fork) vs gogal v0.1.8</h1>`)
	for _, days := range []int{1, 9, 30, 120, 400, 1100} {
		hist := spikeHistory(start, days)
		current := buildCustomerChartSVG(hist)
		spike := gogalCustomerChartSVG(hist)
		for name, svg := range map[string]string{"current": current, "gogal": spike} {
			path := fmt.Sprintf("%s/spike-%dd-%s.svg", dir, days, name)
			if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		var xLabels, yLabels []string
		for _, m := range spikeDateLabel.FindAllStringSubmatch(spike, -1) {
			xLabels = append(xLabels, m[2]+"@"+m[1])
		}
		for _, m := range spikeYLabel.FindAllStringSubmatch(spike, -1) {
			yLabels = append(yLabels, m[1])
		}
		t.Logf("%4dd  %s..%s  current %6d bytes, gogal %7d bytes", days,
			hist[0].Date.Format(chartDateFormat), hist[len(hist)-1].Date.Format(chartDateFormat), len(current), len(spike))
		t.Logf("      gogal x: %s", strings.Join(xLabels, " "))
		t.Logf("      gogal y: %s  (data %d..%d)", strings.Join(yLabels, " "), hist[0].Count, hist[len(hist)-1].Count)
		fmt.Fprintf(&page, `<h2 class="title is-5 mt-5">%d days: %s to %s</h2><div class="columns"><div class="column"><p class="is-size-7">go-analyze/charts</p>%s</div><div class="column"><p class="is-size-7">gogal</p>%s</div></div>`,
			days, hist[0].Date.Format(chartDateFormat), hist[len(hist)-1].Date.Format(chartDateFormat), current, spike)
	}
	page.WriteString(`</section></body></html>`)
	if err := os.WriteFile(dir+"/spike-gogal.html", []byte(page.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	// The current chart's label-overlap scan, over gogal's output.
	const approxCharW = 7
	overlaps, unanchored := 0, 0
	for days := 1; days <= 1500; days++ {
		svg := gogalCustomerChartSVG(spikeHistory(start, days))
		var xs []float64
		for _, m := range spikeDateLabel.FindAllStringSubmatch(svg, -1) {
			var x float64
			fmt.Sscanf(m[1], "%g", &x)
			xs = append(xs, x)
		}
		for i := range xs {
			for j := i + 1; j < len(xs); j++ {
				if d := xs[i] - xs[j]; d < 10*approxCharW && d > -10*approxCharW {
					overlaps++
				}
			}
		}
		// the current chart anchors both ends with a label within 10% of the edge (axis x 60..620)
		anchoredStart, anchoredEnd := false, false
		for _, x := range xs {
			if x <= 60+56 {
				anchoredStart = true
			}
			if x >= 620-56 {
				anchoredEnd = true
			}
		}
		if !anchoredStart || !anchoredEnd {
			unanchored++
		}
	}
	t.Logf("spans 1..1500d: %d label overlaps, %d spans without a label near both ends", overlaps, unanchored)
}
