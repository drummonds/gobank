package main

import (
	"os"
	"strings"
	"testing"
)

// Both backends answer the explorer's PostgreSQL catalog queries, so under
// pglike it lists the demo's tables and contract views with no backend switch.
func TestExplorerListsTablesOnPglike(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	page := ds.BuildExplorerPage(t.Context(), "/internal/explorer")
	for _, want := range []string{"customer_accounts", "movements", "contract_payments", `href="/internal/explorer/c/customers">customers</a>`} {
		if !strings.Contains(page, want) {
			t.Errorf("explorer index on pglike missing %q", want)
		}
	}
}

// On a real PostgreSQL (GOBANK_PG_DSN) the explorer lists the demo's
// tables and contract views.
func TestExplorerListsTablesOnPostgres(t *testing.T) {
	dsn := os.Getenv("GOBANK_PG_DSN")
	if dsn == "" {
		t.Skip("GOBANK_PG_DSN not set")
	}
	ds := NewDemoStateWithDSN(dsn)
	addFundedCustomer(ds)
	page := ds.BuildExplorerPage(t.Context(), "/internal/explorer")
	for _, want := range []string{"customer_accounts", "movements", "contract_payments", `href="/internal/explorer/c/customers">customers</a>`} {
		if !strings.Contains(page, want) {
			t.Errorf("explorer index on PostgreSQL missing %q", want)
		}
	}
	if strings.Contains(page, "No tables found") {
		t.Error("explorer index on PostgreSQL reports no tables")
	}
}

// An explorer URL — as the explorer's own links emit it — renders with every
// query parameter honoured, including the filter a foreign-key link sets.
func TestExplorerPageHonoursFilter(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	addFundedCustomer(ds)
	page := ds.BuildExplorerPage(t.Context(), "/internal/explorer/customer_accounts?filter=customer_id&value=cust-002")
	if !strings.Contains(page, "Filter: customer_id = cust-002") {
		t.Errorf("filtered page does not show the filter: %.300s", page)
	}
	if strings.Contains(page, ">cust-001<") {
		t.Error("filtered page still lists cust-001's accounts")
	}
	if !strings.Contains(page, ">cust-002<") {
		t.Error("filtered page does not list cust-002's accounts")
	}
}

// The customers component holds PII, so browsing it needs view_pii; every
// other component, and unowned tables, are open to all roles.
func TestRoleCanViewComponent(t *testing.T) {
	cases := []struct {
		role      Role
		component string
		want      bool
	}{
		{RoleAdmin, "customers", true},
		{RoleAuditor, "customers", true},
		{RoleCustomerService, "customers", true},
		{RoleReadOnly, "customers", false},
		{RoleReadOnly, "payments", true},
		{RoleReadOnly, "ledger", true},
		{RoleReadOnly, "", true},
	}
	for _, c := range cases {
		if got := c.role.CanViewComponent(c.component); got != c.want {
			t.Errorf("%s.CanViewComponent(%q) = %v, want %v", c.role, c.component, got, c.want)
		}
	}
}

// The explorer applies the viewer's role: read-only never sees PII tables.
func TestExplorerHidesPIIFromReadOnly(t *testing.T) {
	ds := NewDemoState()
	readOnly := withRole(t.Context(), RoleReadOnly)
	if page := ds.BuildExplorerPage(readOnly, "/internal/explorer"); strings.Contains(page, ">cust_pii</a>") || !strings.Contains(page, ">payments</a>") {
		t.Error("read-only index should hide the customers component and keep the rest")
	}
	if page := ds.BuildExplorerPage(readOnly, "/internal/explorer/cust_pii"); !strings.Contains(page, "Access denied") {
		t.Error("read-only should be denied cust_pii")
	}
	if page := ds.BuildExplorerPage(withRole(t.Context(), RoleAuditor), "/internal/explorer"); !strings.Contains(page, ">cust_pii</a>") {
		t.Error("auditor should see the customers component")
	}
}
