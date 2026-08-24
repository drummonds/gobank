package main

import (
	"fmt"
	"strings"

	gbp "git.bytestone.uk/hum3/gobank-products"
)

// BuildChartsHTML renders historical charts: NIM, balances, customer count, BoE rate.
func (ds *DemoState) BuildChartsHTML() string {
	ds.mu.Lock()
	nimHist := make([]NIMPoint, len(ds.nimHistory))
	copy(nimHist, ds.nimHistory)
	balHist := make([]BalancePoint, len(ds.balanceHistory))
	copy(balHist, ds.balanceHistory)
	custHist := make([]CustomerPoint, len(ds.customerHistory))
	copy(custHist, ds.customerHistory)
	boeHist := make([]RatePoint, len(ds.boeHistory))
	copy(boeHist, ds.boeHistory)
	dayCount := ds.dayCount
	ds.mu.Unlock()

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Charts</h2>`)
	s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">Historical trends over %d simulated days</p>`, dayCount))

	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Net Interest Margin (bps)</h3>`)
	s.WriteString(buildNIMChart(nimHist))
	s.WriteString(`</div>`)

	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Balance History</h3>`)
	s.WriteString(buildBalanceChartSVG(balHist))
	s.WriteString(`</div>`)

	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Customer Count</h3>`)
	s.WriteString(buildCustomerChartSVG(custHist))
	s.WriteString(`</div>`)

	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">BoE Base Rate</h3>`)
	s.WriteString(buildBoERateGraph(boeHist))
	s.WriteString(`</div>`)

	return s.String()
}

