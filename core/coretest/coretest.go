// Package coretest is the contract test suite for the core's interfaces.
// Every implementation — the stub, the demo adapter, later the real core —
// runs the same suite, so a client written against the contract behaves
// the same on all of them.
package coretest

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"git.bytestone.uk/hum3/gobank/core"
)

// UnknownCustomerID is an ID no implementation should know.
const UnknownCustomerID = "no-such-customer"

// Fixture is an implementation under test and a customer it knows about,
// who holds at least one account with at least one transaction. Auth may be
// nil when the implementation has no Authenticator; Password is that
// customer's password when it has.
type Fixture struct {
	Queries    core.CustomerQueries
	Auth       core.Authenticator
	CustomerID string
	Password   string
}

// Run checks the implementation against the CustomerQueries contract and,
// when Auth is set, the Authenticator contract.
func Run(t *testing.T, f Fixture) {
	t.Helper()
	ctx := context.Background()
	q := f.Queries

	t.Run("customer", func(t *testing.T) {
		c, err := q.Customer(ctx, f.CustomerID)
		if err != nil {
			t.Fatalf("Customer(%q): %v", f.CustomerID, err)
		}
		if c.ID != f.CustomerID || c.Name == "" {
			t.Errorf("Customer(%q) = %+v, want that ID and a name", f.CustomerID, c)
		}
	})

	t.Run("unknown customer is ErrNotFound", func(t *testing.T) {
		_, err := q.Customer(ctx, UnknownCustomerID)
		wantNotFound(t, "Customer", err)
		_, err = q.Accounts(ctx, UnknownCustomerID)
		wantNotFound(t, "Accounts", err)
		_, err = q.Transactions(ctx, UnknownCustomerID, 1)
		wantNotFound(t, "Transactions", err)
		_, err = q.AccountTransactions(ctx, UnknownCustomerID, 0, 1)
		wantNotFound(t, "AccountTransactions", err)
	})

	t.Run("accounts are indexed in a stable order", func(t *testing.T) {
		accts, err := q.Accounts(ctx, f.CustomerID)
		if err != nil {
			t.Fatal(err)
		}
		if len(accts) == 0 {
			t.Fatal("fixture customer holds no accounts")
		}
		for i, a := range accts {
			if a.Index != i {
				t.Errorf("accounts[%d].Index = %d", i, a.Index)
			}
			if a.ProductName == "" {
				t.Errorf("accounts[%d] has no product name", i)
			}
			if a.Family != "Savings" && a.Family != "Lending" {
				t.Errorf("accounts[%d].Family = %q", i, a.Family)
			}
		}
		again, err := q.Accounts(ctx, f.CustomerID)
		if err != nil {
			t.Fatal(err)
		}
		for i := range accts {
			if i >= len(again) || again[i].ProductName != accts[i].ProductName || again[i].AccountNum != accts[i].AccountNum {
				t.Fatalf("account order changed between calls at %d", i)
			}
		}
	})

	t.Run("transactions page newest first and cover the total", func(t *testing.T) {
		first, err := q.Transactions(ctx, f.CustomerID, 1)
		if err != nil {
			t.Fatal(err)
		}
		if first.Page != 1 || first.PerPage <= 0 || first.Total <= 0 {
			t.Fatalf("page 1 = {Page %d PerPage %d Total %d}, want page 1 of a positive total", first.Page, first.PerPage, first.Total)
		}
		if want := min(first.PerPage, first.Total); len(first.Entries) != want {
			t.Errorf("page 1 has %d entries, want %d", len(first.Entries), want)
		}
		seen := 0
		page := first
		for n := 1; ; n++ {
			for i := 1; i < len(page.Entries); i++ {
				if page.Entries[i].Date > page.Entries[i-1].Date {
					t.Errorf("page %d not newest first: %s after %s", n, page.Entries[i].Date, page.Entries[i-1].Date)
				}
			}
			seen += len(page.Entries)
			if !page.HasMore() {
				break
			}
			if page, err = q.Transactions(ctx, f.CustomerID, n+1); err != nil {
				t.Fatalf("page %d: %v", n+1, err)
			}
			if page.Page != n+1 || page.Total != first.Total {
				t.Fatalf("page %d = {Page %d Total %d}", n+1, page.Page, page.Total)
			}
		}
		if seen != first.Total {
			t.Errorf("pages hold %d entries, Total says %d", seen, first.Total)
		}
		beyond, err := q.Transactions(ctx, f.CustomerID, first.Total/first.PerPage+2)
		if err != nil || len(beyond.Entries) != 0 || beyond.HasMore() {
			t.Errorf("page beyond the last = %+v, %v; want empty with no more", beyond, err)
		}
	})

	t.Run("account transactions belong to that account", func(t *testing.T) {
		accts, err := q.Accounts(ctx, f.CustomerID)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range accts {
			page, err := q.AccountTransactions(ctx, f.CustomerID, a.Index, 1)
			if err != nil {
				t.Fatalf("AccountTransactions(%d): %v", a.Index, err)
			}
			for _, tx := range page.Entries {
				if tx.ProductName != a.ProductName {
					t.Errorf("account %d (%s) lists a %s transaction", a.Index, a.ProductName, tx.ProductName)
				}
			}
		}
		for _, idx := range []int{len(accts), -1} {
			_, err := q.AccountTransactions(ctx, f.CustomerID, idx, 1)
			wantNotFound(t, fmt.Sprintf("AccountTransactions(index %d)", idx), err)
		}
	})

	if f.Auth == nil {
		return
	}
	t.Run("authenticate", func(t *testing.T) {
		c, err := f.Auth.Authenticate(ctx, f.CustomerID, f.Password)
		if err != nil || c.ID != f.CustomerID {
			t.Errorf("Authenticate(right password) = %+v, %v", c, err)
		}
		_, err = f.Auth.Authenticate(ctx, f.CustomerID, f.Password+"x")
		if !errors.Is(err, core.ErrBadCredentials) {
			t.Errorf("Authenticate(wrong password) err = %v, want ErrBadCredentials", err)
		}
		_, err = f.Auth.Authenticate(ctx, UnknownCustomerID, f.Password)
		if !errors.Is(err, core.ErrBadCredentials) {
			t.Errorf("Authenticate(unknown customer) err = %v, want ErrBadCredentials (not ErrNotFound: no enumeration)", err)
		}
	})
}

func wantNotFound(t *testing.T, what string, err error) {
	t.Helper()
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("%s for an unknown customer: err = %v, want ErrNotFound", what, err)
	}
}
