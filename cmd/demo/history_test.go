package main

import (
	"context"
	"testing"

	"git.bytestone.uk/hum3/gobank/core"
)

// series is the daily series as the bank's record gives them.
func series(t *testing.T, ds *DemoState) core.History {
	t.Helper()
	h, err := ds.history.Series(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// The daily series behind the dashboard charts are stored snapshots, one
// row a day (ADR-0002 stage 2, story e): a restart over the same database
// shows the same charts, and the NIM the position reports is the latest
// snapshot's.
func TestRestartKeepsTheDailySeries(t *testing.T) {
	first := NewDemoState()
	first.createCustomer()
	for range 10 {
		first.advanceDay()
	}
	before := series(t, first)
	if n := len(before.Balances); n != 11 { // the opening day and ten more
		t.Fatalf("balance points before restart = %d, want 11", n)
	}
	if len(before.Customers) != 11 || len(before.NIM) != 11 || len(before.BoERate) != 11 {
		t.Fatalf("series lengths before restart: customers %d, NIM %d, BoE %d; want 11 each",
			len(before.Customers), len(before.NIM), len(before.BoERate))
	}

	second := newDemoStateOn(first.db, "")
	after := series(t, second)
	if len(after.Balances) != len(before.Balances) {
		t.Fatalf("balance points after restart = %d, want %d", len(after.Balances), len(before.Balances))
	}
	for i := range before.Balances {
		if after.Balances[i] != before.Balances[i] {
			t.Errorf("balance point %d = %+v after restart, want %+v", i, after.Balances[i], before.Balances[i])
		}
		if after.Customers[i] != before.Customers[i] {
			t.Errorf("customer point %d = %+v after restart, want %+v", i, after.Customers[i], before.Customers[i])
		}
		if after.NIM[i] != before.NIM[i] {
			t.Errorf("NIM point %d = %+v after restart, want %+v", i, after.NIM[i], before.NIM[i])
		}
		if after.BoERate[i] != before.BoERate[i] {
			t.Errorf("BoE point %d = %+v after restart, want %+v", i, after.BoERate[i], before.BoERate[i])
		}
	}
	if got, want := second.position().NIMBps, before.NIM[len(before.NIM)-1].NIM; got != want {
		t.Errorf("position NIM after restart = %v, want the latest snapshot's %v", got, want)
	}

	// The series carry on from where they were.
	second.advanceDay()
	if n := len(series(t, second).Balances); n != 12 {
		t.Errorf("balance points after one more day = %d, want 12", n)
	}
}

// A reset starts the series again from the new opening day.
func TestResetClearsTheDailySeries(t *testing.T) {
	ds := NewDemoState()
	for range 5 {
		ds.advanceDay()
	}
	ds.Reset()
	h := series(t, ds)
	if len(h.Balances) != 1 || len(h.BoERate) != 1 {
		t.Fatalf("series after reset: %d balance points, %d BoE points; want 1 each", len(h.Balances), len(h.BoERate))
	}
	if !h.Balances[0].Date.Equal(ds.position().Day) {
		t.Errorf("first point after reset is %s, want the opening day %s", h.Balances[0].Date, ds.position().Day)
	}
}
