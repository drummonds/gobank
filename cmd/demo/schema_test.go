package main

import (
	"database/sql"
	"testing"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pglike", "file::memory:?_pragma=temp_store(2)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func appliedVersions(t *testing.T, db *sql.DB, component string) []int {
	t.Helper()
	rows, err := db.Query(`SELECT version FROM schema_versions WHERE component = $1 ORDER BY version`, component)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var v int
		rows.Scan(&v)
		out = append(out, v)
	}
	return out
}

// A component's schema is a numbered list of migrations. Each is applied
// once, in order, and recorded in schema_versions; running again applies
// nothing; a version added later is the only one applied next time.
func TestMigrationsApplyOnceInOrder(t *testing.T) {
	db := openTestDB(t)
	widgets := componentSchema{component: "widgets", migrations: []migration{
		{1, []string{`CREATE TABLE widgets (id INTEGER PRIMARY KEY)`}},
		{2, []string{`ALTER TABLE widgets ADD COLUMN colour VARCHAR(20)`}},
	}}
	if err := migrate(db, []componentSchema{widgets}); err != nil {
		t.Fatal(err)
	}
	if got := appliedVersions(t, db, "widgets"); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("applied versions = %v; want [1 2]", got)
	}
	if _, err := db.Exec(`INSERT INTO widgets (id, colour) VALUES (1, 'red')`); err != nil {
		t.Fatalf("both migrations should have run: %v", err)
	}

	// Again: nothing to do, and the CREATE TABLE must not run a second time.
	if err := migrate(db, []componentSchema{widgets}); err != nil {
		t.Fatalf("re-running migrations should be a no-op, got %v", err)
	}

	// A new version applies on its own.
	widgets.migrations = append(widgets.migrations, migration{3, []string{`ALTER TABLE widgets ADD COLUMN size INTEGER`}})
	if err := migrate(db, []componentSchema{widgets}); err != nil {
		t.Fatal(err)
	}
	if got := appliedVersions(t, db, "widgets"); len(got) != 3 {
		t.Errorf("applied versions = %v; want [1 2 3]", got)
	}
	if _, err := db.Exec(`UPDATE widgets SET size = 3 WHERE id = 1`); err != nil {
		t.Errorf("version 3 should have run: %v", err)
	}
}

// Versions must be declared in ascending order with no gaps: a mistake in
// the list is refused before anything runs.
func TestMigrationsRefuseUnorderedVersions(t *testing.T) {
	db := openTestDB(t)
	bad := componentSchema{component: "widgets", migrations: []migration{
		{1, []string{`CREATE TABLE widgets (id INTEGER PRIMARY KEY)`}},
		{3, []string{`ALTER TABLE widgets ADD COLUMN colour VARCHAR(20)`}},
	}}
	if err := migrate(db, []componentSchema{bad}); err == nil {
		t.Fatal("a gap in the version list should be refused")
	}
	if _, err := db.Exec(`SELECT 1 FROM widgets`); err == nil {
		t.Error("nothing should have been applied")
	}
}

// A schema belongs to a registered component and creates only that
// component's own tables (ADR-0001 extended to migrations, ADR-0003).
func TestSchemasCreateOnlyTheirOwnTables(t *testing.T) {
	owner := map[string]string{}
	registered := map[string]bool{}
	for _, c := range components {
		registered[c.Name] = true
		for _, tbl := range c.Tables {
			owner[tbl] = c.Name
		}
	}
	for _, s := range demoSchemas() {
		if !registered[s.component] {
			t.Errorf("schema %s: no such component in the registry", s.component)
		}
		for _, m := range s.migrations {
			for _, stmt := range m.stmts {
				for _, tbl := range sqlTables(stmt, owner) {
					if owner[tbl] != s.component {
						t.Errorf("schema %s v%d touches %s, a table of %s", s.component, m.version, tbl, owner[tbl])
					}
				}
			}
		}
	}
}

// A database from before schema versions (every table present, nothing
// recorded) is recognised so the demo can start it fresh rather than
// collide with its rows.
func TestLegacyDatabaseIsRecognised(t *testing.T) {
	db := openTestDB(t)
	if isLegacyDatabase(db) {
		t.Error("an empty database is not legacy")
	}
	db.Exec(`CREATE TABLE accounts (id TEXT PRIMARY KEY)`)
	if !isLegacyDatabase(db) {
		t.Error("ledger tables without schema_versions is a legacy database")
	}
	if err := migrate(db, demoSchemas()); err != nil {
		t.Fatal(err)
	}
	if isLegacyDatabase(db) {
		t.Error("once versioned, no longer legacy")
	}
}
