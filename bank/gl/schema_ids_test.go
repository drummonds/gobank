package gl

import (
	"crypto/sha1"
	"database/sql"
	"fmt"
	"testing"
)

// testID is a UUID made from a name, so the test reads as the name.
func testID(name string) string {
	h := sha1.Sum([]byte(name))
	h[6], h[8] = h[6]&0x0f|0x50, h[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

// A journal line's movement id is a UUID column, as the ledger's own id
// is (#66); a journal from before the retype is migrated with its rows.
func TestJournalMovementIDIsUUID(t *testing.T) {
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range Schema.Migrations[0].Statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("v1: %v", err)
		}
	}
	movement := testID("movement-1")
	if _, err := db.Exec(`INSERT INTO gl_journal (day, product_id, code, amount, movement_id) VALUES ('2026-01-02', 'easy-access', 'INT', 1, $1)`, movement); err != nil {
		t.Fatal(err)
	}
	for _, m := range Schema.Migrations[1:] {
		for _, stmt := range m.Statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("v%d: %v", m.Version, err)
			}
		}
	}
	var udt, back string
	if err := db.QueryRow(`SELECT udt_name FROM information_schema.columns WHERE table_name = 'gl_journal' AND column_name = 'movement_id'`).Scan(&udt); err != nil || udt != "uuid" {
		t.Errorf("movement_id is %q, %v; want uuid", udt, err)
	}
	if err := db.QueryRow(`SELECT movement_id FROM gl_journal`).Scan(&back); err != nil || back != movement {
		t.Errorf("row after the migration: %q, %v; want %s", back, err, movement)
	}
}
