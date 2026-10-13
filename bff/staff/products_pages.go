package staff

import (
	"context"
	"fmt"
	"html"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/gobank/internal/view"
	"git.bytestone.uk/hum3/gogal"
)

// The products pages are a master-detail (lofigui's HTMX multipage
// pattern): the master, a family's product cards or the bank's own, lists
// parameters as rows; a row loads the parameter's own page into the
// detail beside it. While the simulation runs the master re-polls itself
// alone, so the detail and anything typed in it stand.

// BuildProductsHTML renders a family's product cards, each with its book
// from the core as the session's view shows it, its own parameters (the
// version scope: the market's and the bank's are on the bank tab) and the
// settings upcoming for each.
func BuildProductsHTML(q core.StaffQueries, family gbp.ProductFamily, v view.View, polling bool) string {
	ctx := context.Background()
	products, _ := q.Products(ctx)
	upcoming, _ := q.UpcomingSettings(ctx)

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
		s.WriteString(fmt.Sprintf(`<div class="column"><strong>Product total:</strong> %s</div>`, FormatMoney(v.Money(p.Balance, p.Family))))
		s.WriteString(fmt.Sprintf(`<div class="column"><strong>Adopted:</strong> %s <span class="has-text-grey is-size-7">gobank-products %s</span></div>`, p.Adopted.Format("2006-01-02"), p.ModuleVersion))
		s.WriteString(`</div>`)
		var rows []string
		for _, param := range p.Parameters {
			if param.Scope == string(gbp.ScopeVersion) {
				rows = append(rows, parameterRow(p.ID, param.Key, param.Kind, param.Value))
			}
		}
		s.WriteString(parameterTable(rows, "Published"))
		s.WriteString(renderUpcoming(p.ID, upcoming))
		s.WriteString(`</div>`)
	}
	return masterDetail(s.String(), "products/"+strings.ToLower(string(family)), polling)
}

// BuildBankHTML renders the bank tab: what is the bank's and not a
// product's. Each market the bank knows (ADR-0009), with the rate in
// force, the next decision day on its calendar and its parameter at the
// market scope; the bank's policy (ADR-0006, ADR-0007: its rates as
// versioned code) with the chart of the risk-free rate against the
// bank's savings and lending rates over the run and its parameters at
// the bank scope, the spreads and lags set here and the rates derived;
// then the bank's own reserve ratio.
func BuildBankHTML(q core.StaffQueries, polling bool) string {
	ctx := context.Background()
	pos, _ := q.Position(ctx)
	hist, _ := q.History(ctx)
	markets, _ := q.Markets(ctx)
	pol, polErr := q.Policy(ctx)

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">The Bank</h2>`)
	for _, m := range markets {
		next := "none on the calendar"
		if !m.NextDecision.IsZero() {
			next = m.NextDecision.UTC().Format("2 Jan 2006 15:04") + " UTC"
		}
		s.WriteString(fmt.Sprintf(`<div class="box">
  <h3 class="title is-5">%s (%s)</h3>
  <p class="subtitle is-6 has-text-grey">In force: %s &middot; Next decision day: %s &middot; <a href="%s">%s</a></p>`,
			html.EscapeString(m.Rate), html.EscapeString(m.Name), FormatRateBps(m.RateBps), next, html.EscapeString(m.Page), html.EscapeString(m.Name)))
		s.WriteString(parameterTable([]string{parameterRow(m.Currency, gbp.RiskFreeRateKey, string(gbp.KindBps), strconv.FormatInt(m.RateBps, 10))}, "In force"))
		s.WriteString(`</div>`)
	}
	if polErr == nil {
		s.WriteString(fmt.Sprintf(`<div class="box"><h3 class="title is-5">Policy <span class="tag is-info is-light">%s v%d</span></h3>
  <p class="subtitle is-6 has-text-grey">How the bank's own rates follow the market: a cut reaches savers at once and a rise after a lag, borrowers the other way about; the spreads and lags are set here, the rates derived</p>`,
			html.EscapeString(pol.Name), pol.Version))
		s.WriteString(buildRatesChart(hist))
		var rows []string
		for _, p := range pol.Parameters {
			rows = append(rows, parameterRow(core.BankOwner, p.Key, p.Kind, p.InForce))
		}
		s.WriteString(parameterTable(rows, "In force"))
		s.WriteString(`</div>`)
	}
	s.WriteString(`<div class="box"><h3 class="title is-5">The bank's own</h3>`)
	s.WriteString(fmt.Sprintf(`<p><strong>Reserve ratio:</strong> %.0f%% <span class="has-text-grey is-size-7">the fraction of deposits held as reserves at the Bank of England; a constant of the bank, not a parameter</span></p>`, pos.ReserveRatio*100))
	s.WriteString(`</div>`)
	return masterDetail(s.String(), "products/bank", polling)
}

