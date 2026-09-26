package main

import (
	"testing"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
)

// A created customer is readable back from the database: identity and KYC
// from the customer store, accounts from the account register, balances and
// accrual from the products engine.
func TestCustomerReadableFromDatabase(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)

	cust, ok := ds.customerByID("cust-001")
	if !ok {
		t.Fatal("cust-001 not found")
	}
	if cust.ID != "cust-001" || cust.JoinDate.IsZero() || !cust.KYCStatus.Verified || cust.KYCStatus.RiskRating == "" {
		t.Errorf("customer identity/KYC not loaded: %+v", cust)
	}
	if len(cust.Accounts) == 0 {
		t.Fatal("no accounts loaded")
	}
	for i, a := range cust.Accounts {
		if a.ProductID == "" || a.ProductName == "" || a.Family == "" || a.Rate == 0 ||
			a.SortCode == "" || a.AccountNum == "" || a.LedgerAccountID == "" || a.OpenDate.IsZero() {
			t.Errorf("account %d incomplete: %+v", i, a)
		}
		ma, ok := ds.sim.GetManagedAccount(a.LedgerAccountID)
		if !ok {
			t.Fatalf("account %d: no managed account %s", i, a.LedgerAccountID)
		}
		if a.Balance != ma.CachedBalance {
			t.Errorf("account %d: balance %d != engine %d", i, a.Balance, ma.CachedBalance)
		}
		if a.Family == gbp.FamilySavings && a.Balance <= 0 {
			t.Errorf("account %d: savings account unfunded", i)
		}
	}

	if _, ok := ds.customerByID("cust-999"); ok {
		t.Error("unknown customer reported as found")
	}
	if n := ds.customerCount(); n != 1 {
		t.Errorf("customerCount = %d, want 1", n)
	}
	page, total := ds.customerPage(1)
	if total != 1 || len(page) != 1 || page[0].ID != "cust-001" || len(page[0].Accounts) != len(cust.Accounts) {
		t.Errorf("customerPage(1) = %d customers of %d, want the one customer with %d accounts", len(page), total, len(cust.Accounts))
	}
}

// Applied interest is derived from the ledger's application movements and
// accrued-but-unapplied interest from the engine, not from a counter kept
// alongside the customer.
func TestCustomerInterestReadFromLedger(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	for range 35 { // crosses the 31 Jan month end
		ds.AdvanceDay()
	}

	cust, ok := ds.customerByID("cust-001")
	if !ok {
		t.Fatal("cust-001 not found")
	}
	for _, a := range cust.Accounts {
		if a.Balance == 0 {
			continue // unfunded loan (no lending headroom yet)
		}
		if a.Interest <= 0 {
			t.Errorf("%s: no applied interest after month end (got %d)", a.ProductName, a.Interest)
		}
		if a.Accrued <= 0 {
			t.Errorf("%s: no accrued interest into February (got %d)", a.ProductName, a.Accrued)
		}
		var ledgerApplied luca.Amount
		err := ds.db.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM movements WHERE to_account_id = $1 AND code = $2`,
			a.LedgerAccountID, luca.CodeInterestAccrual).Scan(&ledgerApplied)
		if err != nil {
			t.Fatalf("%s: query applied interest: %v", a.ProductName, err)
		}
		if a.Interest != ledgerApplied {
			t.Errorf("%s: Interest %d != ledger applied %d", a.ProductName, a.Interest, ledgerApplied)
		}
	}
}
