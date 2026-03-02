package main

import (
	"bytes"
	"fmt"

	m "github.com/erkkah/margaid"
)

func mgSingleLine(data []Point, title string) string {
	series := m.NewSeries(m.Titled(title))
	for _, p := range data {
		series.Add(m.MakeValue(m.SecondsFromTime(p.Date), p.Value))
	}

	diagram := m.New(svgWidth, svgHeight,
		m.WithAutorange(m.XAxis, series),
		m.WithAutorange(m.YAxis, series),
		m.WithInset(60),
		m.WithColorScheme(160),
	)

	diagram.Line(series, m.UsingStrokeWidth(2))
	diagram.Axis(series, m.XAxis, diagram.TimeTicker("2 Jan"), false, "")
	diagram.Axis(series, m.YAxis, diagram.ValueTicker('f', 0, 10), true, "")
	diagram.Frame()

	var buf bytes.Buffer
	if err := diagram.Render(&buf); err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">margaid error: %v</p>`, err)
	}
	return buf.String()
}

func mgDualLine(data []DualPoint, labelA, labelB string) string {
	seriesA := m.NewSeries(m.Titled(labelA))
	seriesB := m.NewSeries(m.Titled(labelB))
	for _, p := range data {
		t := m.SecondsFromTime(p.Date)
		seriesA.Add(m.MakeValue(t, p.A))
		seriesB.Add(m.MakeValue(t, p.B))
	}

	diagram := m.New(svgWidth, svgHeight+25,
		m.WithAutorange(m.XAxis, seriesA),
		m.WithAutorange(m.YAxis, seriesA, seriesB),
		m.WithInset(60),
		m.WithColorScheme(160),
	)

	diagram.Line(seriesA, m.UsingStrokeWidth(2))
	diagram.Line(seriesB, m.UsingStrokeWidth(2))
	diagram.Axis(seriesA, m.XAxis, diagram.TimeTicker("2 Jan"), false, "")
	diagram.Axis(seriesA, m.YAxis, diagram.ValueTicker('f', 0, 10), true, "")
	diagram.Frame()
	diagram.Legend(m.BottomLeft)

	var buf bytes.Buffer
	if err := diagram.Render(&buf); err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">margaid error: %v</p>`, err)
	}
	return buf.String()
}

// mgStepLine synthesizes a step chart by doubling points.
func mgStepLine(data []Point, title string) string {
	series := m.NewSeries(m.Titled(title))
	for i, p := range data {
		t := m.SecondsFromTime(p.Date)
		if i > 0 {
			// Horizontal segment: previous value at current x
			series.Add(m.MakeValue(t, data[i-1].Value*100))
		}
		series.Add(m.MakeValue(t, p.Value*100))
	}

	diagram := m.New(svgWidth, svgHeight,
		m.WithAutorange(m.XAxis, series),
		m.WithAutorange(m.YAxis, series),
		m.WithInset(60),
		m.WithColorScheme(160),
	)

	diagram.Line(series, m.UsingStrokeWidth(2))
	diagram.Axis(series, m.XAxis, diagram.TimeTicker("2 Jan"), false, "")
	diagram.Axis(series, m.YAxis, diagram.ValueTicker('f', 2, 10), true, "")
	diagram.Frame()

	var buf bytes.Buffer
	if err := diagram.Render(&buf); err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">margaid error: %v</p>`, err)
	}
	return buf.String()
}

func mgCustomerCount(data []Point) string {
	return mgSingleLine(data, "Customers")
}

func mgBalances(data []DualPoint) string {
	return mgDualLine(data, "Savings", "Lending")
}

func mgNIM(data []Point) string {
	return mgSingleLine(data, "NIM (bps)")
}

func mgBoERate(data []Point) string {
	return mgStepLine(data, "BoE Rate")
}
