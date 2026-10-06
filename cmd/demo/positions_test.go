package main

import (
	"git.bytestone.uk/hum3/gobank/bank/customers"
	"testing"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/core"
)

// Funding a new customer is an event like any other: the new account's
// position for today and the equity account's are rewritten at once, not
// left to the day's pass.
func TestFundingProjectsAtOnce(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	var funded luca.Amount
	for _, a := range firstCustomerAccounts(t, ds) {
		if a.Balance == 0 {
			continue
		}
		funded += a.Balance
		p, err := ds.Ledger().PositionAt(a.LedgerAccountID, ds.position().Day)
		if err != nil || p == nil || !p.Day.Equal(ds.position().Day) || p.Balance != a.Balance {
			t.Errorf("%s: position right after funding = %+v, %v; want today at %d", a.ProductName, p, err, a.Balance)
		}
	}
	p, err := ds.Ledger().PositionAt(ds.Ledger().Chart.EquityCapital, ds.position().Day)
	if err != nil || p == nil || p.Balance != -funded {
		t.Errorf("equity position = %+v, %v; want today at %d", p, err, -funded)
	}
}

// A transfer is an event that rewrites the projections of the accounts it
// touches: the ledger's position for today moves at once, and the customer
// read model, which reads the live position, shows it without waiting for
// the day's pass.
func TestTransferRewritesTheDaysPositions(t *testing.T) {
	ds := NewDemoState()
	twoFundedCustomers(ds)

	from, _ := ds.customerByID("cust-001")
	to, _ := ds.customerByID("cust-002")
	fromAcc, toAcc := customers.FirstSavings(from.Accounts), customers.FirstSavings(to.Accounts)
	if fromAcc == nil || toAcc == nil || fromAcc.Balance < 1000 {
		t.Fatalf("need two funded savings accounts, got %+v and %+v", fromAcc, toAcc)
	}

	if _, err := ds.transfer(core.Transfer{From: "cust-001", To: "cust-002", Amount: 1000}); err != nil {
		t.Fatal(err)
	}

	after, _ := ds.customerByID("cust-001")
	if got := customers.FirstSavings(after.Accounts).Balance; got != fromAcc.Balance-1000 {
		t.Errorf("payer balance %d, want %d from the live position", got, fromAcc.Balance-1000)
	}
	for _, c := range []struct {
		id   string
		want int64
	}{{fromAcc.LedgerAccountID, int64(fromAcc.Balance) - 1000}, {toAcc.LedgerAccountID, int64(toAcc.Balance) + 1000}} {
		p, err := ds.Ledger().PositionAt(c.id, ds.position().Day)
		if err != nil || p == nil {
			t.Fatalf("position %s: %v %v", c.id, p, err)
		}
		if !p.Day.Equal(ds.position().Day) || int64(p.Balance) != c.want {
			t.Errorf("position %s = %s %d, want today at %d", c.id, p.Day.Format("2006-01-02"), p.Balance, c.want)
		}
	}
}
