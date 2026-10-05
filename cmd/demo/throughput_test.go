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

func TestPassThroughputRollingWindow(t *testing.T) {
	var tp passThroughput
	if got := tp.per(passWindow); got != 0 {
		t.Fatalf("no samples: %d", got)
	}
	tp.record(100, time.Second)
	if got := tp.per(passWindow); got != 100*43200 {
		t.Errorf("one day at 100/s: %d, want %d", got, 100*43200)
	}
	tp.record(300, 3*time.Second) // still 100/s overall
	if got := tp.per(passWindow); got != 100*43200 {
		t.Errorf("two days: %d", got)
	}
	// Only the most recent throughputDays count.
	for range throughputDays {
		tp.record(10, time.Second)
	}
	if got := tp.per(passWindow); got != 10*43200 {
		t.Errorf("window should have rolled past the fast days: %d", got)
	}
	tp.record(5, 0)
	if got := tp.per(passWindow); got <= 0 {
		t.Errorf("a zero-length sample must not zero the rate: %d", got)
	}
}

func TestDashboardReportsAccountDaysPer12h(t *testing.T) {
	ds := NewDemoState()
	clock := steppingClock(time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC))
	ds.now, ds.progress.now = clock, clock
	addFundedCustomer(ds)

	if got := ds.SimStatus().AccountDaysPer12h; got != 0 {
		t.Fatalf("before any day: %d", got)
	}
	ds.AdvanceDay()

	ds.mu.Lock()
	var accounts int
	err := ds.db.QueryRow(`SELECT COUNT(*) FROM customer_accounts`).Scan(&accounts)
	ds.mu.Unlock()
	if err != nil || accounts == 0 {
		t.Fatalf("accounts: %d, %v", accounts, err)
	}
	// The rate is quoted over the whole day — closing yesterday's books and
	// the pass over every account — the same span the runtime page reports
	// as the last day's duration.
	last := ds.progress.snapshot()
	if last.LastDuration <= 0 {
		t.Fatalf("last day took %v", last.LastDuration)
	}
	want := int64(float64(accounts) * float64(passWindow) / float64(last.LastDuration))
	if got := ds.SimStatus().AccountDaysPer12h; got != want {
		t.Errorf("AccountDaysPer12h = %d, want %d (%d accounts over the day's %v)", got, want, accounts, last.LastDuration)
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
	html := renderDashContent(DashData{Sim: SimStatus{AccountDaysPer12h: 4320000, AddingCust: true, AddingProgress: 250, AddingTarget: 1000, CustomersPerSec: 83.4}})
	for _, want := range []string{"Account days / 12h", "4,320,000", "250 / 1000", "83 /s"} {
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
