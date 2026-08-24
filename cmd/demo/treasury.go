package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
)

// TreasurySnapshot holds data for treasury pages, grabbed under one lock.
type TreasurySnapshot struct {
	Day              time.Time
	DayCount         int
	Savings          float64
	Lending          float64
	Cash             float64
	RequiredReserves float64
	ExcessCash       float64
	ReserveRatio     float64
	BoeRate          float64
	BoeInterest      float64
	BalanceHistory   []BalancePoint
}

// TreasuryData returns a snapshot for treasury pages.
func (ds *DemoState) TreasuryData() TreasurySnapshot {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	var savings, lending float64
	for _, c := range ds.customers {
		for _, a := range c.Accounts {
			if a.Family == gbp.FamilySavings {
				savings += a.Balance
			} else {
				lending += a.Balance
			}
		}
	}

	cash := savings - lending
	required := savings * ds.settings.CapitalReserveRatio
	excess := cash - required

	balHist := make([]BalancePoint, len(ds.balanceHistory))
	copy(balHist, ds.balanceHistory)

	return TreasurySnapshot{
		Day:              ds.currentDay,
		DayCount:         ds.dayCount,
		Savings:          savings,
		Lending:          lending,
		Cash:             cash,
		RequiredReserves: required,
		ExcessCash:       excess,
		ReserveRatio:     ds.settings.CapitalReserveRatio,
		BoeRate:          ds.settings.BoEBaseRate,
		BoeInterest:      ds.boeInterestAccum,
		BalanceHistory:   balHist,
	}
}

// GiltYield represents a row from the gilt_yields table.
type GiltYield struct {
	Tenor string
	Rate  float64
}

// GiltHolding represents a row from the gilt_holdings table.
type GiltHolding struct {
	ID           int
	Tenor        string
	FaceValue    float64
	PurchaseDate time.Time
	Yield        float64
}

