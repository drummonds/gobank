package main

import (
	"strings"
	"testing"
	"time"
)

// steppingClock advances one second per reading, so any interval measured
// between two readings is exactly 1s.
func steppingClock(start time.Time) func() time.Time {
	t := start
	return func() time.Time {
		t = t.Add(time.Second)
		return t
	}
}

func TestInterestThroughputRollingWindow(t *testing.T) {
	var tp interestThroughput
	if got := tp.per(interestWindow); got != 0 {
		t.Fatalf("no samples: %d", got)
	}
	tp.record(100, time.Second)
	if got := tp.per(interestWindow); got != 100*43200 {
		t.Errorf("one day at 100/s: %d, want %d", got, 100*43200)
	}
	tp.record(300, 3*time.Second) // still 100/s overall
	if got := tp.per(interestWindow); got != 100*43200 {
		t.Errorf("two days: %d", got)
	}
	// Only the most recent throughputDays count.
	for range throughputDays {
		tp.record(10, time.Second)
	}
	if got := tp.per(interestWindow); got != 10*43200 {
		t.Errorf("window should have rolled past the fast days: %d", got)
	}
	tp.record(5, 0)
	if got := tp.per(interestWindow); got <= 0 {
		t.Errorf("a zero-length sample must not zero the rate: %d", got)
	}
}

func TestDashboardReportsInterestMovementsPer12h(t *testing.T) {
	ds := NewDemoState()
	clock := steppingClock(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC))
	ds.now, ds.progress.now = clock, clock
	addFundedCustomer(ds)

	if got := ds.SimStatus().InterestPer12h; got != 0 {
		t.Fatalf("before any day: %d", got)
	}
	ds.AdvanceDay()

	ds.mu.Lock()
	var movements int
	// Customer accruals only: the BoE reserve posting is one movement a day
	// outside the per-account sweep the rate measures.
	err := ds.db.QueryRow(`SELECT COUNT(*) FROM movements WHERE code = $1 AND description = 'Daily interest accrual'`, codeDailyAccrual).Scan(&movements)
	ds.mu.Unlock()
	if err != nil || movements == 0 {
		t.Fatalf("accrual movements: %d, %v", movements, err)
	}
	// The rate is quoted over the whole day — engine, postings, bookkeeping
	// and projecting every account's position — the same span the runtime
	// page reports as the last day's duration, so it tracks how fast the
	// movements table actually grows. On the stepping clock the accrual
	// phase alone is 1s and the day is longer.
	last := ds.progress.snapshot()
	if last.LastDuration <= time.Second {
		t.Fatalf("last day took %v; the test needs a day longer than its accrual phase", last.LastDuration)
	}
	want := int64(float64(movements) * float64(interestWindow) / float64(last.LastDuration))
	if got := ds.SimStatus().InterestPer12h; got != want {
		t.Errorf("InterestPer12h = %d, want %d (%d movements over the day's %v)", got, want, movements, last.LastDuration)
	}
}

func TestDashboardReportsCustomersAddedPerSecond(t *testing.T) {
	ds := NewDemoState()
	ds.now = steppingClock(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC))

	ds.AddCustomersBatch(5)
	deadline := time.Now().Add(10 * time.Second)
	for ds.IsAddingCustomers() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := ds.position().Customers; n != 5 {
		t.Fatalf("customers = %d", n)
	}
	d := ds.SimStatus()
	// Batch start and end are the only clock readings: 5 customers in 1s.
	if d.LastCustomersPerSec != 5 {
		t.Errorf("LastCustomersPerSec = %v, want 5", d.LastCustomersPerSec)
	}
	if d.CustomersPerSec != 0 {
		t.Errorf("not adding, so the live rate should be 0, got %v", d.CustomersPerSec)
	}
}

func TestDashboardShowsRates(t *testing.T) {
	html := renderDashContent(DashData{Sim: SimStatus{InterestPer12h: 4320000, AddingCust: true, AddingProgress: 250, AddingTarget: 1000, CustomersPerSec: 83.4}})
	for _, want := range []string{"Interest movements / 12h", "4,320,000", "250 / 1000", "83 /s"} {
		if !contains(html, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
	html = renderDashContent(DashData{Sim: SimStatus{LastCustomersPerSec: 83.4}})
	if !contains(html, "Last add") || !contains(html, "83 /s") {
		t.Errorf("finished batch should keep its rate on show:\n%s", html)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
