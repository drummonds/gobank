package main

import (
	"context"
	"git.bytestone.uk/hum3/gobank/bank/parameters"
	"git.bytestone.uk/hum3/gobank/bank/payments"
	"git.bytestone.uk/hum3/gobank/cmd/demo/sim"
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
	bank := ds.Bank
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
	// The posting is the settlement (ADR-0010): completed at the same instant.
	if settled, _ := ds.paymentByID(p.ID); settled.Status != payments.Completed || !settled.SettledAt.Equal(at) {
		t.Errorf("transfer %+v, want completed at the clock's %v", settled, at)
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
	bank := ds.Bank
	ctx := context.Background()
	addFundedCustomer(ds)
	// The test moves the bank's clock itself, past the simulation's
	// timeline: the market's opening publication is made by hand.
	if err := bank.Publish(ctx, sim.BoERates[0]); err != nil {
		t.Fatal(err)
	}

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
	if pending, _ := anyUnprojected(ds, day); pending {
		t.Error("the caught-up day's pass is not complete")
	}

	if again, _ := bank.StartDay(ctx); !again.Equal(day) || ds.position().DayCount != 2 {
		t.Errorf("a second StartDay on the same clock day started another day: %v, count %d", again, ds.position().DayCount)
	}
}

// The risk-free rate is the market's publication (ADR-0009): the
// simulation replays the Bank of England's series as the pound's feed, so
// the rate a day closed at is the series' rate in force at its end, in the
// position and in the day's snapshot.
func TestTheRateIsTheMarketsPublication(t *testing.T) {
	ds := NewDemoState()
	defer ds.db.Close()
	addFundedCustomer(ds)
	ds.AdvanceDay()
	pos := ds.position()
	closed := pos.Day.AddDate(0, 0, -1)
	want, ok := sim.BoERates.InForce(parameters.EndOfDay(closed))
	if !ok || pos.RiskFreeBps != want || want == 0 {
		t.Errorf("risk-free rate %d on %v, want the series' %d, %v", pos.RiskFreeBps, pos.Day, want, ok)
	}
	if h, err := ds.History(context.Background()); err != nil || len(h.RiskFree) == 0 || h.RiskFree[len(h.RiskFree)-1].Bps != pos.RiskFreeBps {
		t.Errorf("day's snapshot rate %+v, want the market's %d", h.RiskFree, pos.RiskFreeBps)
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

// The instant the simulation's clock stands at is on the run row, so the
// next process resumes at the same instant and the bank is on its day.
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
	if now := second.sim.Clock().Now(); !now.Equal(first.sim.Clock().Now()) {
		t.Errorf("resumed clock Now = %v, want the recorded instant %v", now, first.sim.Clock().Now())
	}
}
