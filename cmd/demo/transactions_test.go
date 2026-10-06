package main

import (
	"context"
	"git.bytestone.uk/hum3/gobank/bank/customers"
	"reflect"
	"testing"

	"git.bytestone.uk/hum3/gobank/core"
)

// twoSavingsCustomers returns two customers that each hold a savings
// account, creating customers until there are two.
func twoSavingsCustomers(t *testing.T, ds *DemoState) (from, to customers.Record) {
	t.Helper()
	for range 20 {
		ds.createCustomer()
		custs, _ := ds.customerPage(1)
		var withSavings []customers.Record
		for _, c := range custs {
			if customers.FirstSavings(c.Accounts) != nil {
				withSavings = append(withSavings, c)
			}
		}
		if len(withSavings) >= 2 {
			return withSavings[0], withSavings[1]
		}
	}
	t.Fatal("no two customers with savings accounts")
	return
}

// A customer's transactions are a projection of the ledger (ADR-0002
// stage 2): every line is a movement on one of the customer's accounts
// with the balance it left, so a restart over the same database shows the
// same transactions.
func TestTransactionsAreTheLedgersAndSurviveRestart(t *testing.T) {
	first := NewDemoState()
	from, to := twoSavingsCustomers(t, first)
	for range 40 { // crosses a month end, so interest has been applied
		first.advanceDay()
	}
	if _, err := first.transfer(core.Transfer{From: from.ID, To: to.ID, Amount: 1234}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	before, err := first.Bank.Transactions(ctx, from.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]bool{}
	for _, tx := range before.Entries {
		types[tx.Type] = true
	}
	for _, want := range []string{"Deposit", "Interest", "Transfer Out"} {
		if !types[want] {
			t.Errorf("transactions lack a %q line: %+v", want, before.Entries)
		}
	}
	latest := before.Entries[0]
	cust, _ := first.customerByID(from.ID)
	savings := customers.FirstSavings(cust.Accounts)
	if latest.Type != "Transfer Out" || latest.Amount != 1234 || latest.Balance != savings.Balance || latest.ProductName != savings.ProductName {
		t.Errorf("latest line = %+v; want the transfer out of 1234 leaving %d on %s", latest, savings.Balance, savings.ProductName)
	}
	if latest.Reference == "" || latest.ID == "" {
		t.Errorf("latest line has no reference or id: %+v", latest)
	}

	second := newDemoStateOn(first.db, "").Bank
	after, err := second.Transactions(ctx, from.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("transactions after restart:\n%+v\nbefore:\n%+v", after, before)
	}
	in, err := second.Transactions(ctx, to.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := in.Entries[0]; got.Type != "Transfer In" || got.Amount != 1234 || got.Reference != latest.Reference {
		t.Errorf("payee's latest line = %+v; want the transfer in under %s", got, latest.Reference)
	}
}
