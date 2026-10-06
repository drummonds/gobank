package main

import (
	"context"
	"testing"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
)

// Opening a customer is a command (ADR-0002 stage 4, story a): through it
// alone the customer is registered, each account is funded by a payment on
// record and has its position for the day it opens, and the loan is kept
// within the bank's lending headroom.
func TestOpenCustomerFundsAndProjectsOnTheOpeningDay(t *testing.T) {
	ds := NewDemoState()
	defer ds.db.Close()
	bank := newCoreAdapter(ds, "")
	ctx := context.Background()
	day := ds.position().Day

	const deposit, loanAsked luca.Amount = 1_000_00, 2_000_00
	rec, err := bank.OpenCustomer(ctx, core.NewCustomer{
		KYC:      core.KYC{Verified: true, LastCheck: day, RiskRating: "Standard"},
		PII:      core.PII{Name: "Ada Lovelace", NI: "AB123456C", DOB: "1815-12-10", Address: "12 St James's Square, London", Email: "ada@example.com", Phone: "07000 000001"},
		Accounts: []core.NewAccount{{ProductID: gbp.EasyAccess().ID, Opening: deposit}, {ProductID: gbp.PersonalLoan().ID, Opening: loanAsked}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != core.CustomerID(1) {
		t.Errorf("first customer is %s, want %s", rec.ID, core.CustomerID(1))
	}

	// The loan is trimmed to the headroom the deposit opens up.
	wantLoan := luca.Amount(float64(deposit) * (1 - defaultReserveRatio))
	if len(rec.Accounts) != 2 || rec.Accounts[0].Balance != deposit || rec.Accounts[1].Balance != wantLoan {
		t.Fatalf("accounts %+v; want the deposit %d and the loan trimmed to %d", rec.Accounts, deposit, wantLoan)
	}

	cust, ok := ds.customerByID(rec.ID)
	if !ok {
		t.Fatal("opened customer not on the register")
	}
	for _, a := range cust.Accounts {
		pos, err := ds.ledger.PositionAt(a.LedgerAccountID, day)
		if err != nil || pos == nil {
			t.Fatalf("%s: no position on the opening day: %v %v", a.ProductName, pos, err)
		}
		if pos.Balance != a.Balance || pos.Accrued.Num != int64(a.Balance)*gbp.RateBps(a.Rate) {
			t.Errorf("%s: position %+v; want balance %d accruing a day at %v", a.ProductName, pos, a.Balance, a.Rate)
		}
	}
	if n := ds.paymentCount(); n != 2 {
		t.Errorf("%d payments on record, want the deposit and the loan", n)
	}
	if ds.position().Lending != wantLoan || ds.position().Savings != deposit {
		t.Errorf("book %+v; want savings %d, lending %d", ds.position(), deposit, wantLoan)
	}
	if pending, _ := anyUnprojected(ds.db, day); pending {
		t.Error("the day's pass has work left after the opening: the new accounts should be projected")
	}
}

// A lending-only opening with no deposits on the books lends nothing: the
// account opens empty and no loan payment is raised.
func TestOpenCustomerLendsNothingWithoutHeadroom(t *testing.T) {
	ds := NewDemoState()
	defer ds.db.Close()
	bank := newCoreAdapter(ds, "")
	rec, err := bank.OpenCustomer(context.Background(), core.NewCustomer{
		PII:      core.PII{Name: "No Headroom"},
		Accounts: []core.NewAccount{{ProductID: gbp.Mortgage().ID, Opening: 50_000_00}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Accounts) != 1 || rec.Accounts[0].Balance != 0 {
		t.Errorf("accounts %+v; want one empty lending account", rec.Accounts)
	}
	if n := ds.paymentCount(); n != 0 {
		t.Errorf("%d payments on record, want none", n)
	}
}
