package main

import (
	"context"
	"testing"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/bank/customers"
	"git.bytestone.uk/hum3/gobank/bank/payments"
	"git.bytestone.uk/hum3/gobank/core"
)

// Shorthands for driving the simulation and reading the bank in tests.

func (ds *DemoState) createCustomer()                { ds.sim.OpenCustomer(context.Background()) }
func (ds *DemoState) advanceDay()                    { ds.sim.Step(context.Background()) }
func (ds *DemoState) nextDayCtx(ctx context.Context) { ds.sim.Step(ctx) }

// addFundedCustomer adds one customer through the real pipeline.
func addFundedCustomer(ds *DemoState) { ds.createCustomer() }

// twoFundedCustomers gives SendPayment a sender and a recipient.
func twoFundedCustomers(ds *DemoState) {
	addFundedCustomer(ds)
	addFundedCustomer(ds)
}

func (ds *DemoState) customerByID(id string) (customers.Record, bool) {
	rec, err := ds.Customers().ByID(context.Background(), id)
	return rec, err == nil
}

func (ds *DemoState) customerPage(page int) ([]customers.Record, int) {
	recs, total, _ := ds.Customers().Page(context.Background(), page, 50)
	return recs, total
}

func (ds *DemoState) customerCount() int {
	n, _ := ds.Customers().Count(context.Background())
	return n
}

func (ds *DemoState) lookupName(id string) string {
	return ds.Customers().Name(context.Background(), id)
}

func (ds *DemoState) paymentByID(id int) (payments.Payment, bool) {
	p, err := ds.Payments().ByID(context.Background(), id)
	return p, err == nil
}

func (ds *DemoState) paymentCount() int {
	n, _ := ds.Payments().Count(context.Background())
	return n
}

func (ds *DemoState) paymentPage(page int) ([]payments.Payment, int) {
	ps, total, _ := ds.Payments().Page(context.Background(), page, 20)
	return ps, total
}

func (ds *DemoState) paymentsOf(id string) []payments.Payment {
	ps, _ := ds.Payments().Of(context.Background(), id)
	return ps
}

func (ds *DemoState) setPaymentStatus(id int, status payments.Status, at time.Time) {
	ds.Payments().SetStatus(context.Background(), id, status, at) //nolint:errcheck
}

// interestTotals is the bank's customer interest to date: loan interest
// income and deposit interest expense, from the P&L.
func (ds *DemoState) interestTotals() (loanIncome, depositExpense luca.Amount) {
	pl, _ := ds.ProfitAndLoss(context.Background())
	return pl.LoanInterestIncome, pl.DepositInterestExpense
}

func (ds *DemoState) transfer(t core.Transfer) (core.Payment, error) {
	return ds.Transfer(context.Background(), t)
}

func anyUnprojected(ds *DemoState, day time.Time) (bool, error) {
	return ds.Catalogue().AnyUnprojected(context.Background(), day)
}

// firstCustomerAccounts reads cust-001's accounts through the read model.
func firstCustomerAccounts(t *testing.T, ds *DemoState) []customers.Account {
	t.Helper()
	cust, ok := ds.customerByID("cust-001")
	if !ok {
		t.Fatal("cust-001 not found")
	}
	return cust.Accounts
}

// allCustomers reads every customer through the read model (tests keep to
// one page).
func allCustomers(t *testing.T, ds *DemoState) []customers.Record {
	t.Helper()
	page, total := ds.customerPage(1)
	if total != len(page) {
		t.Fatalf("allCustomers: %d of %d on the first page", len(page), total)
	}
	return page
}
