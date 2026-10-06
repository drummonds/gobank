package main

import (
	"context"
	"testing"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
)

// productTotal is the number of accounts on a product and their combined
// balance, as the test sums them.
type productTotal struct {
	Accounts int
	Balance  luca.Amount
}

// sumCustomerFigures totals balances and interest over every customer via
// the read model, as an independent check on the book-level derivations.
func sumCustomerFigures(t *testing.T, ds *DemoState) (savings, lending, loanInterest, depositInterest luca.Amount, perProduct map[string]productTotal) {
	t.Helper()
	perProduct = map[string]productTotal{}
	// Accrued interest is exact on the positions, so the bank's accrual-basis
	// figure truncates the family's sum once, not each account's share.
	var accruedSavingsE7, accruedLendingE7 int64
	page, total := ds.customerPage(1)
	if total != len(page) {
		t.Fatalf("test needs all customers on one page: %d of %d", len(page), total)
	}
	for _, c := range page {
		for _, a := range c.Accounts {
			pt := perProduct[a.ProductID]
			pt.Accounts++
			pt.Balance += a.Balance
			perProduct[a.ProductID] = pt
			if a.Family == gbp.FamilySavings {
				savings += a.Balance
				depositInterest += a.Interest
				accruedSavingsE7 += a.AccruedE7
			} else {
				lending += a.Balance
				loanInterest += a.Interest
				accruedLendingE7 += a.AccruedE7
			}
		}
	}
	depositInterest += poundsE7(accruedSavingsE7).Pence()
	loanInterest += poundsE7(accruedLendingE7).Pence()
	return
}

// The position's book, the P&L's interest and the products' books are
// readings of the ledger: they agree with the accounts summed one by one.
func TestBookTotalsFollowTheLedger(t *testing.T) {
	ds := NewDemoState()
	for range 5 {
		addFundedCustomer(ds)
	}
	for range 35 { // crosses a month end so interest is applied
		ds.AdvanceDay()
	}
	ds.SendPayment()

	savings, lending, loanInterest, depositInterest, perProduct := sumCustomerFigures(t, ds)
	if savings <= 0 || lending <= 0 {
		t.Fatalf("test needs funded savings and lending: %d, %d", savings, lending)
	}

	got := ds.position()
	if got.Savings != savings || got.Lending != lending {
		t.Errorf("position = savings %d lending %d, want %d, %d", got.Savings, got.Lending, savings, lending)
	}
	ledgerSavings, _, err := ds.Ledger().BalanceByPath("Liability:Savings", got.Day.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if got.Savings != ledgerSavings {
		t.Errorf("position savings %d != ledger %d", got.Savings, ledgerSavings)
	}

	income, expense := ds.interestTotals()
	if income != loanInterest || expense != depositInterest {
		t.Errorf("interestTotals = %d, %d; want loan %d deposit %d", income, expense, loanInterest, depositInterest)
	}

	gotProducts, _ := ds.Products(context.Background())
	for _, p := range gotProducts {
		if want := perProduct[p.ID]; p.Accounts != want.Accounts || p.Balance != want.Balance {
			t.Errorf("product %s = %d accounts, %d; want %+v", p.ID, p.Accounts, p.Balance, want)
		}
	}
}
