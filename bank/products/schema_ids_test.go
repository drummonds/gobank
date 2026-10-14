package products

import (
	"database/sql"
	"regexp"
	"testing"

	"git.bytestone.uk/hum3/gobank/bank/schema"
)

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

func columnType(t *testing.T, db *sql.DB, table, column string) string {
	t.Helper()
	var udt string
	if err := db.QueryRow(`SELECT udt_name FROM information_schema.columns WHERE table_name = $1 AND column_name = $2`,
		table, column).Scan(&udt); err != nil {
		t.Fatalf("%s.%s: %v", table, column, err)
	}
	return udt
}

// The ledger ids the component records (a posting's movement and
// account, the account whose parameters are applied) are UUID columns,
// as the ledger's own are (#66); a record from before the retype is
// migrated with its rows.
func TestLedgerIDsAreUUID(t *testing.T) {
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	applyVersions(t, db, Schema, 0, 5)
	movement, account := causeID("movement-1"), causeID("account-1")
	if _, err := db.Exec(`INSERT INTO product_postings (movement_id, account_id, product_id, rules, rules_version, event, parameters, input_balance, input_accrued_num, posted_at)
		VALUES ($1, $2, 'easy-access', 'savings', 1, 'accrue', '', 0, 0, '2026-01-02')`, movement, account); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO product_account_parameters (account_id, applied_to) VALUES ($1, '2026-01-02')`, account); err != nil {
		t.Fatal(err)
	}
	applyVersions(t, db, Schema, 5, len(Schema.Migrations))
	for _, col := range [][2]string{
		{"product_postings", "movement_id"}, {"product_postings", "account_id"}, {"product_account_parameters", "account_id"},
	} {
		if got := columnType(t, db, col[0], col[1]); got != "uuid" {
			t.Errorf("%s.%s is %s, want uuid", col[0], col[1], got)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM product_postings p JOIN product_account_parameters ap ON ap.account_id = p.account_id
		WHERE p.movement_id = $1`, movement).Scan(&n); err != nil || n != 1 {
		t.Errorf("rows after the migration: %d, %v; want 1", n, err)
	}
}

// The first cursor of a pass over accounts in id order sorts before every
// UUID and is one, since on PostgreSQL a uuid column cannot be compared
// to an empty string.
func TestFirstCursorIsAUUIDBeforeAllOthers(t *testing.T) {
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`).MatchString(firstCursor) {
		t.Fatalf("firstCursor %q is not a UUID", firstCursor)
	}
	if id := causeID("any"); firstCursor >= id {
		t.Errorf("firstCursor %q does not sort before %q", firstCursor, id)
	}
}
