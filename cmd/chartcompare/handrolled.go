package main

import (
	"fmt"
	"math"
	"strings"
)

const (
	svgWidth  = 660
	chartPadL = 70
	chartPadR = 20
	chartW    = svgWidth - chartPadL - chartPadR
	chartH    = 160
	chartPadT = 25
	chartPadB = 30
	svgHeight = chartPadT + chartH + chartPadB
)

// hrSingleLine renders a single-line chart as a standalone SVG string.
func hrSingleLine(data []Point, color, yFmt string) string {
	if len(data) == 0 {
		return ""
	}

	minV, maxV := data[0].Value, data[0].Value
	for _, p := range data {
		if p.Value < minV {
			minV = p.Value
		}
		if p.Value > maxV {
			maxV = p.Value
		}
	}
	vRange := maxV - minV
	if vRange < 1e-9 {
		vRange = 1
		minV -= 0.5
	} else {
		minV -= vRange * 0.1
		maxV += vRange * 0.1
		vRange = maxV - minV
	}
	if minV < 0 {
		minV = 0
		vRange = maxV - minV
	}

	var s strings.Builder
	s.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg" style="width:100%%;height:auto">`, svgWidth, svgHeight))
	s.WriteString(`<style>text{font-family:Arial,Helvetica,sans-serif}</style>`)

	// Background
	s.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="#fafafa" stroke="#dbdbdb" stroke-width="1"/>`,
		chartPadL, chartPadT, chartW, chartH))

	// Y-axis
	for i := 0; i <= 4; i++ {
		val := minV + vRange*float64(i)/4.0
		y := float64(chartPadT+chartH) - float64(chartH)*float64(i)/4.0
		s.WriteString(fmt.Sprintf(`<line x1="%d" y1="%.0f" x2="%d" y2="%.0f" stroke="#ededed" stroke-width="1"/>`,
			chartPadL, y, chartPadL+chartW, y))
		s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" text-anchor="end" font-size="9" fill="#7a7a7a">`+yFmt+`</text>`,
			chartPadL-5, y+3, val))
	}

	// X-axis date labels
	xLabels(data, &s)

	// Line
	var pts strings.Builder
	for i, p := range data {
		x := float64(chartPadL) + float64(chartW)*float64(i)/float64(len(data)-1)
		y := float64(chartPadT+chartH) - float64(chartH)*(p.Value-minV)/vRange
		y = math.Max(float64(chartPadT), math.Min(float64(chartPadT+chartH), y))
		if i == 0 {
			pts.WriteString(fmt.Sprintf("%.1f,%.1f", x, y))
		} else {
			pts.WriteString(fmt.Sprintf(" %.1f,%.1f", x, y))
		}
	}
	s.WriteString(fmt.Sprintf(`<polyline points="%s" fill="none" stroke="%s" stroke-width="2"/>`, pts.String(), color))

	s.WriteString(`</svg>`)
	return s.String()
}

// hrDualLine renders a dual-line chart as a standalone SVG string.
func hrDualLine(data []DualPoint, colorA, colorB, labelA, labelB string) string {
	if len(data) == 0 {
		return ""
	}

	minV, maxV := data[0].A, data[0].A
	for _, p := range data {
		for _, v := range []float64{p.A, p.B} {
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
		}
	}
	vRange := maxV - minV
	if vRange < 100 {
		vRange = 200
		minV -= 100
	} else {
		minV -= vRange * 0.1
		maxV += vRange * 0.1
		vRange = maxV - minV
	}
	if minV < 0 {
		minV = 0
		vRange = maxV - minV
	}

	legendH := 25
	totalH := svgHeight + legendH

	var s strings.Builder
	s.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg" style="width:100%%;height:auto">`, svgWidth, totalH))
	s.WriteString(`<style>text{font-family:Arial,Helvetica,sans-serif}</style>`)

	// Background
	s.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="#fafafa" stroke="#dbdbdb" stroke-width="1"/>`,
		chartPadL, chartPadT, chartW, chartH))

	// Y-axis
	for i := 0; i <= 4; i++ {
		val := minV + vRange*float64(i)/4.0
		y := float64(chartPadT+chartH) - float64(chartH)*float64(i)/4.0
		s.WriteString(fmt.Sprintf(`<line x1="%d" y1="%.0f" x2="%d" y2="%.0f" stroke="#ededed" stroke-width="1"/>`,
			chartPadL, y, chartPadL+chartW, y))
		s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" text-anchor="end" font-size="9" fill="#7a7a7a">%.0f</text>`,
			chartPadL-5, y+3, val))
	}

	// X-axis date labels (reuse from A dates)
	singlePts := make([]Point, len(data))
	for i, p := range data {
		singlePts[i] = Point{Date: p.Date}
	}
	xLabels(singlePts, &s)

	// Lines
	for _, spec := range []struct {
		color string
		val   func(DualPoint) float64
	}{
		{colorA, func(p DualPoint) float64 { return p.A }},
		{colorB, func(p DualPoint) float64 { return p.B }},
	} {
		var pts strings.Builder
		for i, p := range data {
			v := spec.val(p)
			x := float64(chartPadL) + float64(chartW)*float64(i)/float64(len(data)-1)
			y := float64(chartPadT+chartH) - float64(chartH)*(v-minV)/vRange
			y = math.Max(float64(chartPadT), math.Min(float64(chartPadT+chartH), y))
			if i == 0 {
				pts.WriteString(fmt.Sprintf("%.1f,%.1f", x, y))
			} else {
				pts.WriteString(fmt.Sprintf(" %.1f,%.1f", x, y))
			}
		}
		s.WriteString(fmt.Sprintf(`<polyline points="%s" fill="none" stroke="%s" stroke-width="2"/>`, pts.String(), spec.color))
	}

	// Legend
	ly := float64(chartPadT+chartH+chartPadB) + 5
	s.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%.0f" r="4" fill="%s"/>`, chartPadL, ly, colorA))
	s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" font-size="10" fill="#363636">%s</text>`, chartPadL+8, ly+4, labelA))
	s.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%.0f" r="4" fill="%s"/>`, chartPadL+90, ly, colorB))
	s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" font-size="10" fill="#363636">%s</text>`, chartPadL+98, ly+4, labelB))

	s.WriteString(`</svg>`)
	return s.String()
}

