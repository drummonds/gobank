package main

import (
	"context"
	"git.bytestone.uk/hum3/gobank/bank/products"
	"testing"
)

// The currency is a property of the product (#26). It reaches the
// account and its transactions; the customer web's screens pick the
// symbol from it (TestCustomerWebNeedsASession).
func TestCurrencyComesFromTheProduct(t *testing.T) {
	for _, p := range products.Catalogue() {
		if p.Currency != "GBP" {
			t.Errorf("product %s has currency %q, want GBP", p.ID, p.Currency)
		}
	}
	ds := NewDemoState()
	addFundedCustomer(ds)
	a := ds.Bank
	ctx := context.Background()
	accounts, err := a.Accounts(ctx, "cust-001")
	if err != nil || len(accounts) == 0 {
		t.Fatalf("Accounts: %v, %d", err, len(accounts))
	}
	for _, acc := range accounts {
		if acc.Currency != "GBP" {
			t.Errorf("account %s has currency %q, want GBP", acc.ProductName, acc.Currency)
		}
	}
	page, err := a.Transactions(ctx, "cust-001", 1)
	if err != nil || len(page.Entries) == 0 {
		t.Fatalf("Transactions: %v, %d", err, len(page.Entries))
	}
	for _, tx := range page.Entries {
		if tx.Currency != "GBP" {
			t.Errorf("transaction %s has currency %q, want GBP", tx.ID, tx.Currency)
		}
	}
	products, _ := a.Products(ctx)
	for _, p := range products {
		if p.Currency != "GBP" {
			t.Errorf("core product %s has currency %q, want GBP", p.ID, p.Currency)
		}
	}

}
