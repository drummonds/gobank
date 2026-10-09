package staff

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/gogal"
)

// The daily series are the core's history types (ADR-0002 stage 1),
// stored as daily snapshots (bank/history).
type (
	RatePoint     = core.RatePoint
	BalancePoint  = core.BalancePoint
	CustomerPoint = core.CustomerPoint
	NIMPoint      = core.NIMPoint
)

// BuildDashboardHTML renders the dashboard's data sections, without the
// console's controls.
func BuildDashboardHTML(d DashData) string {
	var s strings.Builder

	// B5: Memory warning banner
	if d.Sim.MemoryExceeded {
		s.WriteString(`<div class="notification is-danger"><strong>Memory limit approaching.</strong> Simulation paused. Export data or reset to continue.</div>`)
	}

	// Summary stats level
	dateStr := d.Bank.Day.Format("2 Jan 2006")
	nimStr := fmt.Sprintf("%.0f bps", d.Bank.NIMBps)
	s.WriteString(`<nav class="level mb-4">`)
	countdown := ""
	if d.Sim.DayEndsIn > 0 {
		countdown = fmt.Sprintf(`<p class="heading">ends in %s</p>`, d.Sim.DayEndsIn.Round(time.Second))
	}
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Day</p><p class="title is-5">%d &mdash; %s</p>%s</div></div>`, d.Bank.DayCount, dateStr, countdown))
	// The two clocks side by side give a feel for the rate simulated time
	// passes at.
	warp := `<p class="heading">flat out</p>`
	if d.Sim.Warp > 0 {
		warp = fmt.Sprintf(`<p class="heading">&times;%s wall pace</p>`, strconv.FormatFloat(d.Sim.Warp, 'f', -1, 64))
	}
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Wall clock</p><p class="title is-5">%s</p></div></div>`, d.Sim.Wall.UTC().Format("15:04:05")))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Sim clock</p><p class="title is-5">%s</p>%s</div></div>`, d.Sim.Clock.UTC().Format("2 Jan 2006 15:04:05"), warp))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Customers</p><p class="title is-5">%d</p></div></div>`, d.Bank.Customers))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">NIM</p><p class="title is-5">%s</p></div></div>`, nimStr))
	s.WriteString(generalLedgerItem(d.Bank.GL))
	if d.Sim.DayLength > 0 {
		s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Day length</p><p class="title is-5">%s</p></div></div>`, d.Sim.DayLength))
	}
	if d.Sim.AccountDaysPer12h > 0 {
		s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Account days / 12h</p><p class="title is-5">%s</p></div></div>`, GroupThousands(strconv.FormatInt(d.Sim.AccountDaysPer12h, 10))))
	}
	if d.Sim.AddingCust {
		s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Adding</p><p class="title is-6">%d / %d</p><p class="heading">%.0f /s</p></div></div>`, d.Sim.AddingProgress, d.Sim.AddingTarget, d.Sim.CustomersPerSec))
	} else if d.Sim.LastCustomersPerSec > 0 {
		s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Last add</p><p class="title is-6">%.0f /s</p></div></div>`, d.Sim.LastCustomersPerSec))
	}
	s.WriteString(`</nav>`)

	// Balance boxes
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="box dash-box has-background-success-light"><p class="heading">Savings (deposits)</p><p class="title is-5">%s</p></div></div>`, FormatMoney(d.Bank.Savings)))
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="box dash-box has-background-info-light"><p class="heading">Lending (loans)</p><p class="title is-5">%s</p></div></div>`, FormatMoney(d.Bank.Lending)))
	reserveClass := "has-background-warning-light"
	if d.Bank.Cash < d.Bank.RequiredReserves {
		reserveClass = "has-background-danger-light"
	}
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="box dash-box %s"><p class="heading">BoE Cash Reserve</p><p class="title is-5">%s</p><p class="subtitle is-7 mb-0">Required: %s (%.0f%%) | BoE: %.2f%%</p></div></div>`,
		reserveClass, FormatMoney(d.Bank.Cash), FormatMoney(d.Bank.RequiredReserves), d.Bank.ReserveRatio*100, d.Bank.BoERate*100))
	s.WriteString(`</div>`)

	// Balance chart
	s.WriteString(`<h3 class="title is-6 has-text-grey mt-4 mb-2">Balance History</h3>`)
	s.WriteString(buildBalanceChartSVG(d.History.Balances))

	// Customer chart
	s.WriteString(`<h3 class="title is-6 has-text-grey mt-4 mb-2">Customer Count</h3>`)
	s.WriteString(buildCustomerChartSVG(d.History.Customers))

	return s.String()
}

// generalLedgerItem is the general ledger on the summary level (ADR-0005):
// the day it is posted through and whether that close reconciled. A
// break is shown, never hidden.
func generalLedgerItem(gl core.GeneralLedger) string {
	if gl.PostedThrough.IsZero() {
		return `<div class="level-item has-text-centered"><div><p class="heading">General ledger</p><p class="title is-6">not yet closed</p></div></div>`
	}
	state := `<p class="heading">reconciled</p>`
	if gl.Breaks == 1 {
		state = `<p class="heading"><span class="tag is-danger">1 break</span></p>`
	} else if gl.Breaks > 1 {
		state = fmt.Sprintf(`<p class="heading"><span class="tag is-danger">%d breaks</span></p>`, gl.Breaks)
	}
	return fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">General ledger</p><p class="title is-6">%s</p>%s</div></div>`, gl.PostedThrough.Format("2 Jan 2006"), state)
}

