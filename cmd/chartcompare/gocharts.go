package main

import (
	"fmt"

	"github.com/go-analyze/charts"
)

func thinLabels(labels []string, max int) []string {
	if len(labels) <= max {
		return labels
	}
	out := make([]string, len(labels))
	copy(out, labels)
	step := len(out) / max
	for i := range out {
		if i%step != 0 && i != len(out)-1 {
			out[i] = ""
		}
	}
	return out
}

func gcCustomerCount(data []Point) string {
	values := make([]float64, len(data))
	labels := make([]string, len(data))
	for i, p := range data {
		values[i] = p.Value
		labels[i] = p.Date.Format("2 Jan")
	}
	labels = thinLabels(labels, 6)

	p, err := charts.LineRender(
		[][]float64{values},
		charts.SVGOutputOptionFunc(),
		charts.DimensionsOptionFunc(svgWidth, svgHeight),
		charts.XAxisLabelsOptionFunc(labels),
		charts.LegendOptionFunc(charts.LegendOption{Show: charts.Ptr(false)}),
		charts.PaddingOptionFunc(charts.Box{Left: 60, Right: 10, Top: 10, Bottom: 10, IsSet: true}),
		func(opt *charts.ChartOption) {
			opt.Symbol = charts.SymbolNone
			opt.LineStrokeWidth = 2
		},
	)
	if err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">Chart error: %v</p>`, err)
	}
	b, err := p.Bytes()
	if err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">Render error: %v</p>`, err)
	}
	return string(b)
}

func gcBalances(data []DualPoint) string {
	savings := make([]float64, len(data))
	lending := make([]float64, len(data))
	labels := make([]string, len(data))
	for i, p := range data {
		savings[i] = p.A
		lending[i] = p.B
		labels[i] = p.Date.Format("2 Jan")
	}
	labels = thinLabels(labels, 6)

	p, err := charts.LineRender(
		[][]float64{savings, lending},
		charts.SVGOutputOptionFunc(),
		charts.DimensionsOptionFunc(svgWidth, svgHeight+25),
		charts.XAxisLabelsOptionFunc(labels),
		charts.LegendOptionFunc(charts.LegendOption{
			Show:        charts.Ptr(true),
			SeriesNames: []string{"Savings", "Lending"},
		}),
		charts.PaddingOptionFunc(charts.Box{Left: 60, Right: 10, Top: 10, Bottom: 10, IsSet: true}),
		func(opt *charts.ChartOption) {
			opt.Symbol = charts.SymbolNone
			opt.LineStrokeWidth = 2
		},
	)
	if err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">Chart error: %v</p>`, err)
	}
	b, err := p.Bytes()
	if err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">Render error: %v</p>`, err)
	}
	return string(b)
}

func gcNIM(data []Point) string {
	values := make([]float64, len(data))
	labels := make([]string, len(data))
	for i, p := range data {
		values[i] = p.Value
		labels[i] = p.Date.Format("2 Jan")
	}
	labels = thinLabels(labels, 6)

	p, err := charts.LineRender(
		[][]float64{values},
		charts.SVGOutputOptionFunc(),
		charts.DimensionsOptionFunc(svgWidth, svgHeight),
		charts.XAxisLabelsOptionFunc(labels),
		charts.LegendOptionFunc(charts.LegendOption{Show: charts.Ptr(false)}),
		charts.PaddingOptionFunc(charts.Box{Left: 60, Right: 10, Top: 10, Bottom: 10, IsSet: true}),
		func(opt *charts.ChartOption) {
			opt.Symbol = charts.SymbolNone
			opt.LineStrokeWidth = 2
		},
	)
	if err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">Chart error: %v</p>`, err)
	}
	b, err := p.Bytes()
	if err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">Render error: %v</p>`, err)
	}
	return string(b)
}

func gcBoERate(data []Point) string {
	// Convert rate to percentage for display
	values := make([]float64, len(data))
	labels := make([]string, len(data))
	for i, p := range data {
		values[i] = p.Value * 100
		labels[i] = p.Date.Format("2 Jan")
	}
	labels = thinLabels(labels, 6)

	p, err := charts.LineRender(
		[][]float64{values},
		charts.SVGOutputOptionFunc(),
		charts.DimensionsOptionFunc(svgWidth, svgHeight),
		charts.XAxisLabelsOptionFunc(labels),
		charts.LegendOptionFunc(charts.LegendOption{Show: charts.Ptr(false)}),
		charts.PaddingOptionFunc(charts.Box{Left: 60, Right: 10, Top: 10, Bottom: 10, IsSet: true}),
		func(opt *charts.ChartOption) {
			opt.Symbol = charts.SymbolNone
			opt.LineStrokeWidth = 2
		},
	)
	if err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">Chart error: %v</p>`, err)
	}
	b, err := p.Bytes()
	if err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">Render error: %v</p>`, err)
	}
	return string(b)
}
