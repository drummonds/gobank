package main

import (
	"fmt"
	"strings"
	"time"

	gbp "codeberg.org/hum3/gobank-products"
)

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
// Names are looked up from piiStore; NI is not shown on the list page.
func (ds *DemoState) BuildCustomersHTML(page int) string {
	ds.mu.Lock()
	customers := make([]CustomerRecord, len(ds.customers))
	copy(customers, ds.customers)
	piiStore := ds.piiStore
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
	totalPages := (total + customersPerPage - 1) / customersPerPage
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * customersPerPage
	end := start + customersPerPage
	if end > total {
		end = total
	}
	pageCustomers := customers[start:end]

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Customers</h2>`)
	s.WriteString(fmt.Sprintf(`<div class="field is-grouped is-grouped-multiline mb-4">
  <div class="control"><span class="tag is-info is-light">%d customers</span></div>
  <div class="control"><span class="tag is-success is-light">Savings: %s</span></div>
  <div class="control"><span class="tag is-link is-light">Lending: %s</span></div>
</div>`, total, fmtMoney(aggSavings), fmtMoney(aggLending)))
	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped is-hoverable">`)
	s.WriteString(`<thead><tr><th>Name</th><th>Accounts</th><th>Total Savings</th><th>Total Lending</th><th></th></tr></thead><tbody>`)

	for _, c := range pageCustomers {
		name := piiStore.RetrieveName(c.ID)
		savings := 0.0
		lending := 0.0
		for _, a := range c.Accounts {
			if a.Family == gbp.FamilySavings {
				savings += a.Balance
			} else {
				lending += a.Balance
			}
		}
		s.WriteString(fmt.Sprintf(`<tr>
  <td>%s</td><td>%d</td><td>%s</td><td>%s</td>
  <td><a href="/customers/%s" class="button is-small is-link is-light">View</a></td>
</tr>`, name, len(c.Accounts), fmtMoney(savings), fmtMoney(lending), c.ID))
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

// BuildCustomerDetailHTML renders a single customer's detail page with card-based layout.
// Shows auth gate if piiAuthorized is false; full PII only shown when authorized.
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
	piiStore := ds.piiStore
	ds.mu.Unlock()

	if cust == nil {
		return `<div class="notification is-warning">Customer not found.</div>`
	}

	name := piiStore.RetrieveName(cust.ID)

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

	s.WriteString(fmt.Sprintf(`<h2 class="title is-4">%s</h2>`, name))
	s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">ID: %s</p>`, cust.ID))

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
		piiData, err := piiStore.Retrieve(cust.ID)
		if err != nil {
			piiData = PIIData{NI: "unavailable", DOB: "unavailable", Address: "unavailable", Email: "unavailable", Phone: "unavailable"}
		}
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

	// D. Accounts table with sort code + account number
	s.WriteString(`<h3 class="title is-5 mt-5">Accounts</h3>`)
	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped">`)
	s.WriteString(`<thead><tr><th>Product</th><th>Type</th><th>Sort Code</th><th>Account No.</th><th>Rate</th><th>Balance</th><th>Interest</th><th>Opened</th></tr></thead><tbody>`)
	for _, a := range cust.Accounts {
		familyTag := `<span class="tag is-success is-light">Savings</span>`
		if a.Family == gbp.FamilyLending {
			familyTag = `<span class="tag is-info is-light">Lending</span>`
		}
		s.WriteString(fmt.Sprintf(`<tr>
  <td>%s</td><td>%s</td><td><code>%s</code></td><td><code>%s</code></td><td>%.1f%%</td><td>%s</td><td>%s</td><td>%s</td>
</tr>`, a.ProductName, familyTag, a.SortCode, a.AccountNum, a.Rate*100, fmtMoney(a.Balance), fmtMoney(a.Interest), a.OpenDate.Format("2 Jan 2006")))
	}
	s.WriteString(`</tbody></table></div>`)

	// E. Transaction history (paginated from txlog)
	if txPage < 1 {
		txPage = 1
	}
	txEntries, txTotal := ds.CustomerTransactions(cust.ID, txPage, txPerDetailPage)
	txTotalPages := (txTotal + txPerDetailPage - 1) / txPerDetailPage
	if txTotalPages < 1 {
		txTotalPages = 1
	}

	s.WriteString(`<h3 class="title is-5 mt-5">Transaction History</h3>`)
	if txTotal == 0 {
		s.WriteString(`<p class="has-text-grey">No transactions yet.</p>`)
	} else {
		s.WriteString(fmt.Sprintf(`<p class="mb-2 has-text-grey is-size-7">%d transactions</p>`, txTotal))
		s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped is-hoverable">`)
		s.WriteString(`<thead><tr><th>Date</th><th>Product</th><th>Type</th><th>Amount</th><th>Balance</th><th>Reference</th></tr></thead><tbody>`)
		for _, tx := range txEntries {
			typeTag := tx.Type.String()
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td>
</tr>`, tx.Date.Format("2 Jan 2006"), tx.ProductName, typeTag, fmtMoney(tx.Amount), fmtMoney(tx.Balance), tx.Reference))
		}
		s.WriteString(`</tbody></table></div>`)

		// Pagination
		if txTotalPages > 1 {
			s.WriteString(`<nav class="pagination is-small mt-4" role="navigation">`)
			if txPage > 1 {
				s.WriteString(fmt.Sprintf(`<a class="pagination-previous" href="/customers/%s?txpage=%d">Previous</a>`, cust.ID, txPage-1))
			} else {
				s.WriteString(`<a class="pagination-previous" disabled>Previous</a>`)
			}
			if txPage < txTotalPages {
				s.WriteString(fmt.Sprintf(`<a class="pagination-next" href="/customers/%s?txpage=%d">Next</a>`, cust.ID, txPage+1))
			} else {
				s.WriteString(`<a class="pagination-next" disabled>Next</a>`)
			}
			s.WriteString(fmt.Sprintf(`<span class="pagination-list">Page %d of %d</span>`, txPage, txTotalPages))
			s.WriteString(`</nav>`)
		}
	}

	return s.String()
}
