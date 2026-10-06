package bank

import (
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
	var p progress
	p.now = fixedClock(&clock)
	day := time.Date(2020, 2, 29, 0, 0, 0, 0, time.UTC)

	if got := p.snapshot(); got.Active || !got.LastDay.IsZero() {
		t.Fatalf("zero value should be idle with no last day: %+v", got)
	}

	p.begin(day)
	clock = t0.Add(2 * time.Second)
	p.Phase("accrual postings", 1000)
	p.Add(300)
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
	var p progress
	p.now = fixedClock(&clock)
	day := time.Date(2020, 2, 28, 0, 0, 0, 0, time.UTC)

	p.begin(day)
	p.Phase("accrual postings", 500)
	p.Add(200)
	p.Add(300)
	p.Phase("bookkeeping", 0)
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

// The pass's throughput is averaged over the recent days only.
func TestThroughputRollingWindow(t *testing.T) {
	const window = 12 * time.Hour
	var tp throughput
	if got := tp.per(window); got != 0 {
		t.Fatalf("no samples: %d", got)
	}
	tp.record(100, time.Second)
	if got := tp.per(window); got != 100*43200 {
		t.Errorf("one day at 100/s: %d, want %d", got, 100*43200)
	}
	tp.record(300, 3*time.Second) // still 100/s overall
	if got := tp.per(window); got != 100*43200 {
		t.Errorf("two days: %d", got)
	}
	for range throughputDays {
		tp.record(10, time.Second)
	}
	if got := tp.per(window); got != 10*43200 {
		t.Errorf("window should have rolled past the fast days: %d", got)
	}
	tp.record(5, 0)
	if got := tp.per(window); got <= 0 {
		t.Errorf("a zero-length sample must not zero the rate: %d", got)
	}
}

// A day begun is the one the pass projected, and its duration and account
// count are what the throughput is quoted on.
func TestADayRecordsItsProgressAndThroughput(t *testing.T) {
	f := open(t)
	if _, err := f.bank.OpenCustomer(f.ctx, ada); err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	clock := t0
	f.bank.SetProgressClock(func() time.Time { clock = clock.Add(time.Second); return clock })
	if got := f.bank.AccountDaysPer(12 * time.Hour); got != 0 {
		t.Fatalf("before any day: %d", got)
	}
	f.nextDay()
	got := f.bank.Progress()
	if got.Active || !got.LastDay.Equal(opening.AddDate(0, 0, 1)) || got.LastAccounts != 2 || got.LastDuration <= 0 {
		t.Errorf("progress after a day %+v; want day 2 done with 2 accounts", got)
	}
	want := int64(float64(2) * float64(12*time.Hour) / float64(got.LastDuration))
	if per := f.bank.AccountDaysPer(12 * time.Hour); per != want {
		t.Errorf("AccountDaysPer(12h) = %d, want %d", per, want)
	}
}
