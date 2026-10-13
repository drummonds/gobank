package customers

import (
	"crypto/sha1"
	"database/sql"
	"fmt"
	"testing"

	"git.bytestone.uk/hum3/gobank/bank/schema"
)

// testID is a UUID made from a name, so the test reads as the name.
func testID(name string) string {
	h := sha1.Sum([]byte(name))
	h[6], h[8] = h[6]&0x0f|0x50, h[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

// applyVersions runs a component's migrations after version from, up to
// and including version to.
func applyVersions(t *testing.T, db *sql.DB, c schema.Component, from, to int) {
	t.Helper()
	for _, m := range c.Migrations {
		if m.Version <= from || m.Version > to {
			continue
		}
		for _, stmt := range m.Statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("%s v%d: %v", c.Name, m.Version, err)
			}
		}
	}
}

// columnType reads a column's type from the catalog.
func columnType(t *testing.T, db *sql.DB, table, column string) string {
	t.Helper()
	var udt string
	if err := db.QueryRow(`SELECT udt_name FROM information_schema.columns WHERE table_name = $1 AND column_name = $2`,
		table, column).Scan(&udt); err != nil {
		t.Fatalf("%s.%s: %v", table, column, err)
	}
	return udt
}

// The ledger account id is a UUID column, as the ledger's own id is
// (#66): a register from before the retype is migrated with its rows.
func TestLedgerAccountIDIsUUID(t *testing.T) {
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	applyVersions(t, db, Schema, 0, 1)
	if got := columnType(t, db, "customer_accounts", "ledger_account_id"); got != "varchar" {
		t.Fatalf("precondition: v1 ledger_account_id is %s, want varchar", got)
	}
	id := testID("account-1")
	if _, err := db.Exec(`INSERT INTO customer_accounts (customer_id, idx, ledger_account_id, product_id, sort_code, account_num, opened)
		VALUES ('cust-000001', 0, $1, 'easy-access', '04-00-04', '00000001', '2026-01-02')`, id); err != nil {
		t.Fatal(err)
	}
	applyVersions(t, db, Schema, 1, len(Schema.Migrations))
	if got := columnType(t, db, "customer_accounts", "ledger_account_id"); got != "uuid" {
		t.Errorf("ledger_account_id is %s, want uuid", got)
	}
	var back string
	if err := db.QueryRow(`SELECT ledger_account_id FROM customer_accounts WHERE customer_id = 'cust-000001'`).Scan(&back); err != nil || back != id {
		t.Errorf("row after the migration: %q, %v; want %s", back, err, id)
	}
	if _, err := db.Exec(`INSERT INTO customer_accounts (customer_id, idx, ledger_account_id, product_id, sort_code, account_num, opened)
		VALUES ('cust-000002', 0, 'not-an-id', 'easy-access', '04-00-04', '00000002', '2026-01-02')`); err == nil {
		t.Error("a ledger account id that is not a UUID was accepted")
	}
}
