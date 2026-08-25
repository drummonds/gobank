package main

import (
	"testing"

	luca "git.bytestone.uk/hum3/go-luca"
)

// TestInterestAccountsCreated verifies initLedger creates the bank-level
// interest accounts and caches their IDs. Regression: these were only
// looked up (never created), so every interest posting was silently skipped.
func TestInterestAccountsCreated(t *testing.T) {
	ds := NewDemoState()
	if ds.interestExpenseAcctID == "" {
		t.Error("interestExpenseAcctID not set — Expense:Interest missing")
	}
	if ds.interestIncomeAcctID == "" {
		t.Error("interestIncomeAcctID not set — Income:Interest missing")
	}
}

// TestDailyInterestPostedToDB verifies that advancing a day writes interest
// accrual movements into the movements table as normal transactions.
func TestDailyInterestPostedToDB(t *testing.T) {
	ds := NewDemoState()

	// Add a funded customer the same way advanceDay does.
	ds.mu.Lock()
	cust, pii := generateCustomer(ds.rng, ds.nextCustSeq, ds.products, ds.currentDay)
	ds.nextCustSeq++
	ds.persistCustomer(&cust, pii)
	ds.customers = append(ds.customers, cust)
	ds.addCustomerToLedger(&ds.customers[len(ds.customers)-1])
	ds.fundCustomer(len(ds.customers) - 1)
	ds.mu.Unlock()

	ds.AdvanceDay()

	ds.mu.Lock()
	pending := len(ds.pendingMovements)
	db := ds.db
	ds.mu.Unlock()

	if pending != 0 {
		t.Errorf("pendingMovements not flushed after AdvanceDay: %d left", pending)
	}

	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM movements WHERE code = $1`, luca.CodeInterestAccrual).Scan(&count)
	if err != nil {
		t.Fatalf("query movements: %v", err)
	}
	if count == 0 {
		t.Fatal("no interest accrual movements in the database after AdvanceDay")
	}
}
