package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
)

// lookupName returns the decrypted customer name from the SQL store, falling back to id.
func (ds *DemoState) lookupName(id string) string {
	if ds.custStore != nil {
		name, err := ds.custStore.GetNameByID(context.Background(), id)
		if err == nil && name != "" {
			return name
		}
	}
	return id
}

// lookupPII returns decrypted PII from the SQL store for a customer.
func (ds *DemoState) lookupPII(id string) PIIData {
	if ds.custStore != nil {
		pii, err := ds.custStore.GetPIIByID(context.Background(), id)
		if err == nil && pii != nil {
			return PIIData{
				Name:    pii.Name,
				NI:      pii.NI,
				DOB:     pii.DOB,
				Address: pii.Address,
				Email:   pii.Email,
				Phone:   pii.Phone,
			}
		}
	}
	return PIIData{NI: "unavailable", DOB: "unavailable", Address: "unavailable", Email: "unavailable", Phone: "unavailable"}
}

// custStoreCount returns the count of customers in the SQL store.
func (ds *DemoState) custStoreCount() int {
	if ds.custStore != nil {
		n, err := ds.custStore.Count(context.Background())
		if err == nil {
			return n
		}
	}
	return 0
}

// KYCStatus tracks Know Your Customer verification state.
type KYCStatus struct {
	Verified      bool
	LastCheckDate time.Time
	RiskRating    string // "Low", "Standard", "Medium"
}

// CustomerRecord holds non-sensitive account data. PII (name, NI, DOB, etc.)
// is stored separately in the encrypted PIIStore.
type CustomerRecord struct {
	ID        string
	Accounts  []CustomerAccount
	JoinDate  time.Time
	KYCStatus KYCStatus
}

type CustomerAccount struct {
	ProductID       string
	ProductName     string
	Family          gbp.ProductFamily
	Balance         float64
	Rate            float64
	Interest        float64 // accrued interest
	OpenDate        time.Time
	SortCode        string
	AccountNum      string
	LedgerAccountID string // go-luca account ID for dual-write
}

const customersPerPage = 50

// BuildCustomersHTML renders the customer list table with pagination.
// Names are shown only when piiAuth is true; otherwise only the customer ID is displayed.
func (ds *DemoState) BuildCustomersHTML(page int, piiAuth bool) string {
	ds.mu.Lock()
	customers := make([]CustomerRecord, len(ds.customers))
	copy(customers, ds.customers)
	ds.mu.Unlock()

	// Compute aggregate totals
	var aggSavings, aggLending float64
	for _, c := range customers {
		for _, a := range c.Accounts {
			if a.Family == gbp.FamilySavings {
				aggSavings += a.Balance
			} else {
				aggLending += a.Balance
			}
		}
	}

	total := len(customers)
	if page < 1 {
		page = 1
	}
	totalPages := max((total+customersPerPage-1)/customersPerPage, 1)
	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * customersPerPage
	end := min(start+customersPerPage, total)
	pageCustomers := customers[start:end]

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Customers</h2>`)
	s.WriteString(fmt.Sprintf(`<div class="field is-grouped is-grouped-multiline mb-4">
  <div class="control"><span class="tag is-info is-light">%d customers</span></div>
  <div class="control"><span class="tag is-success is-light">Savings: %s</span></div>
  <div class="control"><span class="tag is-link is-light">Lending: %s</span></div>
</div>`, total, fmtMoney(aggSavings), fmtMoney(aggLending)))
	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped is-hoverable">`)
	if piiAuth {
		s.WriteString(`<thead><tr><th>ID</th><th>Name</th><th>Accounts</th><th>Total Savings</th><th>Total Lending</th><th></th></tr></thead><tbody>`)
	} else {
		s.WriteString(`<thead><tr><th>ID</th><th>Accounts</th><th>Total Savings</th><th>Total Lending</th><th></th></tr></thead><tbody>`)
	}

	for _, c := range pageCustomers {
		savings := 0.0
		lending := 0.0
		for _, a := range c.Accounts {
			if a.Family == gbp.FamilySavings {
				savings += a.Balance
			} else {
				lending += a.Balance
			}
		}
		if piiAuth {
			name := ds.lookupName(c.ID)
			s.WriteString(fmt.Sprintf(`<tr>
  <td><code>%s</code></td><td>%s</td><td>%d</td><td>%s</td><td>%s</td>
  <td><a href="/customers/%s" class="button is-small is-link is-light">View</a></td>
</tr>`, c.ID, name, len(c.Accounts), fmtMoney(savings), fmtMoney(lending), c.ID))
		} else {
			s.WriteString(fmt.Sprintf(`<tr>
  <td><code>%s</code></td><td>%d</td><td>%s</td><td>%s</td>
  <td><a href="/customers/%s" class="button is-small is-link is-light">View</a></td>
</tr>`, c.ID, len(c.Accounts), fmtMoney(savings), fmtMoney(lending), c.ID))
		}
	}

	s.WriteString(`</tbody></table></div>`)

	// Pagination
	if totalPages > 1 {
		s.WriteString(`<nav class="pagination is-small mt-4" role="navigation">`)
		if page > 1 {
			s.WriteString(fmt.Sprintf(`<a class="pagination-previous" href="/customers?page=%d">Previous</a>`, page-1))
		} else {
			s.WriteString(`<a class="pagination-previous" disabled>Previous</a>`)
		}
		if page < totalPages {
			s.WriteString(fmt.Sprintf(`<a class="pagination-next" href="/customers?page=%d">Next</a>`, page+1))
		} else {
			s.WriteString(`<a class="pagination-next" disabled>Next</a>`)
		}
		s.WriteString(fmt.Sprintf(`<span class="pagination-list">Page %d of %d</span>`, page, totalPages))
		s.WriteString(`</nav>`)
	}

	return s.String()
}

