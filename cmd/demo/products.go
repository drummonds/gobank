package main

import (
	"fmt"
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
func (ds *DemoState) BuildProductsHTML(family ProductFamily) string {
	ds.mu.Lock()
	products := ds.products
	customers := make([]Customer, len(ds.customers))
	copy(customers, ds.customers)
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
		s.WriteString(fmt.Sprintf(`<div class="column"><strong>Total:</strong> £%.2f</div>`, totalBal))
		s.WriteString(`</div></div>`)
	}

	return s.String()
}
