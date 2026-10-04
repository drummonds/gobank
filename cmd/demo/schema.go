package main

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// Schema versions. Every component that owns tables declares its schema as
// a numbered list of migrations (ADR-0001 gives each table one owner;
// ADR-0003 extends that to the table's migrations). schema_versions records
// which versions each component's tables are at, so a start applies only
// what is new: an upgrade migrates the database it finds rather than
// rebuilding it, and a migration written expand/contract leaves the
// previous release able to run on the same database.

// migration is one step of a component's schema: the statements that take
// it from version-1 to version, applied in one transaction.
type migration struct {
	version int
	stmts   []string
}

// componentSchema is a component's migrations in version order, 1, 2, 3…
type componentSchema struct {
	component  string
	migrations []migration
}

// demoSchemas is every component's schema. Libraries (the ledger, the
// customer store) create their own tables and are not listed.
func demoSchemas() []componentSchema {
	return []componentSchema{simulationSchema, productsSchema, customersSchema, paymentsSchema, treasurySchema, historySchema}
}

// migrate brings every component's tables up to its latest version,
// recording each version applied. The version lists are checked first: a
// gap or a repeat is a mistake in the code and nothing is applied.
func migrate(db *sql.DB, schemas []componentSchema) error {
	for _, s := range schemas {
		for i, m := range s.migrations {
			if m.version != i+1 {
				return fmt.Errorf("schema %s: migration %d listed where version %d is due", s.component, m.version, i+1)
			}
		}
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_versions (
		component VARCHAR(50) NOT NULL,
		version INTEGER NOT NULL,
		applied_at TIMESTAMP NOT NULL,
		PRIMARY KEY (component, version)
	)`); err != nil {
		return fmt.Errorf("schema_versions: %w", err)
	}
	for _, s := range schemas {
		var current int
		if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_versions WHERE component = $1`, s.component).Scan(&current); err != nil {
			return fmt.Errorf("schema %s: current version: %w", s.component, err)
		}
		for _, m := range s.migrations {
			if m.version <= current {
				continue
			}
			if err := apply(db, s.component, m); err != nil {
				return err
			}
			log.Printf("schema: %s at version %d", s.component, m.version)
		}
	}
	return nil
}

// apply runs one migration and records it, in one transaction.
func apply(db *sql.DB, component string, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("schema %s v%d: begin: %w", component, m.version, err)
	}
	for _, stmt := range m.stmts {
		if _, err := tx.Exec(stmt); err != nil {
			tx.Rollback()
			return fmt.Errorf("schema %s v%d: %w", component, m.version, err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_versions (component, version, applied_at) VALUES ($1, $2, $3)`,
		component, m.version, time.Now().UTC()); err != nil {
		tx.Rollback()
		return fmt.Errorf("schema %s v%d: record: %w", component, m.version, err)
	}
	return tx.Commit()
}

// isLegacyDatabase reports a database from before schema versions: a
// ledger in it but no record of what version anything is at. Such a
// database was written by a demo that dropped every table at start, so
// its rows were never meant to outlive their process.
func isLegacyDatabase(db *sql.DB) bool {
	return tableExists(db, "accounts") && !tableExists(db, "schema_versions")
}

// tableExists probes for a table by selecting nothing from it.
func tableExists(db *sql.DB, name string) bool {
	_, err := db.Exec(fmt.Sprintf(`SELECT 1 FROM %s WHERE 1 = 0`, name))
	return err == nil
}

// appliedSchemaVersions is the version each component's tables are at,
// as the about endpoint reports it.
func appliedSchemaVersions(db *sql.DB) []SchemaVersion {
	if db == nil {
		return nil
	}
	rows, err := db.Query(`SELECT component, MAX(version) FROM schema_versions GROUP BY component ORDER BY component`)
	if err != nil {
		log.Printf("appliedSchemaVersions: %v", err)
		return nil
	}
	defer rows.Close()
	var out []SchemaVersion
	for rows.Next() {
		var s SchemaVersion
		if err := rows.Scan(&s.Component, &s.Version); err == nil {
			out = append(out, s)
		}
	}
	return out
}
