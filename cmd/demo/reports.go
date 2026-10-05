package main

import (
	"context"
	"fmt"
	"strings"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
)

// buildChartsHTML renders historical charts: NIM, balances, customer count, BoE rate.
func buildChartsHTML(q core.BookQueries) string {
	ctx := context.Background()
	hist, _ := q.History(ctx)
	pos, _ := q.Position(ctx)
	nimHist, balHist, custHist, boeHist := hist.NIM, hist.Balances, hist.Customers, hist.BoERate
	dayCount := pos.DayCount

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

// buildBBSIHTML renders the BBSI annual report. Shows auth gate when not authorized.
func buildBBSIHTML(q core.StaffQueries, piiAuthorized bool) string {
	ctx := context.Background()
	pos, _ := q.Position(ctx)
	currentDay := pos.Day

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

	// BBSI reports gross interest paid: applied plus accrued-but-unapplied.
	var totalInterest luca.Amount
	rows, _ := q.SavingsInterestByCustomer(ctx)
	for _, row := range rows {
		totalInterest += row.Interest
		piiData, _ := q.CustomerPII(ctx, row.CustomerID)
		s.WriteString(fmt.Sprintf(`<tr>
  <td>%s</td><td><code>%s</code></td><td>%s</td><td>%s</td>
</tr>`, piiData.Name, piiData.NI, fmtMoney(row.Interest), fmtMoney(0)))
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

// buildCustomerViewHTML renders a comprehensive single-customer report.
func buildCustomerViewHTML(q core.StaffQueries, id string, piiAuthorized bool) string {
	ctx := context.Background()
	cust, err := q.CustomerRecord(ctx, id)
	if err != nil {
		return `<div class="notification is-warning">Customer not found.</div>`
	}
	pos, _ := q.Position(ctx)
	currentDay := pos.Day
	custPayments, _ := q.PaymentsOf(ctx, cust.ID)

	name, _ := q.CustomerName(ctx, cust.ID)

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

	piiData, _ := q.CustomerPII(ctx, cust.ID)
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
	var totalSavings, totalLending, totalInterest luca.Amount
	for _, a := range cust.Accounts {
		if a.Family == string(gbp.FamilySavings) {
			totalSavings += a.Balance
		} else {
			totalLending += a.Balance
		}
		totalInterest += a.Interest + poundsE7(a.AccruedE7).Pence()
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
		interestCell := fmtMoney(a.Interest)
		if accrued := poundsE7(a.AccruedE7); accrued != 0 {
			// Accrued-but-unapplied interest modelled as 7dp pounds; it
			// converts to whole pence on application.
			interestCell += fmt.Sprintf(` <span class="has-text-grey is-size-7">+%s accruing</span>`, accrued)
		}
		s.WriteString(fmt.Sprintf(`<tr>
  <td>%s</td><td>%s</td><td>%.1f%%</td><td>%s</td><td>%s</td><td>%s</td>
</tr>`, a.ProductName, familyTag(a.Family), a.Rate*100, fmtMoney(a.Balance), interestCell, fmtISODate(a.OpenDate)))
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
			counterpartyID := p.To
			if p.To == cust.ID {
				direction = "Received"
				counterpartyID = p.From
			}
			counterparty := counterpartyID
			if n, err := q.CustomerName(ctx, counterpartyID); err == nil {
				counterparty = n
			}
			dirTag := `<span class="tag is-danger is-light">Sent</span>`
			if direction == "Received" {
				dirTag = `<span class="tag is-success is-light">Received</span>`
			}
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%d</td><td>%s</td><td>%s</td><td>%s</td>
  <td><span class="tag %s">%s</span></td><td>%s</td><td>%s</td>
</tr>`, p.ID, dirTag, counterparty, fmtMoney(p.Amount), paymentStatusTag(p.Status), p.Status, p.Reference, fmtUTC(p.CreatedAt)))
		}
		s.WriteString(`</tbody></table></div>`)
	}

	return s.String()
}