const txPerDetailPage = 20

// phonePreviewFunc renders an inline phone preview for the admin customer pages.
// Set by init() in customers_http.go (HTTP mode only); nil in WASM mode.
var phonePreviewFunc func(ds *DemoState, custID string, accountIdx int) string

// BuildCustomerDetailHTML renders a single customer's detail page with two-column layout.
// Left column: summary, PII, KYC, accounts table. Right column: phone preview.
func (ds *DemoState) BuildCustomerDetailHTML(id string, piiAuthorized bool, txPage int) string {
	ds.mu.Lock()
	var cust *CustomerRecord
	for i := range ds.customers {
		if ds.customers[i].ID == id {
			c := ds.customers[i]
			cust = &c
			break
		}
	}
	ds.mu.Unlock()

	if cust == nil {
		return `<div class="notification is-warning">Customer not found.</div>`
	}

	name := ds.lookupName(cust.ID)

	// Compute aggregate values
	var totalSavings, totalLending float64
	for _, a := range cust.Accounts {
		if a.Family == gbp.FamilySavings {
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

	s.WriteString(fmt.Sprintf(`<div class="level"><div class="level-left"><div class="level-item"><h2 class="title is-4 mb-0">%s</h2></div><div class="level-item"><a href="/app/customer/%s" target="_blank" class="button is-small is-success is-outlined">Bank App</a></div></div></div>`, name, cust.ID))
	s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">ID: %s</p>`, cust.ID))

	s.WriteString(`<div class="columns">`)
	s.WriteString(`<div class="column is-7">`)

	// A. Summary panel (no PII)
	s.WriteString(`<div class="box">
<h3 class="title is-5">Summary</h3>
<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Customer since:</strong> %s (%s)</div>`, cust.JoinDate.Format("2 Jan 2006"), tenure))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Products:</strong> %d</div>`, len(cust.Accounts)))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Relationship value:</strong> %s</div>`, fmtMoney(relationshipValue)))
	s.WriteString(fmt.Sprintf(`<div class="column"><span class="tag is-success">Active</span></div>`))
	s.WriteString(`</div></div>`)

	// B. PII section (behind auth gate)
	if piiAuthorized {
		piiData := ds.lookupPII(cust.ID)

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
  <form action="/auth/authorize" method="post" class="mt-2">
    <input type="hidden" name="redirect" value="/customers/%s">
    <button class="button is-warning is-small">Confirm PII Access</button>
  </form>
</div>`, cust.ID))
	}

	// C. KYC panel
	kycTag := `<span class="tag is-success">Verified</span>`
	if !cust.KYCStatus.Verified {
		kycTag = `<span class="tag is-danger">Unverified</span>`
	}
	riskTag := `<span class="tag is-success is-light">Low</span>`
	switch cust.KYCStatus.RiskRating {
	case "Standard":
		riskTag = `<span class="tag is-info is-light">Standard</span>`
	case "Medium":
		riskTag = `<span class="tag is-warning is-light">Medium</span>`
	}
	s.WriteString(`<div class="box">
<h3 class="title is-5">KYC Status</h3>
<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Status:</strong> %s</div>`, kycTag))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Last Check:</strong> %s</div>`, cust.KYCStatus.LastCheckDate.Format("2 Jan 2006")))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Risk Rating:</strong> %s</div>`, riskTag))
	s.WriteString(`</div></div>`)

	// D. Accounts table with View links
	s.WriteString(`<h3 class="title is-5 mt-5">Accounts</h3>`)
	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped">`)
	s.WriteString(`<thead><tr><th>Product</th><th>Type</th><th>Sort Code</th><th>Account No.</th><th>Rate</th><th>Balance</th><th>Interest</th><th>Opened</th><th></th></tr></thead><tbody>`)
	for i, a := range cust.Accounts {
		familyTag := `<span class="tag is-success is-light">Savings</span>`
		if a.Family == gbp.FamilyLending {
			familyTag = `<span class="tag is-info is-light">Lending</span>`
		}
		s.WriteString(fmt.Sprintf(`<tr>
  <td>%s</td><td>%s</td><td><code>%s</code></td><td><code>%s</code></td><td>%.1f%%</td><td>%s</td><td>%s</td><td>%s</td>
  <td><a href="/customers/%s/account/%d" class="button is-small is-link is-light">View</a></td>
</tr>`, a.ProductName, familyTag, a.SortCode, a.AccountNum, a.Rate*100, fmtMoney(a.Balance), fmtMoney(a.Interest), a.OpenDate.Format("2 Jan 2006"), cust.ID, i))
	}
	s.WriteString(`</tbody></table></div>`)

	s.WriteString(`</div>`) // end column is-7

	// Right column: phone preview
	s.WriteString(`<div class="column is-5">`)
	if phonePreviewFunc != nil {
		s.WriteString(phonePreviewFunc(ds, cust.ID, -1))
	}
	s.WriteString(`</div>`)

	s.WriteString(`</div>`) // end columns

	return s.String()
}

