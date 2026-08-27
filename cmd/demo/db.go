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

	// Create customer store (shares same DB)
	custStore, err := customers.NewSQLCustomerStore(db, piiKeyProvider)
	if err != nil {
		log.Printf("initDB: customer store: %v", err)
	} else {
		ds.custStore = custStore
	}
}

// createGiltTables creates gilt_yields and gilt_holdings tables and seeds yields.
func (ds *DemoState) createGiltTables() {
	db := ds.db

	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS gilt_yields (
		tenor VARCHAR(10) PRIMARY KEY,
		rate REAL NOT NULL,
		effective_date TIMESTAMP DEFAULT NOW()
	)`)
	if err != nil {
		log.Printf("initDB: create gilt_yields: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS gilt_holdings (
		id SERIAL PRIMARY KEY,
		tenor VARCHAR(10) NOT NULL,
		face_value BIGINT NOT NULL, -- minor units (pence)
		purchase_date TIMESTAMP NOT NULL,
		yield REAL NOT NULL
	)`)
	if err != nil {
		log.Printf("initDB: create gilt_holdings: %v", err)
	}

	// Seed gilt yields
	yields := []struct {
		tenor string
		rate  float64
	}{
		{"1Y", 0.0435},
		{"2Y", 0.0410},
		{"5Y", 0.0395},
		{"10Y", 0.0405},
		{"30Y", 0.0445},
	}
	for _, y := range yields {
		_, err = db.Exec(`INSERT INTO gilt_yields (tenor, rate) VALUES ($1, $2)
			ON CONFLICT (tenor) DO UPDATE SET rate = EXCLUDED.rate`, y.tenor, y.rate)
		if err != nil {
			log.Printf("initDB: seed %s: %v", y.tenor, err)
		}
	}
}

// boeAccrualKey is the accrual_state row holding the bank-level BoE interest
// numerator. Ledger account IDs are UUIDs, so it can never collide.
const boeAccrualKey = "_boe"

// createAccrualTable creates accrual_state, which persists accrued-but-unapplied
// interest as exact integer numerators (minor units = numerator /
// gbp.AccrualDenominator). The ledger only sees interest at month-end
// application, so without this table the DB is missing up to a month of
// accrual per account plus the BoE accumulator.
func (ds *DemoState) createAccrualTable() {
	_, err := ds.db.Exec(`CREATE TABLE IF NOT EXISTS accrual_state (
		account_id VARCHAR(64) PRIMARY KEY,
		numerator BIGINT NOT NULL,
		accrued_pounds_e7 BIGINT NOT NULL DEFAULT 0, -- accrual as 7dp pounds (rounded view; numerator is canonical)
		as_of TIMESTAMP NOT NULL
	)`)
	if err != nil {
		log.Printf("initDB: create accrual_state: %v", err)
	}
	// Migrate tables created before the 7dp pounds column existed (durable
	// postgres DBs); errors on an already-migrated table are expected.
	ds.db.Exec(`ALTER TABLE accrual_state ADD COLUMN accrued_pounds_e7 BIGINT NOT NULL DEFAULT 0`)
}

// DB returns the database handle.
func (ds *DemoState) DB() *sql.DB {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.db
}
