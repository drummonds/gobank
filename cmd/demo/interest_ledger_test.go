package main

import (
	"git.bytestone.uk/hum3/gobank/bank"
	"git.bytestone.uk/hum3/gobank/bank/customers"
	"git.bytestone.uk/hum3/gobank/bank/payments"
	"git.bytestone.uk/hum3/gobank/bank/products"
	"git.bytestone.uk/hum3/gobank/bff/staff"
	"testing"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/core"
)

// TestInterestAccruesDaily verifies the pass accrues interest every day
// with visible accrued amounts, and that no application movements appear
// before month end. Daily accrual posts nothing: it lives on the position.
func TestInterestAccruesDaily(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)

	for range 5 { // stays within January — no application yet
		ds.AdvanceDay()
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()
	var accrued, applied luca.Amount
	for _, a := range firstCustomerAccounts(t, ds) {
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
		t.Errorf("daily accrual should not write application movements, found %d", count)
	}
	var customerAccruals int
	if err := ds.db.QueryRow(`SELECT COUNT(*) FROM movements WHERE code = $1 AND description = 'Daily interest accrual'`, bank.CodeDailyAccrual).Scan(&customerAccruals); err != nil {
		t.Fatalf("query movements: %v", err)
	}
	if customerAccruals != 0 {
		t.Errorf("customer accrual posted %d daily movements; it belongs on the position", customerAccruals)
	}
}

// interestByFamily sums the read model's applied interest and its accrued
// interest at 7dp over every account of every customer, per family.
func interestByFamily(t *testing.T, ds *DemoState) (appliedSavings, appliedLending luca.Amount, accruedSavingsE7, accruedLendingE7 int64) {
	t.Helper()
	for _, c := range allCustomers(t, ds) {
		for _, a := range c.Accounts {
			if a.Family == gbp.FamilySavings {
				appliedSavings += a.Interest
				accruedSavingsE7 += a.AccruedE7
			} else {
				appliedLending += a.Interest
				accruedLendingE7 += a.AccruedE7
			}
		}
	}
	return
}

// The bank's interest to date on an accrual basis is what the ledger
// shows: the P&L accounts carry the applications, the positions carry the
// accrual not yet applied, and nothing is posted daily to get there.
func TestInterestTotalsOnAnAccrualBasis(t *testing.T) {
	ds := NewDemoState()
	for range 3 {
		addFundedCustomer(ds)
	}
	for range 35 { // crosses the 31 Jan application
		ds.AdvanceDay()
	}

	appliedSavings, appliedLending, accruedSavingsE7, accruedLendingE7 := interestByFamily(t, ds)
	if appliedSavings <= 0 || accruedSavingsE7 <= 0 {
		t.Fatalf("savings applied %d, accrued %d e7: the test needs both", appliedSavings, accruedSavingsE7)
	}
	income, expense := ds.interestTotals()
	if want := appliedSavings + staff.PoundsE7(accruedSavingsE7).Pence(); expense != want {
		t.Errorf("deposit interest expense %d, want applied %d + accrued %d", expense, appliedSavings, staff.PoundsE7(accruedSavingsE7).Pence())
	}
	if want := appliedLending + staff.PoundsE7(accruedLendingE7).Pence(); income != want {
		t.Errorf("loan interest income %d, want applied %d + accrued %d", income, appliedLending, staff.PoundsE7(accruedLendingE7).Pence())
	}
}

// TestInterestAppliedMonthly verifies accrued interest is applied to
// balances as ledger movements at month end, booked by the pass the
// following morning.
func TestInterestAppliedMonthly(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)

	for range 35 { // crosses the 31 Jan month end
		ds.AdvanceDay()
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()
	var applied luca.Amount
	for _, a := range firstCustomerAccounts(t, ds) {
		applied += a.Interest
		if a.Rate > 0 && a.Balance > 0 && a.Interest == 0 {
			t.Errorf("%s: no interest applied after month end (balance %d, rate %v)", a.ProductName, a.Balance, a.Rate)
		}
		bal, err := ds.Ledger().Balance(a.LedgerAccountID)
		if err != nil {
			t.Fatal(err)
		}
		if bal != a.Balance {
			t.Errorf("%s: read model balance %d != ledger %d", a.ProductName, a.Balance, bal)
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
		_ luca.Amount = customers.Account{}.Balance
		_ luca.Amount = customers.Account{}.Interest
		_ luca.Amount = customers.Account{}.Accrued
		_ luca.Amount = payments.Payment{}.Amount
		_ luca.Amount = core.Transaction{}.Amount
		_ luca.Amount = core.Transaction{}.Balance
		_ luca.Amount = core.BalancePoint{}.Savings
		_ luca.Amount = core.BalancePoint{}.Lending
		_ luca.Amount = core.GiltHolding{}.FaceValue
		_ int64       = customers.Account{}.AccruedE7
		_ int64       = luca.Position{}.Accrued.Num
		_ int64       = products.DayResult{}.Accrued
		_ luca.Amount = products.DayResult{}.Applied
		_ luca.Amount = core.Account{}.Balance
		_ luca.Amount = core.Account{}.Interest
		_ int64       = core.Account{}.AccruedE7
		_ luca.Amount = ledger.Position{}.Balance
		_ int64       = ledger.Position{}.AccruedE7
		_ luca.Amount = core.Payment{}.Amount
		_ luca.Amount = core.Position{}.Savings
	)
}

// TestPoundsE7 pins the 7dp-pounds accrual model: numerator-to-7dp conversion
// rounds to nearest, and the pence conversion truncates like the engine's
// application (whole pence move, remainders keep accruing).
func TestPoundsE7(t *testing.T) {
	if got := staff.AccrualPoundsE7(gbp.AccrualDenominator); got != staff.PoundsE7PerPenny {
		t.Errorf("one penny of numerator = %d e7-units, want %d", got, staff.PoundsE7PerPenny)
	}
	if got := staff.AccrualPoundsE7(gbp.AccrualDenominator / 2); got != staff.PoundsE7PerPenny/2 {
		t.Errorf("half penny = %d e7-units, want %d", got, staff.PoundsE7PerPenny/2)
	}
	if got := staff.AccrualPoundsE7(-gbp.AccrualDenominator); got != -staff.PoundsE7PerPenny {
		t.Errorf("negative penny = %d e7-units, want %d", got, -staff.PoundsE7PerPenny)
	}
	if got := staff.PoundsE7(staff.PoundsE7PerPenny / 2).Pence(); got != 0 {
		t.Errorf("half penny should truncate to 0 pence, got %d", got)
	}
	if got := staff.PoundsE7(staff.PoundsE7PerPenny).Pence(); got != 1 {
		t.Errorf("one penny in e7-units = %d pence, want 1", got)
	}
	if got := staff.PoundsE7(12_345_678).String(); got != "£1.2345678" {
		t.Errorf("String() = %q, want £1.2345678", got)
	}
	if got := staff.PoundsE7(-12_345_678_900_000).String(); got != "-£1,234,567.8900000" {
		t.Errorf("String() = %q, want -£1,234,567.8900000", got)
	}
}
