package main

import (
	"fmt"
	"math"
	"strings"
)

type ProductFamily string

const (
	FamilySavings ProductFamily = "Savings"
	FamilyLending ProductFamily = "Lending"
)

type Product struct {
	ID          string
	Name        string
	Family      ProductFamily
	Rate        float64 // annual rate as decimal
	Terms       string
	Description string
}

func AllProducts() []Product {
	return []Product{
		{ID: "easy-access", Name: "Easy Access", Family: FamilySavings, Rate: 0.015, Terms: "No notice", Description: "Instant access savings with competitive rate"},
		{ID: "fixed-term", Name: "Fixed Term", Family: FamilySavings, Rate: 0.040, Terms: "2 year fixed", Description: "Higher rate for locking funds for 2 years"},
		{ID: "isa", Name: "ISA", Family: FamilySavings, Rate: 0.035, Terms: "Annual allowance", Description: "Tax-free savings up to annual ISA allowance"},
		{ID: "personal-loan", Name: "Personal Loan", Family: FamilyLending, Rate: 0.069, Terms: "1-5 years", Description: "Unsecured personal loan for any purpose"},
		{ID: "mortgage", Name: "Mortgage", Family: FamilyLending, Rate: 0.045, Terms: "25 year", Description: "Residential mortgage with fixed rate period"},
		{ID: "overdraft", Name: "Overdraft", Family: FamilyLending, Rate: 0.159, Terms: "Revolving", Description: "Arranged overdraft facility on current account"},
	}
}

// BuildProductsHTML renders product cards for a given family, with account counts from state.
// For savings family, appends BoE base rate history graph.
func (ds *DemoState) BuildProductsHTML(family ProductFamily) string {
	ds.mu.Lock()
	products := ds.products
	customers := make([]CustomerRecord, len(ds.customers))
	copy(customers, ds.customers)
	boeHistory := make([]RatePoint, len(ds.boeHistory))
	copy(boeHistory, ds.boeHistory)
	boeRate := ds.settings.BoEBaseRate
	ds.mu.Unlock()

	var s strings.Builder
	s.WriteString(fmt.Sprintf(`<h2 class="title is-4">%s Products</h2>`, family))

	for _, p := range products {
		if p.Family != family {
			continue
		}
		// Count accounts and total balance
		count := 0
		totalBal := 0.0
		for _, c := range customers {
			for _, a := range c.Accounts {
				if a.ProductID == p.ID {
					count++
					totalBal += a.Balance
				}
			}
		}

		s.WriteString(`<div class="box">`)
		s.WriteString(fmt.Sprintf(`<h3 class="title is-5">%s</h3>`, p.Name))
		s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">%s</p>`, p.Description))
		s.WriteString(`<div class="columns">`)
		s.WriteString(fmt.Sprintf(`<div class="column"><strong>Rate:</strong> %.1f%%</div>`, p.Rate*100))
		s.WriteString(fmt.Sprintf(`<div class="column"><strong>Terms:</strong> %s</div>`, p.Terms))
		s.WriteString(fmt.Sprintf(`<div class="column"><strong>Accounts:</strong> %d</div>`, count))
		s.WriteString(fmt.Sprintf(`<div class="column"><strong>Total:</strong> %s</div>`, fmtMoney(totalBal)))
		s.WriteString(`</div></div>`)
	}

	// Append BoE rate graph for savings family
	if family == FamilySavings {
		s.WriteString(fmt.Sprintf(`<div class="box mt-4">
  <h3 class="title is-5">BoE Base Rate History</h3>
  <p class="subtitle is-6 has-text-grey">Current: %.2f%%</p>`, boeRate*100))
		s.WriteString(buildBoERateGraph(boeHistory))
		s.WriteString(`</div>`)
	}

	return s.String()
}

// buildBoERateGraph renders an SVG line chart of BoE base rate over simulated time.
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
	minRate := history[0].Rate
	maxRate := history[0].Rate
	for _, rp := range history {
		if rp.Rate < minRate {
			minRate = rp.Rate
		}
		if rp.Rate > maxRate {
			maxRate = rp.Rate
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
		numLabels := 5
		if len(history) < numLabels {
			numLabels = len(history)
		}
		// Use "Jan 06" format (month + 2-digit year) for compactness
		dateFmt := "Jan 06"
		// If span is less than a year, include day
		if endDate.Sub(startDate).Hours() < 365*24 {
			dateFmt = "2 Jan 06"
		}
		for i := 0; i < numLabels; i++ {
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
			y := float64(padT+chartH) - float64(chartH)*(rp.Rate-minRate)/rateRange
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
