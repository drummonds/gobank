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
	page := ds.BuildExplorerHTML()
	for _, want := range []string{"customer_accounts", "movements", "contract_payments", `<span class="tag is-light">customers</span>`} {
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
	page := ds.BuildExplorerHTML()
	for _, want := range []string{"customer_accounts", "movements", "contract_payments", `<span class="tag is-light">customers</span>`} {
		if !strings.Contains(page, want) {
			t.Errorf("explorer index on PostgreSQL missing %q", want)
		}
	}
	if strings.Contains(page, "No tables found") {
		t.Error("explorer index on PostgreSQL reports no tables")
	}
}
