package main

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

type Customer struct {
	ID       string
	Name     string
	NI       string // simulated NI number
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

var customerNames = []string{
	"Alice", "Bob", "Charlie", "Diana", "Eve",
	"Frank", "Grace", "Henry", "Iris", "Jack",
}

func initCustomers(rng *rand.Rand, products []Product, startDate time.Time) []Customer {
	customers := make([]Customer, len(customerNames))
	// Simulated NI numbers
	niPrefixes := []string{"AB", "CD", "EF", "GH", "JK", "LM", "NP", "RS", "TW", "YZ"}

	for i, name := range customerNames {
		id := strings.ToLower(name)
		ni := fmt.Sprintf("%s%06dC", niPrefixes[i], 100000+rng.Intn(900000))

		// Each customer gets 1-3 accounts
		numAccounts := 1 + rng.Intn(3)
		// Shuffle product indices to pick random products
		perm := rng.Perm(len(products))
		if numAccounts > len(products) {
			numAccounts = len(products)
		}

		accounts := make([]CustomerAccount, numAccounts)
		for j := 0; j < numAccounts; j++ {
			p := products[perm[j]]
			balance := 0.0
			if p.Family == FamilySavings {
				balance = float64(500 + rng.Intn(9500)) // £500-£10,000
			} else {
				balance = float64(1000 + rng.Intn(49000)) // £1,000-£50,000
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

		customers[i] = Customer{
			ID:       id,
			Name:     name,
			NI:       ni,
			Accounts: accounts,
		}
	}
	return customers
}

// BuildCustomersHTML renders the customer list table.
func (ds *DemoState) BuildCustomersHTML() string {
	ds.mu.Lock()
	customers := make([]Customer, len(ds.customers))
	copy(customers, ds.customers)
	ds.mu.Unlock()

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Customers</h2>`)
	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped is-hoverable">`)
	s.WriteString(`<thead><tr><th>Name</th><th>Accounts</th><th>Total Savings</th><th>Total Lending</th><th></th></tr></thead><tbody>`)

	for _, c := range customers {
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
</tr>`, c.Name, len(c.Accounts), savings, lending, c.ID))
	}

	s.WriteString(`</tbody></table></div>`)
	return s.String()
}

// BuildCustomerDetailHTML renders a single customer's detail page.
func (ds *DemoState) BuildCustomerDetailHTML(id string) string {
	ds.mu.Lock()
	var cust *Customer
	for i := range ds.customers {
		if ds.customers[i].ID == id {
			c := ds.customers[i]
			cust = &c
			break
		}
	}
	// Grab recent payments for this customer
	var custPayments []Payment
	if cust != nil {
		for _, p := range ds.payments {
			if p.From == cust.Name || p.To == cust.Name {
				custPayments = append(custPayments, p)
			}
		}
	}
	ds.mu.Unlock()

	if cust == nil {
		return `<div class="notification is-warning">Customer not found.</div>`
	}

	var s strings.Builder
	s.WriteString(fmt.Sprintf(`<h2 class="title is-4">%s</h2>`, cust.Name))
	s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">ID: %s &middot; NI: %s</p>`, cust.ID, cust.NI))

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
		// Show last 10
		start := 0
		if len(custPayments) > 10 {
			start = len(custPayments) - 10
		}
		for i := len(custPayments) - 1; i >= start; i-- {
			p := custPayments[i]
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%d</td><td>%s</td><td>%s</td><td>£%.2f</td>
  <td><span class="tag %s">%s</span></td><td>%s</td>
</tr>`, p.ID, p.From, p.To, p.Amount, p.Status.BulmaTag(), p.Status, p.Reference))
		}
		s.WriteString(`</tbody></table></div>`)
	}

	return s.String()
}
