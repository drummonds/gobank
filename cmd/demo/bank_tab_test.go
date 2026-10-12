package main

import (
	"net/http"
	"net/http/httptest"
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

// While the simulation runs the layout re-fetches most pages whole every
// second; a master-detail page must not be, or the detail a row loaded is
// wiped by the next poll. The master polls itself instead.
func TestMasterDetailPagesAreNotRefetchedWhole(t *testing.T) {
	t.Setenv("GOBANK_ADMIN_PASSWORD", testAdminPassword)
	ds := NewDemoState()
	addFundedCustomer(ds)
	site := newSite(ds)
	cookie := staffLogin(t, site)
	ds.Start()
	defer ds.Stop()
	for _, path := range []string{"/products/bank", "/products/savings", "/products/lending"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(cookie)
		site.ServeHTTP(rr, req)
		body := rr.Body.String()
		if rr.Code != http.StatusOK || !strings.Contains(body, `>Running<`) {
			t.Fatalf("%s: %d, or the console is not running:\n%.300s", path, rr.Code, body)
		}
		if strings.Contains(body, `hx-trigger="every 1s"`) {
			t.Errorf("%s is re-fetched whole by the layout while running", path)
		}
		if !strings.Contains(body, `hx-trigger="every 2s"`) || !strings.Contains(body, `hx-select="#products-master"`) {
			t.Errorf("%s: the master does not poll itself while running", path)
		}
	}
}
