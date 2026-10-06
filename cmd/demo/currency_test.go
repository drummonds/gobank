package main

import (
	"context"
	"git.bytestone.uk/hum3/gobank/bank/products"
	"strings"
	"testing"
)

// The currency is a property of the product (#26). It reaches the
// account, its transactions and the app's screens, which pick the symbol
// from it: a GBP savings account shows a pound note, never the dollar
// sign the renderers used to hard-code.
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

	const dollar, pound = "&#128178;", "&#128183;"
	for what, html := range map[string]string{
		"balance":      buildAppBalanceHTML(ds.Bank, "cust-001"),
		"transactions": buildAppTransactionsHTML(ds.Bank, "cust-001", 1),
		"product":      buildAppProductHTML(ds.Bank, "cust-001", 0, 1),
	} {
		if strings.Contains(html, dollar) {
			t.Errorf("app %s page shows the dollar sign", what)
		}
	}
	if !strings.Contains(buildAppBalanceHTML(ds.Bank, "cust-001"), pound) {
		t.Error("app balance page shows no pound note for a GBP savings account")
	}
}
