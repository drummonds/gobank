package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/bff/staff"
)

// The data's budget is PostgreSQL's: pglike holds the book in memory, so
// about.json says nothing of table sizes there rather than something wrong.
func TestAboutJSONOmitsTableSizesOnPglike(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	got := staff.AboutStatusOf(ds.Bank, ds, version)
	if got.Tables != nil || got.DatabaseBytes != 0 {
		t.Errorf("pglike: tables %+v, database %d bytes; want none", got.Tables, got.DatabaseBytes)
	}
	b, _ := json.Marshal(got)
	if strings.Contains(string(b), `"tables"`) || strings.Contains(string(b), `"database_bytes"`) {
		t.Errorf("pglike json carries sizes:\n%s", b)
	}
}

// On PostgreSQL a performance run reads which table is biggest and
// roughly what each takes: every table, biggest first, its size with
// indexes and TOAST, its estimated rows, and the database's whole size.
func TestAboutJSONReportsTableSizesOnPostgres(t *testing.T) {
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
	ds := NewDemoStateWithDSN(dsn)
	for range 3 {
		addFundedCustomer(ds)
	}
	for range 5 {
		ds.advanceDay()
	}
	if _, err := scratch.Exec(`ANALYZE`); err != nil {
		t.Fatal(err)
	}

	got := staff.AboutStatusOf(ds.Bank, ds, version)
	if len(got.Tables) == 0 {
		t.Fatal("no tables")
	}
	var sum int64
	var movements *staff.AboutTable
	for i, tb := range got.Tables {
		sum += tb.Bytes
		if i > 0 && tb.Bytes > got.Tables[i-1].Bytes {
			t.Errorf("not biggest first: %s (%d) after %s (%d)", tb.Name, tb.Bytes, got.Tables[i-1].Name, got.Tables[i-1].Bytes)
		}
		if tb.Name == "movements" {
			movements = &got.Tables[i]
		}
	}
	if movements == nil || movements.Bytes == 0 || movements.Rows == 0 {
		t.Errorf("movements = %+v; want its size and rows", movements)
	}
	if got.DatabaseBytes < sum {
		t.Errorf("database %d bytes, less than its tables' %d", got.DatabaseBytes, sum)
	}
	b, _ := json.Marshal(got)
	for _, want := range []string{`"tables":[{"name":`, `"rows":`, `"bytes":`, `"database_bytes":`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("json missing %s", want)
		}
	}
}
