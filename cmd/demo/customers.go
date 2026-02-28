package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// CustomerRecord holds non-sensitive account data. PII (name, NI) is stored
// separately in the encrypted PIIStore.
type CustomerRecord struct {
	ID       string
	Accounts []CustomerAccount
}

type CustomerAccount struct {
	ProductID   string
	ProductName string
	Family      ProductFamily
	Balance     float64
	Rate        float64
	Interest    float64 // accrued interest
	OpenDate    time.Time
}

// Seed customer names (reduced from 10 to 3).
var seedCustomers = []struct {
	Name string
	NI   string
}{
	{"Alice", "AB123456C"},
	{"Bob", "CD234567C"},
	{"Charlie", "EF345678C"},
}

func initCustomers(rng *rand.Rand, products []Product, startDate time.Time, piiStore *PIIStore) []CustomerRecord {
	customers := make([]CustomerRecord, len(seedCustomers))

	for i, seed := range seedCustomers {
		id := strings.ToLower(seed.Name)

		// Store PII encrypted
		_ = piiStore.Store(id, seed.Name, seed.NI)

		// Each customer gets 1-3 accounts
		numAccounts := 1 + rng.Intn(3)
		perm := rng.Perm(len(products))
		if numAccounts > len(products) {
			numAccounts = len(products)
		}

		accounts := make([]CustomerAccount, numAccounts)
		for j := 0; j < numAccounts; j++ {
			p := products[perm[j]]
			balance := 0.0
			if p.Family == FamilySavings {
				balance = float64(500 + rng.Intn(9500))
			} else {
				balance = float64(1000 + rng.Intn(49000))
			}
			accounts[j] = CustomerAccount{
				ProductID:   p.ID,
				ProductName: p.Name,
				Family:      p.Family,
				Balance:     balance,
				Rate:        p.Rate,
				OpenDate:    startDate.AddDate(0, 0, -rng.Intn(365)),
			}
		}

		customers[i] = CustomerRecord{
			ID:       id,
			Accounts: accounts,
		}
	}
	return customers
}

// BuildCustomersHTML renders the customer list table.
// Names are looked up from piiStore; NI is not shown on the list page.
func (ds *DemoState) BuildCustomersHTML() string {
	ds.mu.Lock()
	customers := make([]CustomerRecord, len(ds.customers))
	copy(customers, ds.customers)
	piiStore := ds.piiStore
	ds.mu.Unlock()

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Customers</h2>`)
	s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">%d customers</p>`, len(customers)))
	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped is-hoverable">`)
	s.WriteString(`<thead><tr><th>Name</th><th>Accounts</th><th>Total Savings</th><th>Total Lending</th><th></th></tr></thead><tbody>`)

	for _, c := range customers {
		name := piiStore.RetrieveName(c.ID)
		savings := 0.0
		lending := 0.0
		for _, a := range c.Accounts {
			if a.Family == FamilySavings {
				savings += a.Balance
			} else {
				lending += a.Balance
			}
		}
		s.WriteString(fmt.Sprintf(`<tr>
  <td>%s</td><td>%d</td><td>£%.2f</td><td>£%.2f</td>
  <td><a href="/customers/%s" class="button is-small is-link is-light">View</a></td>
</tr>`, name, len(c.Accounts), savings, lending, c.ID))
	}

	s.WriteString(`</tbody></table></div>`)
	return s.String()
}

// BuildCustomerDetailHTML renders a single customer's detail page.
// Shows auth gate if piiAuthorized is false; NI only shown when authorized.
func (ds *DemoState) BuildCustomerDetailHTML(id string, piiAuthorized bool) string {
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
	piiStore := ds.piiStore
	ds.mu.Unlock()

	if cust == nil {
		return `<div class="notification is-warning">Customer not found.</div>`
	}

	name := piiStore.RetrieveName(cust.ID)

	var s strings.Builder
	s.WriteString(fmt.Sprintf(`<h2 class="title is-4">%s</h2>`, name))

	if piiAuthorized {
		_, ni, err := piiStore.Retrieve(cust.ID)
		niDisplay := "unavailable"
		if err == nil {
			niDisplay = ni
		}
		s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">ID: %s &middot; NI: <code>%s</code></p>`, cust.ID, niDisplay))
	} else {
		s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">ID: %s</p>`, cust.ID))
		s.WriteString(fmt.Sprintf(`<div class="notification is-warning">
  <h3 class="title is-6">PII Access Required</h3>
  <p>National Insurance number is protected. Authorize to view.</p>
  <form action="/auth/authorize" method="post" class="mt-2">
    <input type="hidden" name="redirect" value="/customers/%s">
    <button class="button is-warning is-small">Confirm PII Access</button>
  </form>
</div>`, cust.ID))
	}

	// Accounts table
	s.WriteString(`<h3 class="title is-5 mt-5">Accounts</h3>`)
	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped">`)
	s.WriteString(`<thead><tr><th>Product</th><th>Type</th><th>Rate</th><th>Balance</th><th>Interest</th><th>Opened</th></tr></thead><tbody>`)
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

	// Recent payments
	if len(custPayments) > 0 {
		s.WriteString(`<h3 class="title is-5 mt-5">Recent Payments</h3>`)
		s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped">`)
		s.WriteString(`<thead><tr><th>ID</th><th>From</th><th>To</th><th>Amount</th><th>Status</th><th>Reference</th></tr></thead><tbody>`)
		start := 0
		if len(custPayments) > 10 {
			start = len(custPayments) - 10
		}
		for i := len(custPayments) - 1; i >= start; i-- {
			p := custPayments[i]
			fromName := piiStore.RetrieveName(p.FromID)
			toName := piiStore.RetrieveName(p.ToID)
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%d</td><td>%s</td><td>%s</td><td>£%.2f</td>
  <td><span class="tag %s">%s</span></td><td>%s</td>
</tr>`, p.ID, fromName, toName, p.Amount, p.Status.BulmaTag(), p.Status, p.Reference))
		}
		s.WriteString(`</tbody></table></div>`)
	}

	return s.String()
}
