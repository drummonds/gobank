package main

import (
	"git.bytestone.uk/hum3/gobank/bff/staff"
	"strings"
	"testing"
)

// The PII rule: a customer's name is shown only with PII authorisation.
// The detail and account pages fall back to the ID, as every other page
// does (#23).
func TestCustomerPagesHideNameWithoutPII(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	q := ds.Bank
	name := ds.lookupName("cust-001")
	if name == "" {
		t.Fatal("the funded customer has no name")
	}
	for _, page := range []struct {
		what   string
		render func(pii bool) string
	}{
		{"detail", func(pii bool) string { return staff.BuildCustomerDetailHTML(q, "cust-001", pii, 1) }},
		{"account", func(pii bool) string { return staff.BuildCustomerAccountHTML(q, "cust-001", 0, pii, 1) }},
	} {
		if h := page.render(false); strings.Contains(h, name) {
			t.Errorf("%s page shows %q without PII authorisation", page.what, name)
		}
		if h := page.render(true); !strings.Contains(h, name) {
			t.Errorf("%s page hides %q from an authorised user", page.what, name)
		}
	}
}
