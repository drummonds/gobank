package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"strings"

	_ "git.bytestone.uk/hum3/go-postgres"
	customers "git.bytestone.uk/hum3/gobanks-customers"
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

// piiKeyProvider is the default key for PII encryption in the demo.
// WASM uses this hardcoded key; server mode could use EnvKeyProvider.
var piiKeyProvider customers.KeyProvider = customers.FixedKeyProvider{
	Key: []byte("gobank-demo-pii-key-32bytes!!!!!"),
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

// initDB opens an in-memory pglike database and creates the gilt tables.
func (ds *DemoState) initDB() {
	ds.initDBWithDSN("")
}

// initDBWithDSN opens a database connection. Empty dsn uses in-memory pglike;
// a postgres:// DSN uses pgx for real PostgreSQL.
func (ds *DemoState) initDBWithDSN(dsn string) {
	if ds.db != nil {
		ds.db.Close()
	}
	var db *sql.DB
	var err error
	if dsn == "" {
		db, err = sql.Open("pglike", "file::memory:?_pragma=temp_store(2)")
	} else {
		db, err = sql.Open("pgx", dsn)
	}
	if err != nil {
		log.Printf("initDB: open failed: %v", err)
		return
	}
	// sql.Open is lazy: verify a requested PostgreSQL backend is actually
	// reachable rather than silently failing on first use.
	if dsn != "" {
		if err := db.Ping(); err != nil {
			log.Fatalf("initDB: cannot reach %s: %v", describeBackend(dsn), err)
		}
		// The demo cannot resume from persisted rows — in-memory sim state
		// is authoritative and starts fresh, so leftover tables from a
		// previous run would only collide (duplicate customer IDs etc.).
		// Start every run with an empty database; export .goluca to keep a
		// run's ledger.
		dropAllPublicTables(db)
	}
	ds.db = db
	ds.dbBackend = describeBackend(dsn)
	ds.dbIsPostgres = dsn != ""
	ds.createGiltTables()
	ds.createAccrualTable()
	ds.createCustomerAccountsTable()
	ds.createPaymentsTable()

	// Create customer store (shares same DB)
	custStore, err := customers.NewSQLCustomerStore(db, piiKeyProvider)
	if err != nil {
		log.Printf("initDB: customer store: %v", err)
	} else {
		ds.custStore = custStore
	}
}

// DB returns the database handle.
func (ds *DemoState) DB() *sql.DB {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.db
}
