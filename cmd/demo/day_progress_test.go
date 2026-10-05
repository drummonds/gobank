package main

import (
	"strings"
	"testing"
	"time"
)

// fixedClock returns whatever *t currently holds, so a test moves time by
// assigning to it.
func fixedClock(t *time.Time) func() time.Time {
	return func() time.Time { return *t }
}

func TestDayProgressReportsPhaseCountAndRate(t *testing.T) {
	t0 := time.Date(2026, 9, 28, 17, 0, 0, 0, time.UTC)
	clock := t0
	var p dayProgress
	p.now = fixedClock(&clock)
	day := time.Date(2020, 2, 29, 0, 0, 0, 0, time.UTC)

	if got := p.snapshot(); got.Active || !got.LastDay.IsZero() {
		t.Fatalf("zero value should be idle with no last day: %+v", got)
	}

	p.begin(day)
	clock = t0.Add(2 * time.Second)
	p.phase("accrual postings", 1000)
	p.add(300)
	clock = t0.Add(5 * time.Second)

	got := p.snapshot()
	if !got.Active || !got.Day.Equal(day) || got.Phase != "accrual postings" {
		t.Fatalf("in progress: %+v", got)
	}
	if got.Done != 300 || got.Total != 1000 {
		t.Errorf("count = %d / %d, want 300 / 1000", got.Done, got.Total)
	}
	if got.Elapsed != 5*time.Second {
		t.Errorf("elapsed = %v, want 5s since the day began", got.Elapsed)
	}
	if got.Rate != 100 {
		t.Errorf("rate = %v/s, want 100 (300 in the phase's 3s)", got.Rate)
	}
}

func TestDayProgressRemembersLastDay(t *testing.T) {
	t0 := time.Date(2026, 9, 28, 17, 0, 0, 0, time.UTC)
	clock := t0
	var p dayProgress
	p.now = fixedClock(&clock)
	day := time.Date(2020, 2, 28, 0, 0, 0, 0, time.UTC)

	p.begin(day)
	p.phase("accrual postings", 500)
	p.add(200)
	p.add(300)
	p.phase("bookkeeping", 0)
	clock = t0.Add(10 * time.Second)
	p.finish()

	got := p.snapshot()
	if got.Active {
		t.Fatalf("finished day still active: %+v", got)
	}
	if !got.LastDay.Equal(day) || got.LastDuration != 10*time.Second || got.LastAccounts != 500 {
		t.Errorf("last day = %v in %v with %d accounts, want %v in 10s with 500",
			got.LastDay, got.LastDuration, got.LastAccounts, day)
	}
}

func TestAdvanceDayRecordsDayProgress(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	day := ds.currentDay
	ds.AdvanceDay()

	var accounts int
	ds.mu.Lock()
	err := ds.db.QueryRow(`SELECT COUNT(*) FROM customer_accounts`).Scan(&accounts)
	ds.mu.Unlock()
	if err != nil || accounts == 0 {
		t.Fatalf("accounts: %d, %v", accounts, err)
	}

	got := ds.progress.snapshot()
	if got.Active {
		t.Fatalf("day should be finished: %+v", got)
	}
	// The day that began is the one the pass projected: the day after the
	// one the bank was on when AdvanceDay was called.
	if !got.LastDay.Equal(day.AddDate(0, 0, 1)) || got.LastAccounts != accounts {
		t.Errorf("last day = %v with %d accounts, want %v with %d", got.LastDay, got.LastAccounts, day.AddDate(0, 0, 1), accounts)
	}
}

func TestRuntimeShowsDayInProgress(t *testing.T) {
	ds := NewDemoState()
	t0 := time.Date(2026, 9, 28, 17, 0, 0, 0, time.UTC)
	clock := t0
	ds.progress.now = fixedClock(&clock)

	ds.progress.begin(time.Date(2020, 2, 29, 0, 0, 0, 0, time.UTC))
	ds.progress.phase("projecting positions", 955000)
	ds.progress.add(404763)
	clock = t0.Add(5 * time.Minute)

	html := ds.BuildRuntimeHTML()
	for _, want := range []string{"Day in progress", "29 Feb 2020", "projecting positions", "404,763 / 955,000", "42%", "1,349/s", "5m0s"} {
		if !strings.Contains(html, want) {
			t.Errorf("runtime page missing %q", want)
		}
	}
}

func TestRuntimeShowsLastDayWhenIdle(t *testing.T) {
	ds := NewDemoState()
	t0 := time.Date(2026, 9, 28, 17, 0, 0, 0, time.UTC)
	clock := t0
	ds.progress.now = fixedClock(&clock)

	ds.progress.begin(time.Date(2020, 2, 28, 0, 0, 0, 0, time.UTC))
	ds.progress.phase("projecting positions", 477634)
	ds.progress.add(477634)
	clock = t0.Add(341 * time.Second)
	ds.progress.finish()

	html := ds.BuildRuntimeHTML()
	if strings.Contains(html, "Day in progress") {
		t.Error("idle simulation should not claim a day in progress")
	}
	for _, want := range []string{"Last day", "28 Feb 2020", "477,634 accounts", "5m41s"} {
		if !strings.Contains(html, want) {
			t.Errorf("runtime page missing %q", want)
		}
	}
}

func TestResetClearsDayProgress(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	ds.AdvanceDay()
	ds.Reset()
	if got := ds.progress.snapshot(); got.Active || !got.LastDay.IsZero() {
		t.Errorf("reset should forget the last day: %+v", got)
	}
}
