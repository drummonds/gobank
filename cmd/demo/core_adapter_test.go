package main

import (
	"context"
	"errors"
	"testing"

	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/gobank/core/coretest"
)

// The demo honours the core contracts the BFF and the staff UI are written
// against: customer queries, staff queries and commands.
func TestCoreAdapterContract(t *testing.T) {
	ds := NewDemoState()
	twoFundedCustomers(ds)
	a := newCoreAdapter(ds, "secret")
	coretest.Run(t, coretest.Fixture{
		Queries: a, Auth: a, Staff: a, Commands: a,
		CustomerID: "cust-001", OtherCustomerID: "cust-002", Password: "secret",
	})
}

// With no app password configured, nobody can log in.
func TestCoreAdapterLoginOffWithoutPassword(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	a := newCoreAdapter(ds, "")
	if _, err := a.Authenticate(context.Background(), "cust-001", ""); !errors.Is(err, core.ErrBadCredentials) {
		t.Errorf("Authenticate with login off: err = %v, want ErrBadCredentials", err)
	}
}
