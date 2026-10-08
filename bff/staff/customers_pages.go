package staff

import (
	"context"
	"fmt"
	"strings"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
)

// The customers pages: the register, a customer and one of their
// accounts, read through the core.

// fmtISODate shows a core date (2006-01-02) as the staff pages do.
func fmtISODate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("2 Jan 2006")
}

// familyTag is the Bulma tag for an account's product family.
func familyTag(family string) string {
	if family == string(gbp.FamilyLending) {
		return `<span class="tag is-info is-light">Lending</span>`
	}
	return `<span class="tag is-success is-light">Savings</span>`
}

// BuildCustomersHTML renders the customer list table with pagination.
// Names are shown only when piiAuth is true; otherwise only the customer ID is displayed.
func BuildCustomersHTML(q core.StaffQueries, page int, piiAuth bool) string {
	ctx := context.Background()
	pos, _ := q.Position(ctx)
	aggSavings, aggLending := pos.Savings, pos.Lending

	cp, _ := q.CustomerPage(ctx, page)
	if cp.Page > cp.TotalPages() {
		cp, _ = q.CustomerPage(ctx, cp.TotalPages())
	}
	page, totalPages, total := cp.Page, cp.TotalPages(), cp.Total

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Customers</h2>`)
	s.WriteString(fmt.Sprintf(`<div class="field is-grouped is-grouped-multiline mb-4">
  <div class="control"><span class="tag is-info is-light">%d customers</span></div>
  <div class="control"><span class="tag is-success is-light">Savings: %s</span></div>
  <div class="control"><span class="tag is-link is-light">Lending: %s</span></div>
</div>`, total, FormatMoney(aggSavings), FormatMoney(aggLending)))
	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped is-hoverable">`)
	if piiAuth {
		s.WriteString(`<thead><tr><th>ID</th><th>Name</th><th>Accounts</th><th>Total Savings</th><th>Total Lending</th><th></th></tr></thead><tbody>`)
	} else {
		s.WriteString(`<thead><tr><th>ID</th><th>Accounts</th><th>Total Savings</th><th>Total Lending</th><th></th></tr></thead><tbody>`)
	}

	for _, c := range cp.Customers {
		if piiAuth {
			name, _ := q.CustomerName(ctx, c.ID)
			s.WriteString(fmt.Sprintf(`<tr>
  <td><code>%s</code></td><td>%s</td><td>%d</td><td>%s</td><td>%s</td>
  <td><a href="customers/%s" class="button is-small is-link is-light">View</a></td>
</tr>`, c.ID, name, c.Accounts, FormatMoney(c.Savings), FormatMoney(c.Lending), c.ID))
		} else {
			s.WriteString(fmt.Sprintf(`<tr>
  <td><code>%s</code></td><td>%d</td><td>%s</td><td>%s</td>
  <td><a href="customers/%s" class="button is-small is-link is-light">View</a></td>
</tr>`, c.ID, c.Accounts, FormatMoney(c.Savings), FormatMoney(c.Lending), c.ID))
		}
	}

	s.WriteString(`</tbody></table></div>`)

	// Pagination
	if totalPages > 1 {
		s.WriteString(`<nav class="pagination is-small mt-4" role="navigation">`)
		if page > 1 {
			s.WriteString(fmt.Sprintf(`<a class="pagination-previous" href="customers?page=%d">Previous</a>`, page-1))
		} else {
			s.WriteString(`<a class="pagination-previous" disabled>Previous</a>`)
		}
		if page < totalPages {
			s.WriteString(fmt.Sprintf(`<a class="pagination-next" href="customers?page=%d">Next</a>`, page+1))
		} else {
			s.WriteString(`<a class="pagination-next" disabled>Next</a>`)
		}
		s.WriteString(fmt.Sprintf(`<span class="pagination-list">Page %d of %d</span>`, page, totalPages))
		s.WriteString(`</nav>`)
	}

	return s.String()
}

// customerLabel is how a page names a customer: the name with PII
// authorisation, otherwise the ID (the rule every page follows, see
// partyLabel in payments.go).
func customerLabel(q core.StaffQueries, id string, piiAuthorized bool) string {
	if piiAuthorized {
		if name, err := q.CustomerName(context.Background(), id); err == nil && name != "" {
			return name
		}
	}
	return id
}

