package main

import (
	"context"
	"testing"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
)

// addFundedCustomer adds one customer through the real pipeline.
func addFundedCustomer(ds *DemoState) {
	ds.createCustomer()
}

// firstCustomerAccounts reads cust-001's accounts through the read model.
func firstCustomerAccounts(t *testing.T, ds *DemoState) []CustomerAccount {
	t.Helper()
	cust, ok := ds.customerByID("cust-001")
	if !ok {
		t.Fatal("cust-001 not found")
	}
	return cust.Accounts
}

// allCustomers reads every customer through the read model (tests keep to
// one page).
func allCustomers(t *testing.T, ds *DemoState) []CustomerRecord {
	t.Helper()
	page, total := ds.customerPage(1)
	if total != len(page) {
		t.Fatalf("allCustomers: %d of %d on the first page", len(page), total)
	}
	return page
}

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
	if err := ds.db.QueryRow(`SELECT COUNT(*) FROM movements WHERE code = $1 AND description = 'Daily interest accrual'`, codeDailyAccrual).Scan(&customerAccruals); err != nil {
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
	if want := appliedSavings + poundsE7(accruedSavingsE7).Pence(); expense != want {
		t.Errorf("deposit interest expense %d, want applied %d + accrued %d", expense, appliedSavings, poundsE7(accruedSavingsE7).Pence())
	}
	if want := appliedLending + poundsE7(accruedLendingE7).Pence(); income != want {
		t.Errorf("loan interest income %d, want applied %d + accrued %d", income, appliedLending, poundsE7(accruedLendingE7).Pence())
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
		bal, err := ds.ledger.Balance(a.LedgerAccountID)
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
		_ luca.Amount = CustomerAccount{}.Balance
		_ luca.Amount = CustomerAccount{}.Interest
		_ luca.Amount = CustomerAccount{}.Accrued
		_ luca.Amount = Payment{}.Amount
		_ luca.Amount = TxEntry{}.Amount
		_ luca.Amount = TxEntry{}.Balance
		_ luca.Amount = BalancePoint{}.Savings
		_ luca.Amount = BalancePoint{}.Lending
		_ luca.Amount = core.GiltHolding{}.FaceValue
		_ int64       = CustomerAccount{}.AccruedE7
		_ int64       = luca.Position{}.Accrued.Num
		_ int64       = dayResult{}.accrued
		_ luca.Amount = dayResult{}.applied
		_ luca.Amount = core.Account{}.Balance
		_ luca.Amount = core.Account{}.Interest
		_ int64       = core.Account{}.AccruedE7
		_ luca.Amount = livePosition{}.balance
		_ int64       = livePosition{}.accruedE7
		_ luca.Amount = core.Payment{}.Amount
		_ luca.Amount = core.Position{}.Savings
	)
	var ds DemoState
	var _ int64 = ds.boePostedPence
	var _ luca.Amount = ds.boeInterestApplied
}

// TestPoundsE7 pins the 7dp-pounds accrual model: numerator-to-7dp conversion
// rounds to nearest, and the pence conversion truncates like the engine's
// application (whole pence move, remainders keep accruing).
func TestPoundsE7(t *testing.T) {
	if got := accrualPoundsE7(gbp.AccrualDenominator); got != poundsE7PerPenny {
		t.Errorf("one penny of numerator = %d e7-units, want %d", got, poundsE7PerPenny)
	}
	if got := accrualPoundsE7(gbp.AccrualDenominator / 2); got != poundsE7PerPenny/2 {
		t.Errorf("half penny = %d e7-units, want %d", got, poundsE7PerPenny/2)
	}
	if got := accrualPoundsE7(-gbp.AccrualDenominator); got != -poundsE7PerPenny {
		t.Errorf("negative penny = %d e7-units, want %d", got, -poundsE7PerPenny)
	}
	if got := poundsE7(poundsE7PerPenny / 2).Pence(); got != 0 {
		t.Errorf("half penny should truncate to 0 pence, got %d", got)
	}
	if got := poundsE7(poundsE7PerPenny).Pence(); got != 1 {
		t.Errorf("one penny in e7-units = %d pence, want 1", got)
	}
	if got := poundsE7(12_345_678).String(); got != "£1.2345678" {
		t.Errorf("String() = %q, want £1.2345678", got)
	}
	if got := poundsE7(-12_345_678_900_000).String(); got != "-£1,234,567.8900000" {
		t.Errorf("String() = %q, want -£1,234,567.8900000", got)
	}
}