// getGiltYields reads current gilt yields from the DB.
func (ds *DemoState) getGiltYields() []GiltYield {
	db := ds.DB()
	if db == nil {
		return nil
	}
	rows, err := db.Query(`SELECT tenor, rate FROM gilt_yields ORDER BY tenor`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var yields []GiltYield
	for rows.Next() {
		var y GiltYield
		if err := rows.Scan(&y.Tenor, &y.Rate); err == nil {
			yields = append(yields, y)
		}
	}
	return yields
}

// getGiltHoldings reads gilt holdings from the DB.
func (ds *DemoState) getGiltHoldings() []GiltHolding {
	db := ds.DB()
	if db == nil {
		return nil
	}
	rows, err := db.Query(`SELECT id, tenor, face_value, purchase_date, yield FROM gilt_holdings ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var holdings []GiltHolding
	for rows.Next() {
		var h GiltHolding
		if err := rows.Scan(&h.ID, &h.Tenor, &h.FaceValue, &h.PurchaseDate, &h.Yield); err == nil {
			holdings = append(holdings, h)
		}
	}
	return holdings
}

// BuyGilt purchases a gilt and records it in the DB.
func (ds *DemoState) BuyGilt(tenor string, faceValue float64) error {
	db := ds.DB()
	if db == nil {
		return fmt.Errorf("database not available")
	}

	// Look up current yield
	var rate float64
	err := db.QueryRow(`SELECT rate FROM gilt_yields WHERE tenor = $1`, tenor).Scan(&rate)
	if err != nil {
		return fmt.Errorf("unknown tenor %q", tenor)
	}

	ds.mu.Lock()
	purchaseDate := ds.currentDay
	ds.mu.Unlock()

	_, err = db.Exec(`INSERT INTO gilt_holdings (tenor, face_value, purchase_date, yield) VALUES ($1, $2, $3, $4)`,
		tenor, faceValue, purchaseDate, rate)
	return err
}

// --- Cash Position Page ---

func (ds *DemoState) BuildCashPositionHTML() string {
	t := ds.TreasuryData()

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Cash Position</h2>`)

	// Level boxes
	s.WriteString(`<nav class="level mb-4">`)
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Total Deposits</p><p class="title is-5">%s</p></div></div>`, fmtMoney(t.Savings)))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Total Loans</p><p class="title is-5">%s</p></div></div>`, fmtMoney(t.Lending)))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Cash at BoE</p><p class="title is-5">%s</p></div></div>`, fmtMoney(t.Cash)))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Required Reserves</p><p class="title is-5">%s</p></div></div>`, fmtMoney(t.RequiredReserves)))
	excessClass := "has-text-success"
	if t.ExcessCash < 0 {
		excessClass = "has-text-danger"
	}
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Excess Cash</p><p class="title is-5 %s">%s</p></div></div>`, excessClass, fmtMoney(t.ExcessCash)))
	s.WriteString(`</nav>`)

	// Detail table
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Cash Detail</h3>`)
	s.WriteString(`<table class="table is-fullwidth is-striped">`)
	s.WriteString(`<tbody>`)
	s.WriteString(fmt.Sprintf(`<tr><td>BoE Base Rate</td><td class="has-text-right"><strong>%.2f%%</strong></td></tr>`, t.BoeRate*100))
	s.WriteString(fmt.Sprintf(`<tr><td>BoE Interest Earned (cumulative)</td><td class="has-text-right"><strong>%s</strong></td></tr>`, fmtMoney(t.BoeInterest)))
	s.WriteString(fmt.Sprintf(`<tr><td>Reserve Ratio</td><td class="has-text-right"><strong>%.0f%%</strong></td></tr>`, t.ReserveRatio*100))
	s.WriteString(fmt.Sprintf(`<tr><td>Simulation Day</td><td class="has-text-right"><strong>%d &mdash; %s</strong></td></tr>`, t.DayCount, t.Day.Format("2 Jan 2006")))
	s.WriteString(`</tbody></table>`)
	s.WriteString(`</div>`)

	// Balance history chart
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Balance History</h3>`)
	s.WriteString(buildBalanceChartSVG(t.BalanceHistory))
	s.WriteString(`</div>`)

	return s.String()
}

// --- Capital Requirements Page ---

func (ds *DemoState) BuildCapitalHTML() string {
	t := ds.TreasuryData()

	actualRatio := 0.0
	if t.Savings > 0 {
		actualRatio = t.Cash / t.Savings
	}
	compliant := actualRatio >= t.ReserveRatio

	maxLoans := t.Savings * (1 - t.ReserveRatio)
	headroom := maxLoans - t.Lending
	utilisationPct := 0.0
	if maxLoans > 0 {
		utilisationPct = t.Lending / maxLoans * 100
	}

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Capital Requirements</h2>`)

	// Compliance tag
	if compliant {
		s.WriteString(`<span class="tag is-success is-medium mb-4">Compliant</span>`)
	} else {
		s.WriteString(`<span class="tag is-danger is-medium mb-4">Non-Compliant</span>`)
	}

	// Key metrics
	s.WriteString(`<nav class="level mb-4">`)
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Reserve Ratio (actual)</p><p class="title is-5">%.1f%%</p></div></div>`, actualRatio*100))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Reserve Ratio (required)</p><p class="title is-5">%.0f%%</p></div></div>`, t.ReserveRatio*100))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Lending Headroom</p><p class="title is-5">%s</p></div></div>`, fmtMoney(headroom)))
	s.WriteString(`</nav>`)

	// Utilisation bar SVG
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Lending Capacity Utilisation</h3>`)
	s.WriteString(buildUtilisationBar(utilisationPct))
	s.WriteString(`</div>`)

	// Detail breakdown
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Breakdown</h3>`)
	s.WriteString(`<table class="table is-fullwidth is-striped">`)
	s.WriteString(`<tbody>`)
	s.WriteString(fmt.Sprintf(`<tr><td>Total Deposits</td><td class="has-text-right">%s</td></tr>`, fmtMoney(t.Savings)))
	s.WriteString(fmt.Sprintf(`<tr><td>Total Loans</td><td class="has-text-right">%s</td></tr>`, fmtMoney(t.Lending)))
	s.WriteString(fmt.Sprintf(`<tr><td>Cash at BoE</td><td class="has-text-right">%s</td></tr>`, fmtMoney(t.Cash)))
	s.WriteString(fmt.Sprintf(`<tr><td>Required Reserves (%.0f%% of deposits)</td><td class="has-text-right">%s</td></tr>`, t.ReserveRatio*100, fmtMoney(t.RequiredReserves)))
	s.WriteString(fmt.Sprintf(`<tr><td>Max Lending Capacity</td><td class="has-text-right">%s</td></tr>`, fmtMoney(maxLoans)))
	s.WriteString(fmt.Sprintf(`<tr><td>Utilisation</td><td class="has-text-right">%.1f%%</td></tr>`, utilisationPct))
	s.WriteString(`</tbody></table>`)
	s.WriteString(`</div>`)

	return s.String()
}

// buildUtilisationBar renders an SVG horizontal bar showing utilisation percentage.
// Green 0-80%, amber 80-95%, red 95-100%.
func buildUtilisationBar(pct float64) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	barW := 600.0
	fillW := barW * pct / 100

	color := "#48c78e" // green
	if pct >= 95 {
		color = "#f14668" // red
	} else if pct >= 80 {
		color = "#ffe08a" // amber
	}

	var s strings.Builder
	s.WriteString(`<svg viewBox="0 0 660 40" xmlns="http://www.w3.org/2000/svg" style="width:100%;height:auto">`)
	s.WriteString(`<rect x="30" y="5" width="600" height="30" rx="4" fill="#f0f0f0" stroke="#dbdbdb" stroke-width="1"/>`)
	if fillW > 0 {
		s.WriteString(fmt.Sprintf(`<rect x="30" y="5" width="%.0f" height="30" rx="4" fill="%s" opacity="0.85"/>`, fillW, color))
	}
	s.WriteString(fmt.Sprintf(`<text x="330" y="25" text-anchor="middle" font-size="13" font-weight="bold" fill="#363636">%.1f%%</text>`, pct))
	s.WriteString(`</svg>`)
	return s.String()
}

// --- Gilt Purchases Page ---

func (ds *DemoState) BuildGiltsHTML() string {
	yields := ds.getGiltYields()
	holdings := ds.getGiltHoldings()

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Gilt Purchases</h2>`)

	// Yield curve chart
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">UK Gilt Yield Curve</h3>`)
	s.WriteString(buildYieldCurveSVG(yields))
	s.WriteString(`</div>`)

	// Current yields table
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Current Yields</h3>`)
	s.WriteString(`<table class="table is-fullwidth is-striped">`)
	s.WriteString(`<thead><tr><th>Tenor</th><th class="has-text-right">Yield</th></tr></thead>`)
	s.WriteString(`<tbody>`)
	for _, y := range yields {
		s.WriteString(fmt.Sprintf(`<tr><td>%s</td><td class="has-text-right">%.2f%%</td></tr>`, y.Tenor, y.Rate*100))
	}
	s.WriteString(`</tbody></table>`)
	s.WriteString(`</div>`)

	// Buy form
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Buy Gilt</h3>`)
	s.WriteString(`<form action="/treasury/gilts/buy" method="post">`)
	s.WriteString(`<div class="field is-horizontal"><div class="field-label is-normal"><label class="label">Tenor</label></div>`)
	s.WriteString(`<div class="field-body"><div class="field"><div class="control"><div class="select">`)
	s.WriteString(`<select name="tenor">`)
	for _, y := range yields {
		s.WriteString(fmt.Sprintf(`<option value="%s">%s (%.2f%%)</option>`, y.Tenor, y.Tenor, y.Rate*100))
	}
	s.WriteString(`</select></div></div></div></div></div>`)
	s.WriteString(`<div class="field is-horizontal"><div class="field-label is-normal"><label class="label">Face Value</label></div>`)
	s.WriteString(`<div class="field-body"><div class="field"><div class="control">`)
	s.WriteString(`<input class="input" type="number" name="face_value" value="100000" min="1000" step="1000">`)
	s.WriteString(`</div></div></div></div>`)
	s.WriteString(`<div class="field is-horizontal"><div class="field-label"></div><div class="field-body"><div class="field"><div class="control">`)
	s.WriteString(`<button class="button is-primary" type="submit">Purchase</button>`)
	s.WriteString(`</div></div></div></div>`)
	s.WriteString(`</form></div>`)

	// Holdings table
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Holdings</h3>`)
	if len(holdings) == 0 {
		s.WriteString(`<p class="has-text-grey">No gilt holdings yet.</p>`)
	} else {
		totalFace := 0.0
		weightedTenor := 0.0
		for _, h := range holdings {
			totalFace += h.FaceValue
			weightedTenor += h.FaceValue * tenorYears(h.Tenor)
		}
		avgTenor := 0.0
		if totalFace > 0 {
			avgTenor = weightedTenor / totalFace
		}
		s.WriteString(fmt.Sprintf(`<p class="mb-2"><strong>Total Face Value:</strong> %s | <strong>Avg Tenor:</strong> %.1f years</p>`, fmtMoney(totalFace), avgTenor))
		s.WriteString(`<table class="table is-fullwidth is-striped">`)
		s.WriteString(`<thead><tr><th>#</th><th>Tenor</th><th class="has-text-right">Face Value</th><th>Purchase Date</th><th class="has-text-right">Yield</th></tr></thead>`)
		s.WriteString(`<tbody>`)
		for _, h := range holdings {
			s.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td class="has-text-right">%s</td><td>%s</td><td class="has-text-right">%.2f%%</td></tr>`,
				h.ID, h.Tenor, fmtMoney(h.FaceValue), h.PurchaseDate.Format("2 Jan 2006"), h.Yield*100))
		}
		s.WriteString(`</tbody></table>`)
	}
	s.WriteString(`</div>`)

	return s.String()
}

// tenorYears converts tenor string like "1Y", "10Y" to numeric years.
func tenorYears(tenor string) float64 {
	var n float64
	fmt.Sscanf(tenor, "%fY", &n)
	return n
}

// buildYieldCurveSVG renders the gilt yield curve as an SVG chart.
func buildYieldCurveSVG(yields []GiltYield) string {
	if len(yields) == 0 {
		return `<p class="has-text-grey">No yield data.</p>`
	}

	const (
		padL   = 60
		padR   = 30
		padT   = 20
		padB   = 40
		width  = 660
		chartW = width - padL - padR
		chartH = 180
	)
	totalH := padT + chartH + padB

	// Find Y range
	minRate := yields[0].Rate
	maxRate := yields[0].Rate
	for _, y := range yields {
		if y.Rate < minRate {
			minRate = y.Rate
		}
		if y.Rate > maxRate {
			maxRate = y.Rate
		}
	}
	rateRange := maxRate - minRate
	if rateRange < 0.005 {
		rateRange = 0.01
		minRate -= 0.005
		maxRate += 0.005
	} else {
		minRate -= rateRange * 0.15
		maxRate += rateRange * 0.15
		rateRange = maxRate - minRate
	}

	var s strings.Builder
	s.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg" style="width:100%%;height:auto">`, width, totalH))
	s.WriteString(`<style>text{font-family:Arial,Helvetica,sans-serif}</style>`)

	// Background
	s.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="#fafafa" stroke="#dbdbdb" stroke-width="1"/>`, padL, padT, chartW, chartH))

	// Y-axis labels
	for i := 0; i <= 4; i++ {
		val := minRate + rateRange*float64(i)/4.0
		y := float64(padT+chartH) - float64(chartH)*float64(i)/4.0
		s.WriteString(fmt.Sprintf(`<line x1="%d" y1="%.0f" x2="%d" y2="%.0f" stroke="#ededed" stroke-width="1"/>`, padL, y, padL+chartW, y))
		s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" text-anchor="end" font-size="10" fill="#7a7a7a">%.2f%%</text>`, padL-5, y+4, val*100))
	}

	// Plot points and line
	var pts strings.Builder
	for i, y := range yields {
		x := float64(padL) + float64(chartW)*float64(i)/float64(len(yields)-1)
		cy := float64(padT+chartH) - float64(chartH)*(y.Rate-minRate)/rateRange
		cy = math.Max(float64(padT), math.Min(float64(padT+chartH), cy))
		if i == 0 {
			pts.WriteString(fmt.Sprintf("%.1f,%.1f", x, cy))
		} else {
			pts.WriteString(fmt.Sprintf(" %.1f,%.1f", x, cy))
		}
		// Point dot
		s.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="4" fill="#3273dc"/>`, x, cy))
		// X label
		labelY := float64(padT+chartH) + 18
		s.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.0f" text-anchor="middle" font-size="11" fill="#363636">%s</text>`, x, labelY, y.Tenor))
	}
	s.WriteString(fmt.Sprintf(`<polyline points="%s" fill="none" stroke="#3273dc" stroke-width="2"/>`, pts.String()))

	s.WriteString(`</svg>`)
	return s.String()
}