// buildNIMChart renders a single-line chart of NIM in basis points as an SVG fragment.
func buildNIMChart(history []NIMPoint) string {
	if len(history) == 0 {
		return ""
	}
	const (
		padL   = 80
		padR   = 20
		width  = 660
		chartW = width - padL - padR
		chartH = 140
		padT   = 10
	)

	minVal := history[0].NIM
	maxVal := history[0].NIM
	for _, np := range history {
		if np.NIM < minVal {
			minVal = np.NIM
		}
		if np.NIM > maxVal {
			maxVal = np.NIM
		}
	}
	valRange := maxVal - minVal
	if valRange < 10 {
		valRange = 20
		minVal -= 10
		maxVal += 10
	} else {
		minVal -= valRange * 0.1
		maxVal += valRange * 0.1
		valRange = maxVal - minVal
	}

	var s strings.Builder
	totalH := padT + chartH + 10
	s.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg" style="width:100%%;height:auto">`, width, totalH))

	// Background
	s.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="#fafafa" stroke="#dbdbdb" stroke-width="1"/>`, padL, padT, chartW, chartH))

	// Y-axis labels
	for i := 0; i <= 4; i++ {
		val := minVal + valRange*float64(i)/4.0
		y := float64(padT+chartH) - float64(chartH)*float64(i)/4.0
		s.WriteString(fmt.Sprintf(`<line x1="%d" y1="%.0f" x2="%d" y2="%.0f" stroke="#ededed" stroke-width="1"/>`, padL, y, padL+chartW, y))
		s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" text-anchor="end" font-size="9" fill="#7a7a7a">%.0f</text>`, padL-5, y+3, val))
	}

	// Line
	if len(history) == 1 {
		v := history[0].NIM
		x := float64(padL) + float64(chartW)/2
		y := float64(padT+chartH) - float64(chartH)*(v-minVal)/valRange
		s.WriteString(fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="3" fill="#f59e0b"/>`, x, y))
	} else {
		var pts strings.Builder
		for i, np := range history {
			v := np.NIM
			x := float64(padL) + float64(chartW)*float64(i)/float64(len(history)-1)
			y := float64(padT+chartH) - float64(chartH)*(v-minVal)/valRange
			y = math.Max(float64(padT), math.Min(float64(padT+chartH), y))
			if i == 0 {
				pts.WriteString(fmt.Sprintf("%.1f,%.1f", x, y))
			} else {
				pts.WriteString(fmt.Sprintf(" %.1f,%.1f", x, y))
			}
		}
		s.WriteString(fmt.Sprintf(`<polyline points="%s" fill="none" stroke="#f59e0b" stroke-width="2"/>`, pts.String()))
	}

	s.WriteString(`</svg>`)
	return s.String()
}

// buildBalanceChartSVG renders a standalone SVG balance chart (for HTML dashboard sections).
func buildBalanceChartSVG(history []BalancePoint) string {
	if len(history) == 0 {
		return ""
	}
	const (
		padL   = 80
		padR   = 20
		width  = 660
		chartW = width - padL - padR
		chartH = 160
		padT   = 10
	)

	// Geometry in float64 minor units (display math only — money stays integer).
	minVal := float64(history[0].Savings)
	maxVal := float64(history[0].Savings)
	for _, bp := range history {
		for _, v := range []float64{float64(bp.Savings), float64(bp.Lending)} {
			if v < minVal {
				minVal = v
			}
			if v > maxVal {
				maxVal = v
			}
		}
	}
	valRange := maxVal - minVal
	if valRange < 100_00 {
		valRange = 200_00
		minVal -= 100_00
		maxVal += 100_00
	} else {
		minVal -= valRange * 0.1
		maxVal += valRange * 0.1
		valRange = maxVal - minVal
	}
	if minVal < 0 {
		minVal = 0
		valRange = maxVal - minVal
	}

	var s strings.Builder
	totalH := padT + chartH + 25
	s.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg" style="width:100%%;height:auto">`, width, totalH))
	s.WriteString(`<style>text{font-family:Arial,Helvetica,sans-serif}</style>`)

	// Background
	s.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="#fafafa" stroke="#dbdbdb" stroke-width="1"/>`, padL, padT, chartW, chartH))

	// Y-axis labels
	for i := 0; i <= 4; i++ {
		val := minVal + valRange*float64(i)/4.0
		y := float64(padT+chartH) - float64(chartH)*float64(i)/4.0
		s.WriteString(fmt.Sprintf(`<line x1="%d" y1="%.0f" x2="%d" y2="%.0f" stroke="#ededed" stroke-width="1"/>`, padL, y, padL+chartW, y))
		s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" text-anchor="end" font-size="9" fill="#7a7a7a">%s</text>`, padL-5, y+3, FormatMoney(luca.Amount(math.Round(val)))))
	}

	// Lines
	type lineSpec struct {
		color string
		vals  func(BalancePoint) float64
	}
	lines := []lineSpec{
		{"#48c78e", func(bp BalancePoint) float64 { return float64(bp.Savings) }},
		{"#3e8ed0", func(bp BalancePoint) float64 { return float64(bp.Lending) }},
	}
	for _, line := range lines {
		if len(history) == 1 {
			v := line.vals(history[0])
			x := float64(padL) + float64(chartW)/2
			y := float64(padT+chartH) - float64(chartH)*(v-minVal)/valRange
			s.WriteString(fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="3" fill="%s"/>`, x, y, line.color))
			continue
		}
		var pts strings.Builder
		for i, bp := range history {
			v := line.vals(bp)
			x := float64(padL) + float64(chartW)*float64(i)/float64(len(history)-1)
			y := float64(padT+chartH) - float64(chartH)*(v-minVal)/valRange
			y = math.Max(float64(padT), math.Min(float64(padT+chartH), y))
			if i == 0 {
				pts.WriteString(fmt.Sprintf("%.1f,%.1f", x, y))
			} else {
				pts.WriteString(fmt.Sprintf(" %.1f,%.1f", x, y))
			}
		}
		s.WriteString(fmt.Sprintf(`<polyline points="%s" fill="none" stroke="%s" stroke-width="2"/>`, pts.String(), line.color))
	}

	// Legend
	legendY := float64(padT+chartH) + 18
	s.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%.0f" r="4" fill="#48c78e"/>`, padL, legendY-3))
	s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" font-size="10" fill="#363636">Savings</text>`, padL+8, legendY))
	s.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%.0f" r="4" fill="#3e8ed0"/>`, padL+70, legendY-3))
	s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" font-size="10" fill="#363636">Lending</text>`, padL+78, legendY))

	s.WriteString(`</svg>`)
	return s.String()
}

// buildCustomerChartSVG renders a standalone SVG customer count chart with
// gogal: whole-number counts, the first and last day labelled at the ends of
// the time axis, calendar dates between.
func buildCustomerChartSVG(history []CustomerPoint) string {
	if len(history) == 0 {
		return ""
	}
	times := make([]time.Time, len(history))
	values := make([]float64, len(history))
	for i, cp := range history {
		times[i], values[i] = cp.Date, float64(cp.Count)
	}
	svg, err := gogal.NewLineChart(
		gogal.WithSize(660, 180),
		gogal.WithMargins(10, 40, 24, 60),
		gogal.WithLegend(false),
		gogal.WithPoints(false),
		gogal.WithTooltips(false),
		gogal.WithAccessibility(false),
		gogal.WithTimeFormat(ChartDateFormat),
		gogal.WithYFormat("%.0f"),
		gogal.WithIntegerY(true),
		gogal.WithEndLabels(true),
	).AddTimeSeries("customers", times, values).RenderString()
	if err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">Chart error: %v</p>`, err)
	}
	return svg
}
