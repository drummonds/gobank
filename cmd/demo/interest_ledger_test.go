package main

import (
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

// TestInterestAccruesDaily verifies the products engine accrues interest every
// day with visible accrued amounts, and that no application movements (the
// engine's month-end code) appear before month end. Daily accruals do post
// ledger movements, under codeDailyAccrual — see TestDailyAccrualMovements.
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
}

// accruedPenceByFamily sums floor(numerator/denominator) over every account of
// every customer per family (the holding accounts are bank-wide). Caller holds ds.mu.
func accruedPenceByFamily(t *testing.T, ds *DemoState) (savings, lending luca.Amount) {
	t.Helper()
	for _, c := range allCustomers(t, ds) {
		for _, a := range c.Accounts {
			ma, ok := ds.sim.GetManagedAccount(a.LedgerAccountID)
			if !ok {
				t.Fatalf("%s: managed account missing", a.ProductName)
			}
			pence := luca.Amount(ma.AccruedNumerator / gbp.AccrualDenominator)
			if a.Family == gbp.FamilySavings {
				savings += pence
			} else {
				lending += pence
			}
		}
	}
	return savings, lending
}

// assertAccrualHoldings checks the AccruedInterest holding account balances
// equal the whole pence currently accrued but unapplied. Caller holds ds.mu.
func assertAccrualHoldings(t *testing.T, ds *DemoState) {
	t.Helper()
	wantSavings, wantLending := accruedPenceByFamily(t, ds)
	gotSavings, err := ds.sim.Ledger.Balance(ds.accrSavingsID)
	if err != nil {
		t.Fatalf("savings holding balance: %v", err)
	}
	gotLending, err := ds.sim.Ledger.Balance(ds.accrLendingID)
	if err != nil {
		t.Fatalf("lending holding balance: %v", err)
	}
	if gotSavings != wantSavings {
		t.Errorf("Liability:AccruedInterest balance %d != accrued pence %d", gotSavings, wantSavings)
	}
	if gotLending != wantLending {
		t.Errorf("Asset:AccruedInterest balance %d != accrued pence %d", gotLending, wantLending)
	}
}

