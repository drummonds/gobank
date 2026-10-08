package main

import (
	"testing"
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

func TestResetClearsDayProgress(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	ds.AdvanceDay()
	ds.Reset()
	if got := ds.Progress(); got.Active || !got.LastDay.IsZero() {
		t.Errorf("reset should forget the last day: %+v", got)
	}
}
