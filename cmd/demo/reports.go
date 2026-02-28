package main

import (
	"fmt"
	"strings"
)

// BuildBBSIHTML renders the BBSI (Building Society/Bank Interest) annual report for HMRC.
func (ds *DemoState) BuildBBSIHTML() string {
	ds.mu.Lock()
	customers := make([]Customer, len(ds.customers))
	copy(customers, ds.customers)
	currentDay := ds.currentDay
	ds.mu.Unlock()

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">BBSI Report</h2>`)
	s.WriteString(`<p class="subtitle is-6 has-text-grey">Building Society / Bank Interest — Annual return to HMRC</p>`)
	s.WriteString(fmt.Sprintf(`<p class="mb-4">Tax year ending: %s</p>`, currentDay.Format("2 Jan 2006")))

	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped is-hoverable">`)
	s.WriteString(`<thead><tr>
  <th>Customer</th><th>NI Number</th><th>Gross Interest Paid</th><th>Tax Deducted</th>
</tr></thead><tbody>`)

	totalInterest := 0.0
	for _, c := range customers {
		interest := 0.0
		for _, a := range c.Accounts {
			if a.Family == FamilySavings {
				interest += a.Interest
			}
		}
		if interest > 0 {
			totalInterest += interest
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%s</td><td><code>%s</code></td><td>£%.2f</td><td>£0.00</td>
</tr>`, c.Name, c.NI, interest))
		}
	}

	s.WriteString(`</tbody>`)
	s.WriteString(fmt.Sprintf(`<tfoot><tr class="has-text-weight-bold">
  <td colspan="2">Total</td><td>£%.2f</td><td>£0.00</td>
</tr></tfoot>`, totalInterest))
	s.WriteString(`</table></div>`)

	s.WriteString(`<div class="notification is-info is-light mt-4">
  <p><strong>Note:</strong> Since 6 April 2016, banks and building societies no longer deduct tax from interest.
  The BBSI return still reports gross interest paid for HMRC to reconcile against self-assessment.</p>
</div>`)

	return s.String()
}

// BuildCustomerViewHTML renders a comprehensive single-customer report.
func (ds *DemoState) BuildCustomerViewHTML(id string) string {
	ds.mu.Lock()
	var cust *Customer
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
			if p.From == cust.Name || p.To == cust.Name {
				custPayments = append(custPayments, p)
			}
		}
	}
	currentDay := ds.currentDay
	ds.mu.Unlock()

	if cust == nil {
		return `<div class="notification is-warning">Customer not found.</div>`
	}

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Customer Report</h2>`)

	// Customer summary box
	s.WriteString(`<div class="box">`)
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Name:</strong> %s</div>`, cust.Name))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>NI Number:</strong> <code>%s</code></div>`, cust.NI))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Report Date:</strong> %s</div>`, currentDay.Format("2 Jan 2006")))
	s.WriteString(`</div></div>`)

	// Account details
	s.WriteString(`<h3 class="title is-5 mt-5">Account Summary</h3>`)
	totalSavings := 0.0
	totalLending := 0.0
	totalInterest := 0.0
	for _, a := range cust.Accounts {
		if a.Family == FamilySavings {
			totalSavings += a.Balance
		} else {
			totalLending += a.Balance
		}
		totalInterest += a.Interest
	}

	s.WriteString(`<div class="columns mb-4">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="notification is-success is-light has-text-centered"><p class="heading">Savings</p><p class="title is-5">£%.2f</p></div></div>`, totalSavings))
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="notification is-info is-light has-text-centered"><p class="heading">Lending</p><p class="title is-5">£%.2f</p></div></div>`, totalLending))
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="notification is-warning is-light has-text-centered"><p class="heading">Interest</p><p class="title is-5">£%.2f</p></div></div>`, totalInterest))
	s.WriteString(`</div>`)

	// Account table
	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped">`)
	s.WriteString(`<thead><tr><th>Product</th><th>Type</th><th>Rate</th><th>Balance</th><th>Interest Accrued</th><th>Opened</th></tr></thead><tbody>`)
	for _, a := range cust.Accounts {
		familyTag := `<span class="tag is-success is-light">Savings</span>`
		if a.Family == FamilyLending {
			familyTag = `<span class="tag is-info is-light">Lending</span>`
		}
		s.WriteString(fmt.Sprintf(`<tr>
  <td>%s</td><td>%s</td><td>%.1f%%</td><td>£%.2f</td><td>£%.2f</td><td>%s</td>
</tr>`, a.ProductName, familyTag, a.Rate*100, a.Balance, a.Interest, a.OpenDate.Format("2 Jan 2006")))
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
			counterparty := p.To
			if p.To == cust.Name {
				direction = "Received"
				counterparty = p.From
			}
			dirTag := `<span class="tag is-danger is-light">Sent</span>`
			if direction == "Received" {
				dirTag = `<span class="tag is-success is-light">Received</span>`
			}
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%d</td><td>%s</td><td>%s</td><td>£%.2f</td>
  <td><span class="tag %s">%s</span></td><td>%s</td><td>%s</td>
</tr>`, p.ID, dirTag, counterparty, p.Amount, p.Status.BulmaTag(), p.Status, p.Reference, p.CreatedAt.Format("15:04:05")))
		}
		s.WriteString(`</tbody></table></div>`)
	}

	return s.String()
}