// BuildCustomerAccountHTML renders a per-account detail page with transactions and phone preview.
func (ds *DemoState) BuildCustomerAccountHTML(custID string, accountIdx int, piiAuthorized bool, txPage int) string {
	ds.mu.Lock()
	var cust *CustomerRecord
	for i := range ds.customers {
		if ds.customers[i].ID == custID {
			c := ds.customers[i]
			cust = &c
			break
		}
	}
	ds.mu.Unlock()

	if cust == nil {
		return `<div class="notification is-warning">Customer not found.</div>`
	}
	if accountIdx < 0 || accountIdx >= len(cust.Accounts) {
		return `<div class="notification is-warning">Account not found.</div>`
	}

	name := ds.lookupName(cust.ID)
	a := cust.Accounts[accountIdx]

	var s strings.Builder

	// Breadcrumb
	s.WriteString(`<div class="level"><div class="level-left"><div class="level-item">`)
	s.WriteString(`<nav class="breadcrumb mb-0" aria-label="breadcrumbs"><ul>`)
	s.WriteString(`<li><a href="/customers">Customers</a></li>`)
	s.WriteString(fmt.Sprintf(`<li><a href="/customers/%s">%s</a></li>`, cust.ID, name))
	s.WriteString(fmt.Sprintf(`<li class="is-active"><a href="#">%s</a></li>`, a.ProductName))
	s.WriteString(`</ul></nav>`)
	s.WriteString(`</div><div class="level-item">`)
	s.WriteString(fmt.Sprintf(`<a href="/app/customer/%s/product/%d" target="_blank" class="button is-small is-success is-outlined">Bank App</a>`, cust.ID, accountIdx))
	s.WriteString(`</div></div></div>`)

	s.WriteString(`<div class="columns">`)
	s.WriteString(`<div class="column is-7">`)

	// Account detail card
	familyTag := `<span class="tag is-success is-light">Savings</span>`
	if a.Family == gbp.FamilyLending {
		familyTag = `<span class="tag is-info is-light">Lending</span>`
	}
	s.WriteString(`<div class="box">`)
	s.WriteString(fmt.Sprintf(`<h3 class="title is-5">%s %s</h3>`, a.ProductName, familyTag))
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Sort Code:</strong> <code>%s</code></div>`, a.SortCode))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Account No.:</strong> <code>%s</code></div>`, a.AccountNum))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Rate:</strong> %.2f%%</div>`, a.Rate*100))
	s.WriteString(`</div><div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Balance:</strong> %s</div>`, fmtMoney(a.Balance)))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Interest:</strong> %s</div>`, fmtMoney(a.Interest)))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Opened:</strong> %s</div>`, a.OpenDate.Format("2 Jan 2006")))
	s.WriteString(`</div></div>`)

	// Per-account transaction history
	if txPage < 1 {
		txPage = 1
	}
	txEntries, txTotal := ds.ProductTransactions(cust.ID, accountIdx, txPage, txPerDetailPage)
	txTotalPages := max((txTotal+txPerDetailPage-1)/txPerDetailPage, 1)

	s.WriteString(`<h3 class="title is-5 mt-5">Transaction History</h3>`)
	if txTotal == 0 {
		s.WriteString(`<p class="has-text-grey">No transactions yet.</p>`)
	} else {
		s.WriteString(fmt.Sprintf(`<p class="mb-2 has-text-grey is-size-7">%d transactions</p>`, txTotal))
		s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped is-hoverable">`)
		s.WriteString(`<thead><tr><th>Date</th><th>Type</th><th>Amount</th><th>Balance</th><th>Reference</th></tr></thead><tbody>`)
		for _, tx := range txEntries {
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td>
</tr>`, tx.Date.Format("2 Jan 2006"), tx.Type.String(), fmtMoney(tx.Amount), fmtMoney(tx.Balance), tx.Reference))
		}
		s.WriteString(`</tbody></table></div>`)

		if txTotalPages > 1 {
			s.WriteString(`<nav class="pagination is-small mt-4" role="navigation">`)
			if txPage > 1 {
				s.WriteString(fmt.Sprintf(`<a class="pagination-previous" href="/customers/%s/account/%d?txpage=%d">Previous</a>`, cust.ID, accountIdx, txPage-1))
			} else {
				s.WriteString(`<a class="pagination-previous" disabled>Previous</a>`)
			}
			if txPage < txTotalPages {
				s.WriteString(fmt.Sprintf(`<a class="pagination-next" href="/customers/%s/account/%d?txpage=%d">Next</a>`, cust.ID, accountIdx, txPage+1))
			} else {
				s.WriteString(`<a class="pagination-next" disabled>Next</a>`)
			}
			s.WriteString(fmt.Sprintf(`<span class="pagination-list">Page %d of %d</span>`, txPage, txTotalPages))
			s.WriteString(`</nav>`)
		}
	}

	s.WriteString(`</div>`) // end column is-7

	// Right column: phone preview
	s.WriteString(`<div class="column is-5">`)
	if phonePreviewFunc != nil {
		s.WriteString(phonePreviewFunc(ds, cust.ID, accountIdx))
	}
	s.WriteString(`</div>`)

	s.WriteString(`</div>`) // end columns

	return s.String()
}