// hrStepLine renders a step-function chart (horizontal segments then vertical jumps).
func hrStepLine(data []Point, color, yFmt string) string {
	if len(data) == 0 {
		return ""
	}

	minV, maxV := data[0].Value, data[0].Value
	for _, p := range data {
		if p.Value < minV {
			minV = p.Value
		}
		if p.Value > maxV {
			maxV = p.Value
		}
	}
	vRange := maxV - minV
	if vRange < 1e-9 {
		vRange = 0.01
		minV -= 0.005
	} else {
		minV -= vRange * 0.1
		maxV += vRange * 0.1
		vRange = maxV - minV
	}
	if minV < 0 {
		minV = 0
		vRange = maxV - minV
	}

	var s strings.Builder
	s.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg" style="width:100%%;height:auto">`, svgWidth, svgHeight))
	s.WriteString(`<style>text{font-family:Arial,Helvetica,sans-serif}</style>`)

	// Background
	s.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="#fafafa" stroke="#dbdbdb" stroke-width="1"/>`,
		chartPadL, chartPadT, chartW, chartH))

	// Y-axis
	for i := 0; i <= 4; i++ {
		val := minV + vRange*float64(i)/4.0
		y := float64(chartPadT+chartH) - float64(chartH)*float64(i)/4.0
		s.WriteString(fmt.Sprintf(`<line x1="%d" y1="%.0f" x2="%d" y2="%.0f" stroke="#ededed" stroke-width="1"/>`,
			chartPadL, y, chartPadL+chartW, y))
		s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" text-anchor="end" font-size="9" fill="#7a7a7a">`+yFmt+`</text>`,
			chartPadL-5, y+3, val*100))
	}

	// X-axis date labels
	xLabels(data, &s)

	// Step line: horizontal to next x, then vertical to next y
	var pts strings.Builder
	for i, p := range data {
		x := float64(chartPadL) + float64(chartW)*float64(i)/float64(len(data)-1)
		y := float64(chartPadT+chartH) - float64(chartH)*(p.Value-minV)/vRange
		y = math.Max(float64(chartPadT), math.Min(float64(chartPadT+chartH), y))
		if i == 0 {
			pts.WriteString(fmt.Sprintf("%.1f,%.1f", x, y))
		} else {
			// Horizontal segment first (same y as previous), then step to new y
			prevY := float64(chartPadT+chartH) - float64(chartH)*(data[i-1].Value-minV)/vRange
			prevY = math.Max(float64(chartPadT), math.Min(float64(chartPadT+chartH), prevY))
			pts.WriteString(fmt.Sprintf(" %.1f,%.1f %.1f,%.1f", x, prevY, x, y))
		}
	}
	s.WriteString(fmt.Sprintf(`<polyline points="%s" fill="none" stroke="%s" stroke-width="2"/>`, pts.String(), color))

	s.WriteString(`</svg>`)
	return s.String()
}

// xLabels writes 5 evenly-spaced date labels along the x-axis.
func xLabels(data []Point, s *strings.Builder) {
	if len(data) < 2 {
		return
	}
	numLabels := min(len(data), 5)
	for i := range numLabels {
		idx := i * (len(data) - 1) / (numLabels - 1)
		x := float64(chartPadL) + float64(chartW)*float64(idx)/float64(len(data)-1)
		s.WriteString(fmt.Sprintf(`<text x="%.0f" y="%d" text-anchor="middle" font-size="9" fill="#7a7a7a">%s</text>`,
			x, chartPadT+chartH+15, data[idx].Date.Format("2 Jan")))
	}
}

// Hand-rolled chart functions for each dataset.

func hrCustomerCount(data []Point) string {
	return hrSingleLine(data, "#00947e", "%.0f")
}

func hrBalances(data []DualPoint) string {
	return hrDualLine(data, "#48c78e", "#3e8ed0", "Savings", "Lending")
}

func hrNIM(data []Point) string {
	return hrSingleLine(data, "#f59e0b", "%.0f")
}

func hrBoERate(data []Point) string {
	return hrStepLine(data, "#48c78e", "%.2f%%")
}