// assertPositionsAreTheTruth checks every account's position for today
// is what the read model shows, and that the BoE reserves position for
// the day just closed carries ds.boeAccruedNumerator. Caller holds ds.mu.
func assertPositionsAreTheTruth(t *testing.T, ds *DemoState) {
	t.Helper()
	today := ds.currentDay
	for _, a := range firstCustomerAccounts(t, ds) {
		p, err := ds.ledger.PositionAt(a.LedgerAccountID, today)
		if err != nil || p == nil {
			t.Fatalf("%s: position for %s: %v %v", a.ProductName, today.Format("2006-01-02"), p, err)
		}
		if !p.Day.Equal(today) {
			t.Errorf("%s: latest position is %s, want %s", a.ProductName, p.Day.Format("2006-01-02"), today.Format("2006-01-02"))
		}
		if p.Accrued.Den != gbp.AccrualDenominator {
			t.Errorf("%s: accrual denominator %d, want %d", a.ProductName, p.Accrued.Den, gbp.AccrualDenominator)
		}
		if a.AccruedE7 != int64(accrualPoundsE7(p.Accrued.Num)) {
			t.Errorf("%s: read model accrual %d != position's %d at 7dp", a.ProductName, a.AccruedE7, accrualPoundsE7(p.Accrued.Num))
		}
		if a.Balance != p.Balance {
			t.Errorf("%s: read model balance %d != position's %d", a.ProductName, a.Balance, p.Balance)
		}
	}
	closed := today.AddDate(0, 0, -1)
	p, err := ds.ledger.PositionAt(ds.boeReservesID, closed)
	if err != nil || p == nil || !p.Day.Equal(closed) {
		t.Fatalf("BoE reserves position for %s: %v %v", closed.Format("2006-01-02"), p, err)
	}
	if p.Accrued.Num != ds.boeAccruedNumerator {
		t.Errorf("stored BoE numerator %d != state %d", p.Accrued.Num, ds.boeAccruedNumerator)
	}
}

// TestPositionsAreTheTruth verifies the daily pass projects every account's
// position (mid-month and across a month-end application) so the ledger
// alone carries the balance and accrued-but-unapplied interest, and that
// the retired accrual_state table is gone.
func TestPositionsAreTheTruth(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)

	for range 5 { // mid-month: pure accrual, nothing applied
		ds.AdvanceDay()
	}
	ds.mu.Lock()
	assertPositionsAreTheTruth(t, ds)
	ds.mu.Unlock()

	for range 30 { // crosses the 31 Jan month end: numerators drop to remainders
		ds.AdvanceDay()
	}
	ds.mu.Lock()
	defer ds.mu.Unlock()
	assertPositionsAreTheTruth(t, ds)
	if rows, err := ds.db.Query(`SELECT 1 FROM accrual_state`); err == nil {
		rows.Close()
		t.Error("accrual_state still exists; the products component owns no table")
	}
}