// buildRatesChart draws the risk-free rate against the bank's savings and
// lending rates over the run, as percentages: the policy's effect, a lag
// visible as the bank's line moving after the market's.
func buildRatesChart(hist core.History) string {
	if len(hist.RiskFree) == 0 {
		return `<p class="has-text-grey">No rate history yet.</p>`
	}
	chart := gogal.NewLineChart(
		gogal.WithSize(660, 200),
		gogal.WithMargins(10, 40, 24, 60),
		gogal.WithLegend(true),
		gogal.WithPoints(false),
		gogal.WithTooltips(false),
		gogal.WithAccessibility(false),
		gogal.WithTimeFormat(ChartDateFormat),
		gogal.WithYFormat("%.2f%%"),
		gogal.WithEndLabels(true),
	)
	for _, series := range []struct {
		name   string
		points []core.RatePoint
	}{{"risk-free", hist.RiskFree}, {"savings", hist.SavingsRate}, {"lending", hist.LendingRate}} {
		if len(series.points) == 0 {
			continue
		}
		times := make([]time.Time, len(series.points))
		values := make([]float64, len(series.points))
		for i, p := range series.points {
			times[i], values[i] = p.Date, float64(p.Bps)/100
		}
		chart.AddTimeSeries(series.name, times, values)
	}
	svg, err := chart.RenderString()
	if err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">Chart error: %v</p>`, err)
	}
	return svg
}

// masterDetail lays the master out beside an empty detail. While the
// simulation runs the master re-polls its own page and keeps its own
// column of it, so the detail stands.
func masterDetail(master, page string, polling bool) string {
	poll := ""
	if polling {
		poll = fmt.Sprintf(` hx-get="%s" hx-trigger="every 2s" hx-swap="outerHTML" hx-select="#products-master"`, page)
	}
	return fmt.Sprintf(`<div class="columns"><div class="column is-7" id="products-master"%s>%s</div><div class="column is-5"><div id="parameter-detail"></div></div></div>`, poll, master)
}

// parameterRow is a parameter's row in a master: its key a link that
// loads the parameter's page into the detail, and the whole page for a
// browser without HTMX. The link selects the detail from its reply
// itself, so it does not inherit the polling master's selection.
func parameterRow(owner, key, kind, value string) string {
	page := parameterPath(owner, key)
	return fmt.Sprintf(`<tr><td><a href="%s" hx-get="%s" hx-target="#parameter-detail" hx-swap="outerHTML" hx-select="#parameter-detail" hx-push-url="true"><code>%s</code></a></td><td>%s</td><td>%s</td></tr>`,
		page, page, html.EscapeString(key), html.EscapeString(kind), html.EscapeString(value))
}

func parameterPath(owner, key string) string {
	return "products/" + url.PathEscape(owner) + "/parameters/" + url.PathEscape(key)
}

func parameterTable(rows []string, valueHeading string) string {
	if len(rows) == 0 {
		return ""
	}
	return `<table class="table is-narrow is-size-7"><thead><tr><th>Parameter</th><th>Kind</th><th>` + valueHeading + `</th></tr></thead><tbody>` + strings.Join(rows, "") + `</tbody></table>`
}

// renderUpcoming lists a product's settings decided and not yet in force.
func renderUpcoming(productID string, upcoming []core.Setting) string {
	var rows []string
	for _, u := range upcoming {
		if u.Scope == string(gbp.ScopeVersion) && u.Owner == productID {
			rows = append(rows, upcomingRow(u))
		}
	}
	if len(rows) == 0 {
		return ""
	}
	return `<h4 class="title is-6 mt-3 mb-1">Upcoming</h4>` + upcomingTable(rows)
}

