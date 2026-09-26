package main

import (
	"os"
	"strings"
	"testing"
)

// The explorer reads the catalog of whichever backend the demo runs on:
// sqlite_master under pglike, pg_tables on PostgreSQL. Without the flag it
// asks PostgreSQL for sqlite_master and lists nothing.
func TestExplorerFollowsBackend(t *testing.T) {
	ds := NewDemoState()
	if ds.explorer().Postgres {
		t.Error("explorer set to PostgreSQL catalog on pglike")
	}
	pg := &DemoState{dbIsPostgres: true}
	if !pg.explorer().Postgres {
		t.Error("explorer not set to PostgreSQL catalog on a PostgreSQL backend")
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