// TestBoEInterestInLedger verifies BoE reserve interest is modelled in
// accounts: income recognised daily into an accrual receivable, moved into
// Asset:BoEReserves at month end, with the sub-penny remainder carried in the
// numerator (invariant: posted pence == floor(numerator/denominator)).
func TestBoEInterestInLedger(t *testing.T) {
	ds := NewDemoState()
	for range 8 {
		addFundedCustomer(ds)
	}
	// A savings-only customer makes the book savings-heavy whatever the
	// generator dealt, so excess cash accrues BoE interest.
	if _, err := newCoreAdapter(ds, "").OpenCustomer(context.Background(), core.NewCustomer{
		PII:      core.PII{Name: "Saver"},
		Accounts: []core.NewAccount{{ProductID: gbp.EasyAccess().ID, Opening: 50_000_00}},
	}); err != nil {
		t.Fatal(err)
	}

	assertBoE := func(applied bool) {
		t.Helper()
		if ds.boePostedPence != ds.boeAccruedNumerator/gbp.AccrualDenominator {
			t.Errorf("posted pence %d != floor(numerator) %d", ds.boePostedPence, ds.boeAccruedNumerator/gbp.AccrualDenominator)
		}
		holding, err := ds.ledger.Balance(ds.accrBoEID)
		if err != nil {
			t.Fatalf("holding balance: %v", err)
		}
		if holding != luca.Amount(ds.boePostedPence) {
			t.Errorf("Asset:AccruedInterest:BoE balance %d != posted pence %d", holding, ds.boePostedPence)
		}
		reserves, err := ds.ledger.Balance(ds.boeReservesID)
		if err != nil {
			t.Fatalf("reserves balance: %v", err)
		}
		if reserves != ds.boeInterestApplied {
			t.Errorf("Asset:BoEReserves balance %d != applied %d", reserves, ds.boeInterestApplied)
		}
		income, err := ds.ledger.Balance(ds.incomeBoEID)
		if err != nil {
			t.Fatalf("income balance: %v", err)
		}
		if want := -(ds.boeInterestApplied + luca.Amount(ds.boePostedPence)); income != want {
			t.Errorf("Income:Interest:BoE balance %d != %d", income, want)
		}
		if applied && ds.boeInterestApplied <= 0 {
			t.Error("no BoE interest applied after month end")
		}
	}

	for range 5 { // mid-month: accrual only
		ds.AdvanceDay()
	}
	ds.mu.Lock()
	if ds.boeAccruedNumerator <= 0 {
		t.Fatal("no BoE interest accrued — book not savings-heavy?")
	}
	assertBoE(false)
	if ds.boeInterestApplied != 0 {
		t.Errorf("BoE interest applied before month end: %d", ds.boeInterestApplied)
	}
	ds.mu.Unlock()

	for range 30 { // crosses the 31 Jan month end
		ds.AdvanceDay()
	}
	ds.mu.Lock()
	defer ds.mu.Unlock()
	assertBoE(true)
}

// TestBoEInterestRestored verifies the BoE fields rehydrate from the DB:
// numerator and posted-pence from the reserve account's position, applied
// from its balance.
func TestBoEInterestRestored(t *testing.T) {
	ds := NewDemoState()
	for range 8 {
		addFundedCustomer(ds)
	}
	for range 35 {
		ds.AdvanceDay()
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()
	wantNum, wantPosted, wantApplied := ds.boeAccruedNumerator, ds.boePostedPence, ds.boeInterestApplied
	if wantApplied <= 0 {
		t.Fatal("no applied BoE interest to restore — test would be vacuous")
	}
	ds.boeAccruedNumerator, ds.boePostedPence, ds.boeInterestApplied = 0, 0, 0

	ds.refreshFromLedger()

	if ds.boeAccruedNumerator != wantNum {
		t.Errorf("numerator not restored: got %d, want %d", ds.boeAccruedNumerator, wantNum)
	}
	if ds.boePostedPence != wantPosted {
		t.Errorf("posted pence not restored: got %d, want %d", ds.boePostedPence, wantPosted)
	}
	if ds.boeInterestApplied != wantApplied {
		t.Errorf("applied not restored: got %d, want %d", ds.boeInterestApplied, wantApplied)
	}
}
