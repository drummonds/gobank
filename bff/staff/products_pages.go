package staff

import (
	"context"
	"fmt"
	"html"
	"math"
	"strings"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
)

// BuildProductsHTML renders product cards for a given family, with each
// product's book from the core, the settings upcoming for each, and, for
// a role that may set them, a form to set one of a product's own
// parameters effective at an instant (ADR-0010). For savings, appends
// the pound's market: the rate in force, its next decision day and the
// rate's history.
func BuildProductsHTML(q core.StaffQueries, family gbp.ProductFamily, canSet bool) string {
	ctx := context.Background()
	products, _ := q.Products(ctx)
	pos, _ := q.Position(ctx)
	hist, _ := q.History(ctx)
	upcoming, _ := q.UpcomingSettings(ctx)
	rateHistory, riskFree := hist.RiskFree, pos.RiskFreeBps

	var s strings.Builder
	s.WriteString(fmt.Sprintf(`<h2 class="title is-4">%s Products</h2>`, family))

	for _, p := range products {
		if p.Family != string(family) {
			continue
		}
		sale := `<span class="tag is-success is-light">on sale</span>`
		if !p.OnSale {
			sale = `<span class="tag is-light">withdrawn</span>`
		}
		s.WriteString(`<div class="box">`)
		rules := fmt.Sprintf(`<span class="tag is-info is-light">%s v%d</span>`, p.Rules, p.RulesVersion)
		if !p.Carried {
			rules += ` <span class="tag is-warning is-light">rules not in this build</span>`
		}
		s.WriteString(fmt.Sprintf(`<h3 class="title is-5">%s %s %s</h3>`, p.Name, rules, sale))
		s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">%s</p>`, p.Description))
		s.WriteString(`<div class="columns">`)
		s.WriteString(fmt.Sprintf(`<div class="column"><strong>Rate:</strong> %s</div>`, FormatRateBps(p.RateBps)))
		s.WriteString(fmt.Sprintf(`<div class="column"><strong>Accounts:</strong> %d</div>`, p.Accounts))
		s.WriteString(fmt.Sprintf(`<div class="column"><strong>Product total:</strong> %s</div>`, FormatMoney(p.Balance)))
		s.WriteString(fmt.Sprintf(`<div class="column"><strong>Adopted:</strong> %s <span class="has-text-grey is-size-7">gobank-products %s</span></div>`, p.Adopted.Format("2006-01-02"), p.ModuleVersion))
		s.WriteString(`</div>`)
		s.WriteString(`<table class="table is-narrow is-size-7"><thead><tr><th>Parameter</th><th>Scope</th><th>Kind</th><th>Published</th></tr></thead><tbody>`)
		for _, param := range p.Parameters {
			s.WriteString(fmt.Sprintf(`<tr><td><code>%s</code></td><td>%s</td><td>%s</td><td>%s</td></tr>`, param.Key, param.Scope, param.Kind, param.Value))
		}
		s.WriteString(`</tbody></table>`)
		s.WriteString(renderUpcoming(p.ID, upcoming))
		if canSet && p.Carried {
			s.WriteString(renderSetParameter(p, family, pos.Day))
		}
		s.WriteString(`</div>`)
	}

	// The pound's market (ADR-0009), for the savings family: the rate in
	// force and the next decision day on its calendar, at which it
	// publishes a change or nothing (ADR-0010).
	if family == gbp.FamilySavings {
		markets, _ := q.Markets(ctx)
		for _, m := range markets {
			if m.Currency != "GBP" {
				continue
			}
			next := "none on the calendar"
			if !m.NextDecision.IsZero() {
				next = m.NextDecision.UTC().Format("2 Jan 2006 15:04") + " UTC"
			}
			s.WriteString(fmt.Sprintf(`<div class="box mt-4">
  <h3 class="title is-5">%s (%s)</h3>
  <p class="subtitle is-6 has-text-grey">In force: %s &middot; Next decision day: %s &middot; <a href="%s">%s</a></p>`,
				html.EscapeString(m.Rate), html.EscapeString(m.Name), FormatRateBps(riskFree), next, html.EscapeString(m.Page), html.EscapeString(m.Name)))
			s.WriteString(buildBoERateGraph(rateHistory))
			s.WriteString(`</div>`)
		}
	}

	return s.String()
}