// BuildCustomerDetailHTML renders a single customer's detail page: summary,
// PII, KYC, accounts table.
func BuildCustomerDetailHTML(q core.StaffQueries, id string, piiAuthorized bool, txPage int) string {
	ctx := context.Background()
	cust, err := q.CustomerRecord(ctx, id)
	if err != nil {
		return `<div class="notification is-warning">Customer not found.</div>`
	}

	name := customerLabel(q, cust.ID, piiAuthorized)

	// Compute aggregate values
	var totalSavings, totalLending luca.Amount
	for _, a := range cust.Accounts {
		if a.Family == string(gbp.FamilySavings) {
			totalSavings += a.Balance
		} else {
			totalLending += a.Balance
		}
	}
	relationshipValue := totalSavings + totalLending

	// Tenure
	tenure := ""
	years := int(time.Since(cust.JoinDate).Hours() / 24 / 365)
	if years > 0 {
		tenure = fmt.Sprintf("%d years", years)
	} else {
		days := int(time.Since(cust.JoinDate).Hours() / 24)
		tenure = fmt.Sprintf("%d days", days)
	}

	var s strings.Builder

	s.WriteString(fmt.Sprintf(`<div class="level"><div class="level-left"><div class="level-item"><h2 class="title is-4 mb-0">%s</h2></div><div class="level-item"><a href="v1/screen/login" target="_blank" class="button is-small is-success is-outlined">Bank App</a></div></div></div>`, name))
	s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">ID: %s</p>`, cust.ID))

	// A. Summary panel (no PII)
	s.WriteString(`<div class="box">
<h3 class="title is-5">Summary</h3>
<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Customer since:</strong> %s (%s)</div>`, cust.JoinDate.Format("2 Jan 2006"), tenure))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Products:</strong> %d</div>`, len(cust.Accounts)))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Relationship value:</strong> %s</div>`, FormatMoney(relationshipValue)))
	s.WriteString(`<div class="column"><span class="tag is-success">Active</span></div>`)
	s.WriteString(`</div></div>`)

	// B. PII section (behind auth gate)
	if piiAuthorized {
		piiData, _ := q.CustomerPII(ctx, cust.ID)

		s.WriteString(`<div class="box">
<h3 class="title is-5">Personal Information</h3>
<div class="columns">
<div class="column">`)
		s.WriteString(fmt.Sprintf(`<p><strong>Date of Birth:</strong> %s</p>`, piiData.DOB))
		s.WriteString(fmt.Sprintf(`<p><strong>NI Number:</strong> <code>%s</code></p>`, piiData.NI))
		s.WriteString(fmt.Sprintf(`<p><strong>Address:</strong> %s</p>`, piiData.Address))
		s.WriteString(`</div><div class="column">`)
		s.WriteString(fmt.Sprintf(`<p><strong>Email:</strong> %s</p>`, piiData.Email))
		s.WriteString(fmt.Sprintf(`<p><strong>Phone:</strong> %s</p>`, piiData.Phone))
		s.WriteString(`</div></div></div>`)
	} else {
		s.WriteString(fmt.Sprintf(`<div class="notification is-warning">
  <h3 class="title is-6">PII Access Required</h3>
  <p>Personal information is protected. Authorize to view.</p>
  <form action="auth/authorize" method="post" class="mt-2">
    <input type="hidden" name="redirect" value="customers/%s">
    <button class="button is-warning is-small">Confirm PII Access</button>
  </form>
</div>`, cust.ID))
	}

	// C. KYC panel
	kycTag := `<span class="tag is-success">Verified</span>`
	if !cust.KYC.Verified {
		kycTag = `<span class="tag is-danger">Unverified</span>`
	}
	riskTag := `<span class="tag is-success is-light">Low</span>`
	switch cust.KYC.RiskRating {
	case "Standard":
		riskTag = `<span class="tag is-info is-light">Standard</span>`
	case "Medium":
		riskTag = `<span class="tag is-warning is-light">Medium</span>`
	}
	s.WriteString(`<div class="box">
<h3 class="title is-5">KYC Status</h3>
<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Status:</strong> %s</div>`, kycTag))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Last Check:</strong> %s</div>`, cust.KYC.LastCheck.Format("2 Jan 2006")))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Risk Rating:</strong> %s</div>`, riskTag))
	s.WriteString(`</div></div>`)

	// D. Accounts table with View links
	s.WriteString(`<h3 class="title is-5 mt-5">Accounts</h3>`)
	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped">`)
	s.WriteString(`<thead><tr><th>Product</th><th>Type</th><th>Sort Code</th><th>Account No.</th><th>Rate</th><th>Balance</th><th>Interest</th><th>Opened</th><th></th></tr></thead><tbody>`)
	for i, a := range cust.Accounts {
		s.WriteString(fmt.Sprintf(`<tr>
  <td>%s</td><td>%s</td><td><code>%s</code></td><td><code>%s</code></td><td>%.1f%%</td><td>%s</td><td>%s</td><td>%s</td>
  <td><a href="customers/%s/account/%d" class="button is-small is-link is-light">View</a></td>
</tr>`, a.ProductName, familyTag(a.Family), a.SortCode, a.AccountNum, a.Rate*100, FormatMoney(a.Balance), FormatMoney(a.Interest), fmtISODate(a.OpenDate), cust.ID, i))
	}
	s.WriteString(`</tbody></table></div>`)

	return s.String()
}

