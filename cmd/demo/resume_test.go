package main

import (
	"context"
	"testing"
	"time"
)

// accrualNumerators is every registered account's accrued-but-unapplied
// numerator, by ledger account ID.
func accrualNumerators(ds *DemoState) map[string]int64 {
	out := map[string]int64{}
	custs, _ := ds.customerPage(1)
	for _, c := range custs {
		for _, a := range c.Accounts {
			out[a.LedgerAccountID] = a.AccruedE7
		}
	}
	return out
}

// A restart over the same database resumes the run: the bank is on the
// same day with the same customers, balances and accrued interest, and
// carries on from there without colliding with its own rows.
func TestRestartResumesTheRun(t *testing.T) {
	first := NewDemoState()
	for range 3 {
		first.createCustomer()
	}
	for range 40 { // crosses a month end, so interest has been applied
		first.advanceDay()
	}
	first.SendPayment()
	before := first.position()
	beforeAccrual := accrualNumerators(first)
	beforeInterestIncome, beforeInterestExpense := first.interestTotals()
	if before.Customers < 3 || before.DayCount != 40 || before.Savings == 0 { // the days add customers of their own
		t.Fatalf("run before restart: %+v", before)
	}

	// The new process: same database, nothing in memory.
	second := newDemoStateOn(first.db, "")
	after := second.position()
	if !after.Day.Equal(before.Day) || after.DayCount != before.DayCount || after.Customers != before.Customers {
		t.Errorf("resumed position = day %s (%d), %d customers; want day %s (%d), %d customers",
			after.Day.Format("2006-01-02"), after.DayCount, after.Customers, before.Day.Format("2006-01-02"), before.DayCount, before.Customers)
	}
	if after.Savings != before.Savings || after.Lending != before.Lending {
		t.Errorf("resumed book = savings %d, lending %d; want %d, %d", after.Savings, after.Lending, before.Savings, before.Lending)
	}
	if after.BoEInterest != before.BoEInterest {
		t.Errorf("resumed BoE interest = %d; want %d", after.BoEInterest, before.BoEInterest)
	}
	if got := accrualNumerators(second); len(got) != len(beforeAccrual) {
		t.Fatalf("resumed accounts = %d; want %d", len(got), len(beforeAccrual))
	} else {
		for id, n := range beforeAccrual {
			if got[id] != n {
				t.Errorf("account %s accrued numerator = %d after restart; want %d", id, got[id], n)
			}
		}
	}
	if income, expense := second.interestTotals(); income != beforeInterestIncome || expense != beforeInterestExpense {
		t.Errorf("resumed interest totals = %d, %d; want %d, %d", income, expense, beforeInterestIncome, beforeInterestExpense)
	}

	// Carrying on: a new customer, a payment and a day all work, which means
	// the sequence numbers continued rather than restarting at one.
	second.createCustomer()
	if n := second.customerCount(); n != before.Customers+1 {
		t.Errorf("customers after one more = %d; want %d (a colliding ID is dropped)", n, before.Customers+1)
	}
	second.SendPayment()
	page, _ := second.Bank.PaymentPage(context.Background(), 1)
	if page.Total < 2 {
		t.Errorf("payments after restart and one more = %d; want at least 2", page.Total)
	}
	second.advanceDay()
	if p := second.position(); p.DayCount != 41 {
		t.Errorf("day count after one more day = %d; want 41", p.DayCount)
	}
	if got := accrualNumerators(second); len(got) < len(beforeAccrual) {
		t.Errorf("accounts after one more day = %d; want at least %d", len(got), len(beforeAccrual))
	}
	for id, n := range accrualNumerators(second) {
		if was := beforeAccrual[id]; was > 0 && n == was { // an unfunded loan accrues nothing either way
			t.Errorf("account %s did not accrue after restart (still %d)", id, n)
		}
	}
}

// A fresh database starts at day zero and has nothing to resume.
func TestFreshDatabaseStartsAtDayZero(t *testing.T) {
	ds := NewDemoState()
	if ds.ResumedRunning() {
		t.Error("nothing was running before a fresh start")
	}
	run, ok := loadRun(ds.db)
	if !ok || run.DayCount != 0 || !run.Day.Equal(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("a fresh start records day zero: %+v, %v", run, ok)
	}
}

// Whether the run loop was going is part of the run, so a restart picks
// up where the operator left it: running stays running, stopped stays
// stopped.
func TestRunRowFollowsStartAndStop(t *testing.T) {
	ds := NewDemoState()
	ds.Start()
	if run, _ := loadRun(ds.db); !run.Running {
		t.Error("Start should record the run as running")
	}
	ds.Stop()
	if run, _ := loadRun(ds.db); run.Running {
		t.Error("Stop should record the run as stopped")
	}

	ds.Start()
	if err := ds.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ds.IsRunning() {
		t.Error("after Shutdown the loop has stopped")
	}
	if run, _ := loadRun(ds.db); !run.Running {
		t.Error("Shutdown is not Stop: the run is still running as far as the next process is concerned")
	}
	second := newDemoStateOn(ds.db, "")
	if !second.ResumedRunning() {
		t.Error("the next process should see a run to carry on")
	}
}

// Reset is a fresh run on the same database: day zero, no customers, and
// a restart after it resumes the fresh run, not the old one.
func TestResetStartsAFreshRun(t *testing.T) {
	ds := NewDemoState()
	ds.createCustomer()
	ds.advanceDay()
	ds.Reset()
	run, ok := loadRun(ds.db)
	if !ok || run.DayCount != 0 || run.Running {
		t.Errorf("run after reset: %+v, %v; want day zero, stopped", run, ok)
	}
	second := newDemoStateOn(ds.db, "")
	if p := second.position(); p.DayCount != 0 || p.Customers != 0 {
		t.Errorf("resumed after reset: %+v; want nothing", p)
	}
}

// The day length set on the console is the run's: the next process over
// the same database resumes at it, whatever its environment says. The
// environment's value (GOBANK_DAY_LENGTH) is where a run starts until the
// console sets one.
func TestDayLengthSetOnTheConsoleOutlivesTheProcess(t *testing.T) {
	first := NewDemoState()
	first.DefaultDayLength(0)
	first.SetDayLength(2 * time.Hour)

	second := newDemoStateOn(first.db, "")
	second.DefaultDayLength(0) // the environment, flat out, as main applies it after the resume
	if got := second.Settings().DayLength; got != 2*time.Hour {
		t.Errorf("resumed day length = %s; want the 2h set on the console", got)
	}

	// Flat out set on the console is a setting too, not an absence.
	second.SetDayLength(0)
	third := newDemoStateOn(first.db, "")
	third.DefaultDayLength(2 * time.Hour)
	if got := third.Settings().DayLength; got != 0 {
		t.Errorf("resumed day length = %s; want the flat out set on the console", got)
	}

	fresh := NewDemoState()
	fresh.DefaultDayLength(90 * time.Minute)
	if got := fresh.Settings().DayLength; got != 90*time.Minute {
		t.Errorf("fresh day length = %s; want the environment's 90m", got)
	}
}
