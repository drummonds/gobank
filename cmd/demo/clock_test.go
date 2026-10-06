package main

import (
	"context"
	"testing"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
)

// ADR-0002 stage 4, story (b): the bank takes the time from an injected
// clock. Every banking fact is stamped by it: a payment's creation and
// settlement, a customer's join date, a transaction's date.
func TestBankingFactsAreStampedByTheClock(t *testing.T) {
	ds := NewDemoState()
	defer ds.db.Close()
	at := time.Date(2020, 1, 1, 10, 30, 0, 0, time.UTC)
	ds.clock = core.ClockFunc(func() time.Time { return at })
	bank := newCoreAdapter(ds, "")
	ctx := context.Background()

	twoFundedCustomers(ds)
	for _, id := range []string{"cust-001", "cust-002"} {
		rec, err := bank.CustomerRecord(ctx, id)
		if err != nil || !rec.JoinDate.Equal(core.BusinessDay(at)) {
			t.Errorf("%s joined %v, want the clock's day %v (%v)", id, rec.JoinDate, core.BusinessDay(at), err)
		}
		for _, p := range ds.paymentsOf(id) {
			if !p.CreatedAt.Equal(at) || !p.SettledAt.Equal(at) {
				t.Errorf("%s funding payment %s created %v settled %v, want the clock's %v", id, p.Reference, p.CreatedAt, p.SettledAt, at)
			}
		}
	}

	at = at.Add(3 * time.Hour)
	p, err := bank.Transfer(ctx, core.Transfer{From: "cust-001", To: "cust-002", Amount: 5_00})
	if err != nil {
		t.Fatal(err)
	}
	if !p.CreatedAt.Equal(at) {
		t.Errorf("transfer created %v, want the clock's %v", p.CreatedAt, at)
	}
	at = at.Add(20 * time.Minute) // the clock moves on while the payment settles
	deadline := time.Now().Add(5 * time.Second)
	for {
		settled, _ := ds.paymentByID(p.ID)
		if settled.Status == PaymentCompleted {
			if !settled.SettledAt.Equal(at) {
				t.Errorf("transfer settled %v, want the clock's %v", settled.SettledAt, at)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("transfer never settled")
		}
		time.Sleep(50 * time.Millisecond)
	}
	page, _ := bank.Transactions(ctx, "cust-001", 1)
	if len(page.Entries) == 0 || page.Entries[0].Date != "2020-01-01" {
		t.Errorf("newest transaction %+v, want dated on the clock's day", page.Entries)
	}
}

// StartDay follows the clock: with the clock still on the bank's day there
// is nothing to start; a clock two days on is caught up one day at a time
// in the one call, each day with its snapshot; and a day already started
// is not started again.
func TestStartDayCatchesUpWithTheClock(t *testing.T) {
	ds := NewDemoState()
	defer ds.db.Close()
	at := time.Date(2020, 1, 1, 9, 0, 0, 0, time.UTC)
	ds.clock = core.ClockFunc(func() time.Time { return at })
	bank := newCoreAdapter(ds, "")
	ctx := context.Background()
	addFundedCustomer(ds)

	day, err := bank.StartDay(ctx)
	if err != nil || !day.Equal(core.BusinessDay(at)) {
		t.Fatalf("StartDay with the clock on the bank's day = %v, %v; want %v unchanged", day, err, core.BusinessDay(at))
	}
	if pos := ds.position(); pos.DayCount != 0 {
		t.Errorf("day count %d after a StartDay with nothing to start, want 0", pos.DayCount)
	}

	at = at.AddDate(0, 0, 2)
	day, err = bank.StartDay(ctx)
	if err != nil || !day.Equal(core.BusinessDay(at)) {
		t.Fatalf("StartDay with the clock two days on = %v, %v; want %v", day, err, core.BusinessDay(at))
	}
	pos := ds.position()
	if !pos.Day.Equal(day) || pos.DayCount != 2 {
		t.Errorf("position %+v after catching up, want day %v count 2", pos, day)
	}
	hist, _ := bank.History(ctx)
	if len(hist.Balances) != 3 {
		t.Errorf("%d daily points after catching up two days, want 3 (opening day and each day started)", len(hist.Balances))
	}
	if pending, _ := anyUnprojected(ds.db, day); pending {
		t.Error("the caught-up day's pass is not complete")
	}

	if again, _ := bank.StartDay(ctx); !again.Equal(day) || ds.position().DayCount != 2 {
		t.Errorf("a second StartDay on the same clock day started another day: %v, count %d", again, ds.position().DayCount)
	}
}

// The base rate comes from the injected source: the day's rate is what the
// source says for that day, in the position and in the day's books.
func TestBaseRateComesFromTheSource(t *testing.T) {
	ds := NewDemoState()
	defer ds.db.Close()
	ds.rates = core.BaseRateFunc(func(day time.Time) float64 { return 0.01 + float64(day.Day())/1000 })
	addFundedCustomer(ds)
	ds.AdvanceDay()
	pos := ds.position()
	if want := 0.01 + float64(pos.Day.Day())/1000; pos.BoERate != want {
		t.Errorf("BoE rate %v on %v, want the source's %v", pos.BoERate, pos.Day, want)
	}
	if latest, ok := latestSnapshot(ds.db); !ok || latest.BoERate != pos.BoERate {
		t.Errorf("day's snapshot rate %v, want the source's %v", latest.BoERate, pos.BoERate)
	}
}

// Operational records keep the wall clock: a restart is a fact about the
// process, not the bank, and the simulated clock does not touch it.
func TestRestartRecordKeepsTheWallClock(t *testing.T) {
	ds := NewDemoState()
	defer ds.db.Close()
	restarts := ds.Restarts(1)
	if len(restarts) != 1 {
		t.Fatalf("%d restart rows, want this process's", len(restarts))
	}
	if since := time.Since(restarts[0].StartedAt); since < 0 || since > time.Minute {
		t.Errorf("restart recorded at %v, want the wall clock (the simulated day is %v)", restarts[0].StartedAt, ds.position().Day)
	}
}

// The simulation's clock's day and when it began are on the run row, so
// the next process resumes the same simulated day and the bank is on it.
func TestSimulatedClockResumesFromTheRunRow(t *testing.T) {
	first := NewDemoState()
	addFundedCustomer(first)
	first.AdvanceDay()
	first.AdvanceDay()
	day := first.sim.Clock().Day()
	if want := time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC); !day.Equal(want) {
		t.Fatalf("clock day after two advances = %v, want %v", day, want)
	}

	second := newDemoStateOn(first.db, "")
	if got := second.sim.Clock().Day(); !got.Equal(day) {
		t.Errorf("resumed clock day %v, want %v", got, day)
	}
	if pos := second.position(); !pos.Day.Equal(day) || pos.DayCount != 2 {
		t.Errorf("resumed bank position %+v, want day %v count 2", pos, day)
	}
	if now := second.sim.Clock().Now(); !now.After(day) || now.After(day.Add(24*time.Hour)) {
		t.Errorf("resumed clock Now = %v, want inside %v", now, day)
	}
}