// renderUpcoming lists a product's settings decided and not yet in force.
func renderUpcoming(productID string, upcoming []core.Setting) string {
	var rows []string
	for _, u := range upcoming {
		if u.Scope == string(gbp.ScopeVersion) && u.Owner == productID {
			rows = append(rows, fmt.Sprintf(`<tr><td><code>%s</code></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
				html.EscapeString(u.Key), html.EscapeString(u.Value), u.Effective.UTC().Format("2 Jan 2006 15:04"), u.Decided.UTC().Format("2 Jan 2006 15:04"), html.EscapeString(u.By)))
		}
	}
	if len(rows) == 0 {
		return ""
	}
	return `<h4 class="title is-6 mt-3 mb-1">Upcoming</h4><table class="table is-narrow is-size-7"><thead><tr><th>Parameter</th><th>Value</th><th>Effective (UTC)</th><th>Decided</th><th>By</th></tr></thead><tbody>` +
		strings.Join(rows, "") + `</tbody></table>`
}

// renderSetParameter is the form to set one of a product's own
// parameters: those its rules declare at the product's scope and do not
// derive. The instant is UTC; empty means the bank's day, in force now.
func renderSetParameter(p core.Product, family gbp.ProductFamily, day time.Time) string {
	var options []string
	for _, param := range p.Parameters {
		if param.Scope == string(gbp.ScopeVersion) && !param.Derived {
			options = append(options, fmt.Sprintf(`<option value="%s">%s (%s)</option>`, html.EscapeString(param.Key), html.EscapeString(param.Key), html.EscapeString(param.Kind)))
		}
	}
	if len(options) == 0 {
		return ""
	}
	return fmt.Sprintf(`<form action="products/set" method="post" class="mt-3">
  <input type="hidden" name="product" value="%s"><input type="hidden" name="family" value="%s">
  <div class="field is-grouped is-grouped-multiline">
    <div class="control"><div class="select is-small"><select name="key">%s</select></div></div>
    <div class="control"><input class="input is-small" type="text" name="value" placeholder="value" required></div>
    <div class="control"><input class="input is-small" type="datetime-local" name="effective" value="%s" min="%s"></div>
    <div class="control"><button class="button is-small is-primary" type="submit">Set parameter</button></div>
  </div>
  <p class="help">Decided now, effective at the instant (UTC); the product's accounts are told when the clock reaches it</p>
</form>`, html.EscapeString(p.ID), family, strings.Join(options, ""), day.AddDate(0, 0, 1).Format("2006-01-02T15:04"), day.Format("2006-01-02T15:04"))
}

// buildBoERateGraph renders an SVG line chart of the risk-free rate (basis points, drawn as a percentage) over simulated time.
func buildBoERateGraph(history []RatePoint) string {
	if len(history) == 0 {
		return `<p class="has-text-grey">No rate history yet.</p>`
	}

	const (
		svgW   = 700
		svgH   = 200
		padL   = 60
		padR   = 20
		padT   = 20
		padB   = 40
		chartW = svgW - padL - padR
		chartH = svgH - padT - padB
	)

	// Find min/max rate for Y-axis scaling
	minRate := float64(history[0].Bps) / 10000
	maxRate := float64(history[0].Bps) / 10000
	for _, rp := range history {
		if float64(rp.Bps)/10000 < minRate {
			minRate = float64(rp.Bps) / 10000
		}
		if float64(rp.Bps)/10000 > maxRate {
			maxRate = float64(rp.Bps) / 10000
		}
	}
	// Add padding to range
	rateRange := maxRate - minRate
	if rateRange < 0.01 {
		rateRange = 0.02
		minRate -= 0.01
		maxRate += 0.01
	} else {
		minRate -= rateRange * 0.1
		maxRate += rateRange * 0.1
		rateRange = maxRate - minRate
	}
	if minRate < 0 {
		minRate = 0
		rateRange = maxRate - minRate
	}

	var s strings.Builder
	s.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg" style="max-width:%dpx;width:100%%;height:auto">`, svgW, svgH, svgW))
	s.WriteString(`<style>text{font-family:Arial,Helvetica,sans-serif}</style>`)

	// Background
	s.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="#fafafa" stroke="#dbdbdb" stroke-width="1"/>`, padL, padT, chartW, chartH))

	// Y-axis grid lines and labels (4 lines)
	for i := 0; i <= 4; i++ {
		rate := minRate + rateRange*float64(i)/4.0
		y := float64(padT+chartH) - float64(chartH)*float64(i)/4.0
		s.WriteString(fmt.Sprintf(`<line x1="%d" y1="%.0f" x2="%d" y2="%.0f" stroke="#ededed" stroke-width="1"/>`, padL, y, padL+chartW, y))
		s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" text-anchor="end" font-size="10" fill="#7a7a7a">%.2f%%</text>`, padL-5, y+4, rate*100))
	}

	// Date range annotation
	startDate := history[0].Date
	endDate := history[len(history)-1].Date
	dateRange := fmt.Sprintf("%s — %s", startDate.Format("2 Jan 2006"), endDate.Format("2 Jan 2006"))
	s.WriteString(fmt.Sprintf(`<text x="%d" y="%d" text-anchor="end" font-size="10" fill="#7a7a7a">%s</text>`, padL+chartW, padT-5, dateRange))

	// X-axis date labels
	if len(history) > 1 {
		numLabels := min(len(history), 5)
		// Use "Jan 06" format (month + 2-digit year) for compactness
		dateFmt := "Jan 06"
		// If span is less than a year, include day
		if endDate.Sub(startDate).Hours() < 365*24 {
			dateFmt = "2 Jan 06"
		}
		for i := range numLabels {
			idx := i * (len(history) - 1) / (numLabels - 1)
			x := float64(padL) + float64(chartW)*float64(idx)/float64(len(history)-1)
			s.WriteString(fmt.Sprintf(`<text x="%.0f" y="%d" text-anchor="middle" font-size="9" fill="#7a7a7a">%s</text>`, x, svgH-5, history[idx].Date.Format(dateFmt)))
		}
	}

	// Rate line
	if len(history) > 1 {
		var points strings.Builder
		for i, rp := range history {
			x := float64(padL) + float64(chartW)*float64(i)/float64(len(history)-1)
			y := float64(padT+chartH) - float64(chartH)*(float64(rp.Bps)/10000-minRate)/rateRange
			y = math.Max(float64(padT), math.Min(float64(padT+chartH), y))
			if i == 0 {
				points.WriteString(fmt.Sprintf("%.1f,%.1f", x, y))
			} else {
				points.WriteString(fmt.Sprintf(" %.1f,%.1f", x, y))
			}
		}
		s.WriteString(fmt.Sprintf(`<polyline points="%s" fill="none" stroke="#48c78e" stroke-width="2"/>`, points.String()))
	} else {
		// Single point — draw a dot
		x := float64(padL) + float64(chartW)/2
		y := float64(padT) + float64(chartH)/2
		s.WriteString(fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="4" fill="#48c78e"/>`, x, y))
	}

	s.WriteString(`</svg>`)
	return s.String()
}
