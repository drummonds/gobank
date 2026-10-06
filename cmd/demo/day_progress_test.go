package main

import (
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/gobank/bank"
)

func TestAdvanceDayRecordsDayProgress(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	day := ds.position().Day
	ds.AdvanceDay()

	var accounts int
	ds.mu.Lock()
	err := ds.db.QueryRow(`SELECT COUNT(*) FROM customer_accounts`).Scan(&accounts)
	ds.mu.Unlock()
	if err != nil || accounts == 0 {
		t.Fatalf("accounts: %d, %v", accounts, err)
	}

	got := ds.Progress()
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
	html := progressRow(bank.DayProgress{
		Active: true, Day: time.Date(2020, 2, 29, 0, 0, 0, 0, time.UTC), Phase: "projecting positions",
		Done: 404763, Total: 955000, Elapsed: 5 * time.Minute, Rate: 404763 / (5 * 60.0),
	})
	for _, want := range []string{"Day in progress", "29 Feb 2020", "projecting positions", "404,763 / 955,000", "42%", "1,349/s", "5m0s"} {
		if !strings.Contains(html, want) {
			t.Errorf("runtime page missing %q", want)
		}
	}
}

func TestRuntimeShowsLastDayWhenIdle(t *testing.T) {
	html := progressRow(bank.DayProgress{
		LastDay: time.Date(2020, 2, 28, 0, 0, 0, 0, time.UTC), LastDuration: 341 * time.Second, LastAccounts: 477634,
	})
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
	if got := ds.Progress(); got.Active || !got.LastDay.IsZero() {
		t.Errorf("reset should forget the last day: %+v", got)
	}
}
