package main

import (
	"context"
	"errors"
	"testing"

	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/gobank/core/coretest"
)

// The bank honours the core contracts the BFF and the staff UI are
// written against: customer queries, staff queries and commands, with
// the app's login in front.
func TestBankHonoursTheCoreContracts(t *testing.T) {
	ds := NewDemoState()
	clock := coretest.NewFakeClock(ds.clock.Now())
	ds.clock = clock
	twoFundedCustomers(ds)
	coretest.Run(t, coretest.Fixture{
		Queries: ds.Bank, Auth: newAppLogin(ds.Bank, "secret"), Staff: ds.Bank, Commands: ds.Bank, Clock: clock,
		CustomerID: "cust-001", OtherCustomerID: "cust-002", Password: "secret",
	})
}

// With no app password configured, nobody can log in.
func TestAppLoginOffWithoutPassword(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	a := newAppLogin(ds.Bank, "")
	if _, err := a.Authenticate(context.Background(), "cust-001", ""); !errors.Is(err, core.ErrBadCredentials) {
		t.Errorf("Authenticate with login off: err = %v, want ErrBadCredentials", err)
	}
}
