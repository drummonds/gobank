package main

import (
	"database/sql"
	"os"
	"testing"
)

// On PostgreSQL the next process is a new connection pool, the schema
// is the real one (SERIAL, TIMESTAMP through pgx) and a reset drops real
// tables. Runs when GOBANK_PG_DSN points at a scratch database, which it
// wipes.
func TestRestartResumesOnPostgres(t *testing.T) {
	dsn := os.Getenv("GOBANK_PG_DSN")
	if dsn == "" {
		t.Skip("set GOBANK_PG_DSN to a scratch PostgreSQL database")
	}
	scratch, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer scratch.Close()
	dropAllPublicTables(scratch)

	// A database from before schema versions: a ledger, no record of
	// versions. It is started fresh rather than resumed into.
	if _, err := scratch.Exec(`CREATE TABLE accounts (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	legacy := NewDemoStateWithDSN(dsn)
	if run, ok := loadRun(legacy.db); !ok || run.DayCount != 0 {
		t.Fatalf("a legacy database starts fresh: %+v, %v", run, ok)
	}
	for range 3 {
		legacy.createCustomer()
	}
	for range 35 {
		legacy.advanceDay()
	}
	legacy.Start()
	before := legacy.position()
	beforeAccrual := accrualNumerators(legacy)
	if err := legacy.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	legacy.db.Close()

	// The next process.
	second := NewDemoStateWithDSN(dsn)
	defer second.db.Close()
	if !second.ResumedRunning() {
		t.Error("the run was going at shutdown; the next process should carry it on")
	}
	after := second.position()
	if !after.Day.Equal(before.Day) || after.Customers != before.Customers || after.Savings != before.Savings || after.Lending != before.Lending {
		t.Errorf("resumed position %+v; want %+v", after, before)
	}
	got := accrualNumerators(second)
	for id, n := range beforeAccrual {
		if got[id] != n {
			t.Errorf("account %s accrued numerator = %d after restart; want %d", id, got[id], n)
		}
	}
	second.createCustomer()
	second.advanceDay()
	if p := second.position(); p.DayCount != before.DayCount+1 || p.Customers < before.Customers+1 {
		t.Errorf("after one more customer and day: %+v", p)
	}

	// Reset on PostgreSQL is a fresh run on the same database.
	second.Reset()
	if p := second.position(); p.DayCount != 0 || p.Customers != 0 {
		t.Errorf("after reset: %+v", p)
	}
	second.createCustomer()
	if n := second.customerCount(); n != 1 {
		t.Errorf("customers after reset and one more = %d; want 1", n)
	}
	if !second.dbIsPostgres {
		t.Error("reset must stay on PostgreSQL")
	}
	third := NewDemoStateWithDSN(dsn)
	defer third.db.Close()
	if p := third.position(); p.DayCount != 0 || p.Customers != 1 {
		t.Errorf("resumed after reset: %+v; want day 0 with the one customer", p)
	}
}