// BuildCustomerAccountHTML renders a per-account detail page with transactions.
func BuildCustomerAccountHTML(q core.StaffQueries, custID string, accountIdx int, piiAuthorized bool, txPage int) string {
	ctx := context.Background()
	cust, err := q.CustomerRecord(ctx, custID)
	if err != nil {
		return `<div class="notification is-warning">Customer not found.</div>`
	}
	if accountIdx < 0 || accountIdx >= len(cust.Accounts) {
		return `<div class="notification is-warning">Account not found.</div>`
	}

	name := customerLabel(q, cust.ID, piiAuthorized)
	a := cust.Accounts[accountIdx]

	var s strings.Builder

	// Breadcrumb
	s.WriteString(`<div class="level"><div class="level-left"><div class="level-item">`)
	s.WriteString(`<nav class="breadcrumb mb-0" aria-label="breadcrumbs"><ul>`)
	s.WriteString(`<li><a href="customers">Customers</a></li>`)
	s.WriteString(fmt.Sprintf(`<li><a href="customers/%s">%s</a></li>`, cust.ID, name))
	s.WriteString(fmt.Sprintf(`<li class="is-active"><a aria-current="page">%s</a></li>`, a.ProductName))
	s.WriteString(`</ul></nav>`)
	s.WriteString(`</div><div class="level-item">`)
	s.WriteString(`<a href="v1/screen/login" target="_blank" class="button is-small is-success is-outlined">Bank App</a>`)
	s.WriteString(`</div></div></div>`)

	// Account detail card
	s.WriteString(`<div class="box">`)
	s.WriteString(fmt.Sprintf(`<h3 class="title is-5">%s %s</h3>`, a.ProductName, familyTag(a.Family)))
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Sort Code:</strong> <code>%s</code></div>`, a.SortCode))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Account No.:</strong> <code>%s</code></div>`, a.AccountNum))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Rate:</strong> %.2f%%</div>`, a.Rate*100))
	s.WriteString(`</div><div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Balance:</strong> %s</div>`, FormatMoney(a.Balance)))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Interest:</strong> %s</div>`, FormatMoney(a.Interest)))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Opened:</strong> %s</div>`, fmtISODate(a.OpenDate)))
	s.WriteString(`</div></div>`)

	// Per-account transaction history
	txs, _ := q.AccountTransactions(ctx, cust.ID, accountIdx, txPage)
	txPage = txs.Page
	txTotalPages := max((txs.Total+txs.PerPage-1)/max(txs.PerPage, 1), 1)

	s.WriteString(`<h3 class="title is-5 mt-5">Transaction History</h3>`)
	if txs.Total == 0 {
		s.WriteString(`<p class="has-text-grey">No transactions yet.</p>`)
	} else {
		s.WriteString(fmt.Sprintf(`<p class="mb-2 has-text-grey is-size-7">%d transactions</p>`, txs.Total))
		s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped is-hoverable">`)
		s.WriteString(`<thead><tr><th>Date</th><th>Type</th><th>Amount</th><th>Balance</th><th>Reference</th></tr></thead><tbody>`)
		for _, tx := range txs.Entries {
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td>
</tr>`, fmtISODate(tx.Date), tx.Type, FormatMoney(tx.Amount), FormatMoney(tx.Balance), tx.Reference))
		}
		s.WriteString(`</tbody></table></div>`)

		if txTotalPages > 1 {
			s.WriteString(`<nav class="pagination is-small mt-4" role="navigation">`)
			if txPage > 1 {
				s.WriteString(fmt.Sprintf(`<a class="pagination-previous" href="customers/%s/account/%d?txpage=%d">Previous</a>`, cust.ID, accountIdx, txPage-1))
			} else {
				s.WriteString(`<a class="pagination-previous" disabled>Previous</a>`)
			}
			if txPage < txTotalPages {
				s.WriteString(fmt.Sprintf(`<a class="pagination-next" href="customers/%s/account/%d?txpage=%d">Next</a>`, cust.ID, accountIdx, txPage+1))
			} else {
				s.WriteString(`<a class="pagination-next" disabled>Next</a>`)
			}
			s.WriteString(fmt.Sprintf(`<span class="pagination-list">Page %d of %d</span>`, txPage, txTotalPages))
			s.WriteString(`</nav>`)
		}
	}

	return s.String()
}
