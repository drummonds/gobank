package main

import (
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/bff/staff"
	"git.bytestone.uk/hum3/gobank/internal/view"
)

// The bank tab under Products carries what is the bank's and not a
// product's: the pound's market with the rate in force and its next
// decision day, the reserve ratio, and the parameters at the market and
// bank scopes, each a row that loads the parameter's own page into the
// detail. Nothing of the bank's is yet settable.
func TestBankTabShowsTheMarketAndTheReserveRatio(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	ds.AdvanceDay()
	page := staff.BuildBankHTML(ds.Bank, false)
	for _, want := range []string{"Bank of England", "Bank Rate", "Next decision day: 30 Jan 2020 12:00 UTC", "Reserve ratio", "15%",
		`hx-get="products/GBP/parameters/risk_free_rate_bps"`, "<svg", `id="parameter-detail"`, "policy as code"} {
		if !strings.Contains(page, want) {
			t.Errorf("bank tab lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "hx-trigger") {
		t.Error("a bank tab that is not polling polls")
	}
	if polling := staff.BuildBankHTML(ds.Bank, true); !strings.Contains(polling, `hx-trigger="every 2s"`) || !strings.Contains(polling, `hx-select="#products-master"`) {
		t.Error("while the simulation runs the master re-polls itself, leaving the detail alone")
	}
}

// The product total on the products page follows the session's view, as
// every other balance does: the bank's view shows a savings book
// negative, the customer's positive.
func TestProductTotalFollowsTheView(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	bank := staff.BuildProductsHTML(ds.Bank, "Savings", view.Bank, false)
	customer := staff.BuildProductsHTML(ds.Bank, "Savings", view.Customer, false)
	if !strings.Contains(bank, "<strong>Product total:</strong> -£") {
		t.Errorf("the bank's view shows the savings book positive:\n%s", bank)
	}
	if strings.Contains(customer, "<strong>Product total:</strong> -£") || !strings.Contains(customer, "<strong>Product total:</strong> £") {
		t.Errorf("the customer's view shows the savings book negative:\n%s", customer)
	}
}