// BuildBBSIHTML renders the BBSI annual report. Shows auth gate when not authorized.
func (ds *DemoState) BuildBBSIHTML(piiAuthorized bool) string {
	ds.mu.Lock()
	customers := make([]CustomerRecord, len(ds.customers))
	copy(customers, ds.customers)
	currentDay := ds.currentDay
	ds.mu.Unlock()

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">BBSI Report</h2>`)
	s.WriteString(`<p class="subtitle is-6 has-text-grey">Building Society / Bank Interest — Annual return to HMRC</p>`)
	s.WriteString(fmt.Sprintf(`<p class="mb-4">Tax year ending: %s</p>`, currentDay.Format("2 Jan 2006")))

	if !piiAuthorized {
		s.WriteString(`<div class="notification is-warning">
  <h3 class="title is-6">PII Access Required</h3>
  <p>This report contains personally identifiable information (names and NI numbers).</p>
  <form action="/auth/authorize" method="post" class="mt-2">
    <input type="hidden" name="redirect" value="/reports/bbsi">
    <button class="button is-warning is-small">Confirm PII Access</button>
  </form>
</div>`)
		return s.String()
	}

	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped is-hoverable">`)
	s.WriteString(`<thead><tr>
  <th>Customer</th><th>NI Number</th><th>Gross Interest Paid</th><th>Tax Deducted</th>
</tr></thead><tbody>`)

	totalInterest := 0.0
	for _, c := range customers {
		interest := 0.0
		for _, a := range c.Accounts {
			if a.Family == gbp.FamilySavings {
				interest += a.Interest
			}
		}
		if interest > 0 {
			totalInterest += interest
			piiData := ds.lookupPII(c.ID)
			name := piiData.Name
			ni := piiData.NI
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%s</td><td><code>%s</code></td><td>%s</td><td>%s</td>
</tr>`, name, ni, fmtMoney(interest), fmtMoney(0)))
		}
	}

	s.WriteString(`</tbody>`)
	s.WriteString(fmt.Sprintf(`<tfoot><tr class="has-text-weight-bold">
  <td colspan="2">Total</td><td>%s</td><td>%s</td>
</tr></tfoot>`, fmtMoney(totalInterest), fmtMoney(0)))
	s.WriteString(`</table></div>`)

	s.WriteString(`<div class="notification is-info is-light mt-4">
  <p><strong>Note:</strong> Since 6 April 2016, banks and building societies no longer deduct tax from interest.
  The BBSI return still reports gross interest paid for HMRC to reconcile against self-assessment.</p>
</div>`)

	return s.String()
}

// BuildCustomerViewHTML renders a comprehensive single-customer report.
func (ds *DemoState) BuildCustomerViewHTML(id string, piiAuthorized bool) string {
	ds.mu.Lock()
	var cust *CustomerRecord
	for i := range ds.customers {
		if ds.customers[i].ID == id {
			c := ds.customers[i]
			cust = &c
			break
		}
	}
	var custPayments []Payment
	if cust != nil {
		for _, p := range ds.payments {
			if p.FromID == cust.ID || p.ToID == cust.ID {
				custPayments = append(custPayments, p)
			}
		}
	}
	currentDay := ds.currentDay
	ds.mu.Unlock()

	if cust == nil {
		return `<div class="notification is-warning">Customer not found.</div>`
	}

	name := ds.lookupName(cust.ID)

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Customer Report</h2>`)

	if !piiAuthorized {
		s.WriteString(fmt.Sprintf(`<div class="notification is-warning">
  <h3 class="title is-6">PII Access Required</h3>
  <p>This report contains personally identifiable information.</p>
  <form action="/auth/authorize" method="post" class="mt-2">
    <input type="hidden" name="redirect" value="/reports/customer-view?id=%s">
    <button class="button is-warning is-small">Confirm PII Access</button>
  </form>
</div>`, cust.ID))
		return s.String()
	}

	piiData := ds.lookupPII(cust.ID)
	ni := piiData.NI

	// Customer summary box
	s.WriteString(`<div class="box">`)
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Name:</strong> %s</div>`, name))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>NI Number:</strong> <code>%s</code></div>`, ni))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Report Date:</strong> %s</div>`, currentDay.Format("2 Jan 2006")))
	s.WriteString(`</div></div>`)

	// Account details
	s.WriteString(`<h3 class="title is-5 mt-5">Account Summary</h3>`)
	totalSavings := 0.0
	totalLending := 0.0
	totalInterest := 0.0
	for _, a := range cust.Accounts {
		if a.Family == gbp.FamilySavings {
			totalSavings += a.Balance
		} else {
			totalLending += a.Balance
		}
		totalInterest += a.Interest
	}

	s.WriteString(`<div class="columns mb-4">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="notification is-success is-light has-text-centered"><p class="heading">Savings</p><p class="title is-5">%s</p></div></div>`, fmtMoney(totalSavings)))
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="notification is-info is-light has-text-centered"><p class="heading">Lending</p><p class="title is-5">%s</p></div></div>`, fmtMoney(totalLending)))
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="notification is-warning is-light has-text-centered"><p class="heading">Interest</p><p class="title is-5">%s</p></div></div>`, fmtMoney(totalInterest)))
	s.WriteString(`</div>`)

	// Account table
	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped">`)
	s.WriteString(`<thead><tr><th>Product</th><th>Type</th><th>Rate</th><th>Balance</th><th>Interest Accrued</th><th>Opened</th></tr></thead><tbody>`)
	for _, a := range cust.Accounts {
		familyTag := `<span class="tag is-success is-light">Savings</span>`
		if a.Family == gbp.FamilyLending {
			familyTag = `<span class="tag is-info is-light">Lending</span>`
		}
		s.WriteString(fmt.Sprintf(`<tr>
  <td>%s</td><td>%s</td><td>%.1f%%</td><td>%s</td><td>%s</td><td>%s</td>
</tr>`, a.ProductName, familyTag, a.Rate*100, fmtMoney(a.Balance), fmtMoney(a.Interest), a.OpenDate.Format("2 Jan 2006")))
	}
	s.WriteString(`</tbody></table></div>`)

	// Payment history
	if len(custPayments) > 0 {
		s.WriteString(`<h3 class="title is-5 mt-5">Payment History</h3>`)
		s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped">`)
		s.WriteString(`<thead><tr><th>ID</th><th>Direction</th><th>Counterparty</th><th>Amount</th><th>Status</th><th>Reference</th><th>Created</th></tr></thead><tbody>`)
		for i := len(custPayments) - 1; i >= 0; i-- {
			p := custPayments[i]
			direction := "Sent"
			counterpartyID := p.ToID
			if p.ToID == cust.ID {
				direction = "Received"
				counterpartyID = p.FromID
			}
			counterparty := ds.lookupName(counterpartyID)
			dirTag := `<span class="tag is-danger is-light">Sent</span>`
			if direction == "Received" {
				dirTag = `<span class="tag is-success is-light">Received</span>`
			}
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%d</td><td>%s</td><td>%s</td><td>%s</td>
  <td><span class="tag %s">%s</span></td><td>%s</td><td>%s</td>
</tr>`, p.ID, dirTag, counterparty, fmtMoney(p.Amount), p.Status.BulmaTag(), p.Status, p.Reference, p.CreatedAt.Format("15:04:05")))
		}
		s.WriteString(`</tbody></table></div>`)
	}

	return s.String()
}
