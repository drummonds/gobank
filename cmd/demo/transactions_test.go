package main

import (
	"context"
	"reflect"
	"testing"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
)

// twoSavingsCustomers returns two customers that each hold a savings
// account, creating customers until there are two.
func twoSavingsCustomers(t *testing.T, ds *DemoState) (from, to CustomerRecord) {
	t.Helper()
	for range 20 {
		ds.createCustomer()
		custs, _ := ds.customerPage(1)
		var withSavings []CustomerRecord
		for _, c := range custs {
			if firstSavingsAccount(c.Accounts) != nil {
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
	before, err := newCoreAdapter(first, "").Transactions(ctx, from.ID, 1)
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
	savings := firstSavingsAccount(cust.Accounts)
	if latest.Type != "Transfer Out" || latest.Amount != 1234 || latest.Balance != savings.Balance || latest.ProductName != savings.ProductName {
		t.Errorf("latest line = %+v; want the transfer out of 1234 leaving %d on %s", latest, savings.Balance, savings.ProductName)
	}
	if latest.Reference == "" || latest.ID == "" {
		t.Errorf("latest line has no reference or id: %+v", latest)
	}

	second := newCoreAdapter(newDemoStateOn(first.db, ""), "")
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

// How a movement on a customer's account reads on the statement: by its
// code, the account on the other side and the direction, with the
// account's product family deciding between the savings and lending words.
func TestTransactionTypeOfMovement(t *testing.T) {
	cases := []struct {
		code, counterparty string
		in                 bool
		family             gbp.ProductFamily
		want               TxType
	}{
		{luca.CodeInterestAccrual, "Expense:Interest", true, gbp.FamilySavings, TxInterestCredit},
		{luca.CodeInterestAccrual, "Income:Interest", true, gbp.FamilyLending, TxInterestDebit},
		{luca.CodeBookTransfer, "Equity:Capital", true, gbp.FamilySavings, TxDepositIn},
		{luca.CodeBookTransfer, "Equity:Capital", true, gbp.FamilyLending, TxLoanDisbursement},
		{luca.CodeBookTransfer, "Liability:Savings:C000002:easy-access", true, gbp.FamilySavings, TxTransferIn},
		{luca.CodeBookTransfer, "Liability:Savings:C000002:easy-access", false, gbp.FamilySavings, TxTransferOut},
	}
	for _, c := range cases {
		if got := txTypeOf(c.code, c.counterparty, c.in, c.family); got != c.want {
			t.Errorf("txTypeOf(%s, %s, in=%v, %v) = %s, want %s", c.code, c.counterparty, c.in, c.family, got, c.want)
		}
	}
}