func upcomingRow(u core.Setting) string {
	return fmt.Sprintf(`<tr><td><code>%s</code></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
		html.EscapeString(u.Key), html.EscapeString(u.Value), u.Effective.UTC().Format("2 Jan 2006 15:04"), u.Decided.UTC().Format("2 Jan 2006 15:04"), html.EscapeString(u.By))
}

func upcomingTable(rows []string) string {
	return `<table class="table is-narrow is-size-7"><thead><tr><th>Parameter</th><th>Value</th><th>Effective (UTC)</th><th>Decided</th><th>By</th></tr></thead><tbody>` +
		strings.Join(rows, "") + `</tbody></table>`
}

// BuildParameterHTML renders a parameter's own page, the detail: its
// declaration, its state (the value in force and the settings upcoming),
// and, for a parameter staff may set and a role that may, the set form.
// The state alone re-polls while the simulation runs; the form never
// does, so what is typed stands. A refused setting is shown with its
// reason. The instant is UTC; empty means the bank's day, in force now.
func BuildParameterHTML(d core.ParameterDetail, day time.Time, canSet, polling bool, refused string) string {
	page := parameterPath(d.Owner, d.Key)
	var s strings.Builder
	s.WriteString(`<div id="parameter-detail"><div class="box">`)
	s.WriteString(fmt.Sprintf(`<h3 class="title is-5"><code>%s</code> <span class="tag is-light">%s</span> <span class="tag is-light">%s</span></h3>`,
		html.EscapeString(d.Key), html.EscapeString(d.Scope), html.EscapeString(d.Kind)))
	s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">%s &middot; <a href="%s">%s</a></p>`, html.EscapeString(d.Description), ownerPage(d), html.EscapeString(d.Owner)))
	if d.Derived {
		s.WriteString(fmt.Sprintf(`<p><strong>Derived:</strong> %s <span class="has-text-grey is-size-7">by the rules' code; it has no settings of its own</span></p>`, html.EscapeString(d.Derivation)))
	}
	poll := ""
	if polling {
		poll = fmt.Sprintf(` hx-get="%s" hx-trigger="every 2s" hx-swap="outerHTML" hx-select="#parameter-state"`, page)
	}
	s.WriteString(fmt.Sprintf(`<div id="parameter-state"%s>`, poll))
	switch {
	case d.Derived:
	case d.Scope == string(gbp.ScopeAccount):
		s.WriteString(`<p><strong>In force:</strong> <span class="has-text-grey">one value per account, written at its opening</span></p>`)
	case d.InForce == "":
		s.WriteString(`<p><strong>In force:</strong> <span class="has-text-grey">none yet</span></p>`)
	default:
		s.WriteString(fmt.Sprintf(`<p><strong>In force:</strong> %s</p>`, html.EscapeString(d.InForce)))
	}
	s.WriteString(`<h4 class="title is-6 mt-3 mb-1">Upcoming</h4>`)
	if len(d.Upcoming) == 0 {
		s.WriteString(`<p class="has-text-grey">Nothing upcoming.</p>`)
	} else {
		rows := make([]string, len(d.Upcoming))
		for i, u := range d.Upcoming {
			rows[i] = upcomingRow(u)
		}
		s.WriteString(upcomingTable(rows))
	}
	s.WriteString(`</div>`)
	if refused != "" {
		s.WriteString(fmt.Sprintf(`<div class="notification is-danger is-light mt-3">Refused: %s</div>`, html.EscapeString(refused)))
	}
	if canSet && d.Settable {
		// Tomorrow is offered while the bank stands still; while the clock
		// runs a pre-filled instant goes stale (the form never re-renders),
		// so the instant is left empty, which the handler reads as now.
		tomorrow := day.AddDate(0, 0, 1).Format("2006-01-02T15:04")
		if polling {
			tomorrow = ""
		}
		s.WriteString(fmt.Sprintf(`<form hx-post="%s" hx-target="#parameter-detail" hx-swap="outerHTML" action="%s" method="post" class="mt-3">
  <div class="field is-grouped is-grouped-multiline">
    <div class="control"><input class="input is-small" type="text" name="value" placeholder="value (%s)" required></div>
    <div class="control"><input class="input is-small" type="datetime-local" name="effective" value="%s" min="%s"></div>
    <div class="control"><button class="button is-small is-primary" type="submit">Set parameter</button></div>
  </div>
  <p class="help">Decided now, effective at the instant (UTC), or now when left empty; the product's accounts are told when the clock reaches it</p>
</form>`, page, page, html.EscapeString(d.Kind), tomorrow, day.Format("2006-01-02T15:04")))
	}
	s.WriteString(`</div></div>`)
	return s.String()
}

// ownerPage is the master a parameter's owner is listed on: a product's
// family page, or the bank tab for a market's or the bank's own.
func ownerPage(d core.ParameterDetail) string {
	if d.Family != "" {
		return "products/" + strings.ToLower(d.Family)
	}
	return "products/bank"
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
