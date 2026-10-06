package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"strings"

	_ "git.bytestone.uk/hum3/go-postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// describeBackend returns a display string for the data store backing the
// given DSN, with any password redacted.
func describeBackend(dsn string) string {
	if dsn == "" {
		return "In-memory (pglike/SQLite)"
	}
	if u, err := url.Parse(dsn); err == nil && u.Host != "" {
		return fmt.Sprintf("PostgreSQL (pgx) — %s/%s", u.Host, strings.TrimPrefix(u.Path, "/"))
	}
	return "PostgreSQL (pgx)"
}

// dropAllPublicTables removes all tables from a postgres database.
func dropAllPublicTables(db *sql.DB) {
	rows, err := db.Query("SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
	if err != nil {
		log.Printf("dropAllPublicTables: %v", err)
		return
	}
	var tables []string
	for rows.Next() {
		var t string
		rows.Scan(&t)
		tables = append(tables, t)
	}
	rows.Close()
	for _, t := range tables {
		db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %q CASCADE", t))
	}
}

// dbConfigRows returns label/value pairs describing the live database
// configuration (version, size on disk, key tuning, connections) for the
// runtime page. PostgreSQL only — pglike is in-memory and shows up in the
// heap stats instead. Call without ds.mu held: these are live queries.
func (ds *DemoState) dbConfigRows() [][2]string {
	if !ds.dbIsPostgres || ds.db == nil {
		return nil
	}
	var rows [][2]string
	var v string
	if err := ds.db.QueryRow(`SELECT current_setting('server_version')`).Scan(&v); err == nil {
		rows = append(rows, [2]string{"PostgreSQL version", v})
	}
	if err := ds.db.QueryRow(`SELECT pg_size_pretty(pg_database_size(current_database()))`).Scan(&v); err == nil {
		rows = append(rows, [2]string{"Size on disk", v})
	}
	for _, setting := range []string{"shared_buffers", "effective_cache_size", "work_mem", "max_connections", "max_parallel_workers"} {
		if err := ds.db.QueryRow(`SELECT current_setting($1)`, setting).Scan(&v); err == nil {
			rows = append(rows, [2]string{setting, v})
		}
	}
	var n int
	if err := ds.db.QueryRow(`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()`).Scan(&n); err == nil {
		rows = append(rows, [2]string{"Server connections", fmt.Sprintf("%d", n)})
	}
	return rows
}

// initDB opens a fresh in-memory pglike database.
func (ds *DemoState) initDB() {
	ds.initDBWithDSN("")
}

// initDBWithDSN opens a database and takes it as the demo's. Empty dsn
// uses in-memory pglike; a postgres:// DSN uses pgx for real PostgreSQL.
func (ds *DemoState) initDBWithDSN(dsn string) {
	if ds.db != nil {
		ds.db.Close()
	}
	ds.attachDB(openDB(dsn), dsn)
}

// openDB opens the database behind dsn. A requested PostgreSQL backend is
// verified reachable, since sql.Open is lazy and would otherwise fail
// silently on first use.
func openDB(dsn string) *sql.DB {
	var db *sql.DB
	var err error
	if dsn == "" {
		db, err = sql.Open("pglike", "file::memory:?_pragma=temp_store(2)")
	} else {
		db, err = sql.Open("pgx", dsn)
	}
	if err != nil {
		log.Printf("initDB: open failed: %v", err)
		return nil
	}
	if dsn != "" {
		if err := db.Ping(); err != nil {
			log.Fatalf("initDB: cannot reach %s: %v", describeBackend(dsn), err)
		}
	}
	return db
}

// attachDB takes db as the demo's database. Its tables are kept: every
// component's schema is migrated to the current version, and whatever run
// the rows describe is there for the bank to resume (ADR-0003). The one
// exception is a database from before schema versions, written by a demo
// that dropped every table at start: it is started fresh, as that demo
// would have. The contract views are the components' to refresh when the
// bank opens.
func (ds *DemoState) attachDB(db *sql.DB, dsn string) {
	ds.db = db
	ds.dsn = dsn
	ds.dbBackend = describeBackend(dsn)
	ds.dbIsPostgres = dsn != ""
	if db == nil {
		log.Fatalf("initDB: no database")
	}
	if ds.dbIsPostgres && isLegacyDatabase(db) {
		log.Printf("initDB: database predates schema versions; starting it fresh")
		dropAllPublicTables(db)
	}
	if err := migrate(db, demoSchemas()); err != nil {
		log.Fatalf("initDB: %v", err)
	}
}

// DB returns the database handle.
func (ds *DemoState) DB() *sql.DB {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.db
}
