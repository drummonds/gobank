package main

import (
	"testing"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
)

// addFundedCustomer adds one customer through the real pipeline.
func addFundedCustomer(ds *DemoState) {
	ds.mu.Lock()
	cust, pii := generateCustomer(ds.rng, ds.nextCustSeq, ds.products, ds.currentDay)
	ds.nextCustSeq++
	ds.persistCustomer(&cust, pii)
	ds.customers = append(ds.customers, cust)
	ds.addCustomerToLedger(&ds.customers[len(ds.customers)-1])
	ds.fundCustomer(len(ds.customers) - 1)
	ds.mu.Unlock()
}

// TestInterestAccruesDaily verifies the products engine accrues interest in
// memory every day: no ledger movements, but visible accrued amounts.
func TestInterestAccruesDaily(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)

	for range 5 { // stays within January — no application yet
		ds.AdvanceDay()
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()
	var accrued, applied luca.Amount
	for _, a := range ds.customers[0].Accounts {
		accrued += a.Accrued
		applied += a.Interest
	}
	if accrued <= 0 {
		t.Errorf("no interest accrued after 5 days (accrued=%d)", accrued)
	}
	if applied != 0 {
		t.Errorf("interest applied before month end: %d", applied)
	}
	var count int
	if err := ds.db.QueryRow(`SELECT COUNT(*) FROM movements WHERE code = $1`, luca.CodeInterestAccrual).Scan(&count); err != nil {
		t.Fatalf("query movements: %v", err)
	}
	if count != 0 {
		t.Errorf("daily accrual should not write ledger movements, found %d", count)
	}
}

// TestInterestAppliedMonthly verifies accrued interest is applied to balances
// as ledger movements at month end, and the mirror matches the engine cache.
func TestInterestAppliedMonthly(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)

	for range 35 { // crosses the 31 Jan month end
		ds.AdvanceDay()
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()
	var applied luca.Amount
	for _, a := range ds.customers[0].Accounts {
		applied += a.Interest
		if ma, ok := ds.sim.GetManagedAccount(a.LedgerAccountID); ok {
			if ma.CachedBalance != a.Balance {
				t.Errorf("%s: mirror balance %d != engine cache %d", a.ProductName, a.Balance, ma.CachedBalance)
			}
		} else if a.LedgerAccountID != "" {
			t.Errorf("%s: managed account missing", a.ProductName)
		}
		if a.Rate > 0 && a.Balance > 0 && a.Interest == 0 {
			t.Errorf("%s: no interest applied after month end (balance %d, rate %v)", a.ProductName, a.Balance, a.Rate)
		}
	}
	if applied <= 0 {
		t.Fatalf("no interest applied after month end (applied=%d)", applied)
	}

	var count int
	if err := ds.db.QueryRow(`SELECT COUNT(*) FROM movements WHERE code = $1`, luca.CodeInterestAccrual).Scan(&count); err != nil {
		t.Fatalf("query movements: %v", err)
	}
	if count == 0 {
		t.Fatal("no interest movements in the ledger after month end")
	}
}

// TestNoFloatMoneyStorage is a tripwire for the float64-money prohibition:
// account, payment, and history money fields must be integer minor units.
func TestNoFloatMoneyStorage(t *testing.T) {
	var (
		_ luca.Amount = CustomerAccount{}.Balance
		_ luca.Amount = CustomerAccount{}.Interest
		_ luca.Amount = CustomerAccount{}.Accrued
		_ luca.Amount = Payment{}.Amount
		_ luca.Amount = TxEntry{}.Amount
		_ luca.Amount = TxEntry{}.Balance
		_ luca.Amount = BalancePoint{}.Savings
		_ luca.Amount = BalancePoint{}.Lending
		_ luca.Amount = GiltHolding{}.FaceValue
		_ int64       = ManagedAccountAccruedNumerator()
	)
}

// ManagedAccountAccruedNumerator anchors the engine-side integer accrual type.
func ManagedAccountAccruedNumerator() int64 {
	var ma gbp.ManagedAccount
	return ma.AccruedNumerator
}