// TestDailyAccrualMovements verifies daily accruals are visible in the ledger:
// whole pence move into the AccruedInterest holding accounts each day, and the
// month-end reversal empties them as the engine applies the month's interest,
// leaving net P&L exactly the engine's application amounts.
func TestDailyAccrualMovements(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)

	for range 5 { // mid-month
		ds.AdvanceDay()
	}
	ds.mu.Lock()
	var count int
	if err := ds.db.QueryRow(`SELECT COUNT(*) FROM movements WHERE code = $1`, codeDailyAccrual).Scan(&count); err != nil {
		t.Fatalf("query accrual movements: %v", err)
	}
	if count == 0 {
		t.Fatal("no daily accrual movements in the ledger")
	}
	assertAccrualHoldings(t, ds)
	ds.mu.Unlock()

	for range 30 { // crosses the 31 Jan application
		ds.AdvanceDay()
	}
	ds.mu.Lock()
	defer ds.mu.Unlock()
	// Holdings now carry only the new month's accruals; the January pence
	// were reversed out on application day.
	assertAccrualHoldings(t, ds)

	// Net P&L in the ledger is the engine's applied interest plus the current
	// month's accrued-to-date pence (posted daily, reversed only on
	// application): January's accruals cancelled against January's
	// application, February's still stand. Expense:Interest pays out both
	// (balance = in - out, so it goes negative).
	var appliedSavings, appliedLending luca.Amount
	for _, c := range allCustomers(t, ds) {
		for _, a := range c.Accounts {
			if a.Family == gbp.FamilySavings {
				appliedSavings += a.Interest
			} else {
				appliedLending += a.Interest
			}
		}
	}
	accruedSavings, accruedLending := accruedPenceByFamily(t, ds)
	expense, err := ds.sim.Ledger.Balance(ds.expenseInterestID)
	if err != nil {
		t.Fatalf("expense balance: %v", err)
	}
	income, err := ds.sim.Ledger.Balance(ds.incomeInterestID)
	if err != nil {
		t.Fatalf("income balance: %v", err)
	}
	if want := -(appliedSavings + accruedSavings); expense != want {
		t.Errorf("Expense:Interest balance %d != -(applied %d + accrued %d)", expense, appliedSavings, accruedSavings)
	}
	if want := -(appliedLending + accruedLending); income != want {
		t.Errorf("Income:Interest balance %d != -(applied %d + accrued %d)", income, appliedLending, accruedLending)
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
	for _, a := range firstCustomerAccounts(t, ds) {
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
		_ int64       = CustomerAccount{}.AccruedE7
		_ int64       = ManagedAccountAccruedNumerator()
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

// ManagedAccountAccruedNumerator anchors the engine-side integer accrual type.
func ManagedAccountAccruedNumerator() int64 {
	var ma gbp.ManagedAccount
	return ma.AccruedNumerator
}

// assertAccrualPersisted checks every account's ledger position for the
// closed day carries the engine's numerator and balance, and that the BoE
// reserves position carries ds.boeAccruedNumerator. Caller holds ds.mu.
func assertAccrualPersisted(t *testing.T, ds *DemoState) {
	t.Helper()
	closed := ds.currentDay.AddDate(0, 0, -1)
	for _, a := range firstCustomerAccounts(t, ds) {
		ma, ok := ds.sim.GetManagedAccount(a.LedgerAccountID)
		if !ok {
			t.Fatalf("%s: managed account missing", a.ProductName)
		}
		p, err := ds.ledger.PositionAt(a.LedgerAccountID, closed)
		if err != nil || p == nil {
			t.Fatalf("%s: position for %s: %v %v", a.ProductName, closed.Format("2006-01-02"), p, err)
		}
		if !p.Day.Equal(closed) {
			t.Errorf("%s: latest position is %s, want %s", a.ProductName, p.Day.Format("2006-01-02"), closed.Format("2006-01-02"))
		}
		if p.Accrued.Num != ma.AccruedNumerator || p.Accrued.Den != gbp.AccrualDenominator {
			t.Errorf("%s: stored accrual %d/%d != engine %d/%d", a.ProductName, p.Accrued.Num, p.Accrued.Den, ma.AccruedNumerator, gbp.AccrualDenominator)
		}
		if p.Balance != ma.CachedBalance {
			t.Errorf("%s: stored balance %d != engine %d", a.ProductName, p.Balance, ma.CachedBalance)
		}
		if a.AccruedE7 != int64(accrualPoundsE7(ma.AccruedNumerator)) {
			t.Errorf("%s: view accrual %d != engine's %d at 7dp", a.ProductName, a.AccruedE7, accrualPoundsE7(ma.AccruedNumerator))
		}
	}
	p, err := ds.ledger.PositionAt(ds.boeReservesID, closed)
	if err != nil || p == nil {
		t.Fatalf("BoE reserves position: %v %v", p, err)
	}
	if p.Accrued.Num != ds.boeAccruedNumerator {
		t.Errorf("stored BoE numerator %d != state %d", p.Accrued.Num, ds.boeAccruedNumerator)
	}
	// The shadow table the previous release reads on a rollback keeps pace.
	var boe int64
	if err := ds.db.QueryRow(`SELECT numerator FROM accrual_state WHERE account_id = $1`, boeAccrualKey).Scan(&boe); err != nil {
		t.Fatalf("query BoE accrual row: %v", err)
	}
	if boe != ds.boeAccruedNumerator {
		t.Errorf("shadow BoE numerator %d != state %d", boe, ds.boeAccruedNumerator)
	}
}

// TestAccrualStatePersisted verifies the daily pass projects every
// account's position (mid-month and across a month-end application) so
// the ledger alone carries the balance and accrued-but-unapplied interest.
func TestAccrualStatePersisted(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)

	for range 5 { // mid-month: pure accrual, nothing applied
		ds.AdvanceDay()
	}
	ds.mu.Lock()
	assertAccrualPersisted(t, ds)
	ds.mu.Unlock()

	for range 30 { // crosses the 31 Jan month end: numerators drop to remainders
		ds.AdvanceDay()
	}
	ds.mu.Lock()
	assertAccrualPersisted(t, ds)
	ds.mu.Unlock()
}

// TestAccrualStateRestored verifies refreshFromLedger rehydrates in-memory
// numerators (engine and BoE) and account mirrors from the ledger's positions.
func TestAccrualStateRestored(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	for range 5 {
		ds.AdvanceDay()
	}

	ds.mu.Lock()
	defer ds.mu.Unlock()

	wantBoe := ds.boeAccruedNumerator
	want := make(map[string]int64)
	var total int64
	accounts := firstCustomerAccounts(t, ds)
	ds.simMu.Lock()
	for _, a := range accounts {
		ma, ok := ds.sim.GetManagedAccount(a.LedgerAccountID)
		if !ok {
			ds.simMu.Unlock()
			t.Fatalf("%s: managed account missing", a.ProductName)
		}
		want[a.LedgerAccountID] = ma.AccruedNumerator
		total += ma.AccruedNumerator
		ma.AccruedNumerator = 0 // simulate lost in-memory state
	}
	ds.simMu.Unlock()
	ds.boeAccruedNumerator = 0
	if total <= 0 {
		t.Fatal("no accrual to restore — test would be vacuous")
	}

	ds.refreshFromLedger()

	if ds.boeAccruedNumerator != wantBoe {
		t.Errorf("BoE numerator not restored: got %d, want %d", ds.boeAccruedNumerator, wantBoe)
	}
	for _, a := range firstCustomerAccounts(t, ds) {
		ma, _ := ds.sim.GetManagedAccount(a.LedgerAccountID)
		if ma.AccruedNumerator != want[a.LedgerAccountID] {
			t.Errorf("%s: numerator not restored: got %d, want %d", a.ProductName, ma.AccruedNumerator, want[a.LedgerAccountID])
		}
		if a.Accrued != ma.AccruedInterest() {
			t.Errorf("%s: mirror Accrued %d != engine %d", a.ProductName, a.Accrued, ma.AccruedInterest())
		}
	}
}

// TestBoEInterestInLedger verifies BoE reserve interest is modelled in
// accounts: income recognised daily into an accrual receivable, moved into
// Asset:BoEReserves at month end, with the sub-penny remainder carried in the
// numerator (invariant: posted pence == floor(numerator/denominator)).
func TestBoEInterestInLedger(t *testing.T) {
	ds := NewDemoState()
	for range 8 { // savings-heavy book so excess cash accrues BoE interest
		addFundedCustomer(ds)
	}

	assertBoE := func(applied bool) {
		t.Helper()
		if ds.boePostedPence != ds.boeAccruedNumerator/gbp.AccrualDenominator {
			t.Errorf("posted pence %d != floor(numerator) %d", ds.boePostedPence, ds.boeAccruedNumerator/gbp.AccrualDenominator)
		}
		holding, err := ds.sim.Ledger.Balance(ds.accrBoEID)
		if err != nil {
			t.Fatalf("holding balance: %v", err)
		}
		if holding != luca.Amount(ds.boePostedPence) {
			t.Errorf("Asset:AccruedInterest:BoE balance %d != posted pence %d", holding, ds.boePostedPence)
		}
		reserves, err := ds.sim.Ledger.Balance(ds.boeReservesID)
		if err != nil {
			t.Fatalf("reserves balance: %v", err)
		}
		if reserves != ds.boeInterestApplied {
			t.Errorf("Asset:BoEReserves balance %d != applied %d", reserves, ds.boeInterestApplied)
		}
		income, err := ds.sim.Ledger.Balance(ds.incomeBoEID)
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
// numerator and posted-pence from accrual_state, applied from the ledger.
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
