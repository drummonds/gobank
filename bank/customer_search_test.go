package bank

import (
	"fmt"
	"testing"

	"git.bytestone.uk/hum3/gobank/core"
)

// A customer is found by any part of their name, whatever the case,
// oldest first; a name nobody has, or no name at all, finds nobody.
func TestCustomersByNameFindsByAnyPartOfTheName(t *testing.T) {
	f := open(t)
	for _, c := range []core.NewCustomer{saver("Ada Lovelace", 100_00), saver("Alan Turing", 200_00), saver("Grace Hopper", 300_00)} {
		if _, err := f.bank.OpenCustomer(f.ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	found, err := f.bank.CustomersByName(f.ctx, "LOVELACE")
	if err != nil || len(found) != 1 || found[0].Accounts != 1 || found[0].Savings == 0 {
		t.Fatalf("CustomersByName(LOVELACE) = %+v, %v; want Ada's row with her one account's balance", found, err)
	}
	if name, _ := f.bank.CustomerName(f.ctx, found[0].ID); name != "Ada Lovelace" {
		t.Errorf("the match is %s, want Ada Lovelace", name)
	}
	all, err := f.bank.CustomersByName(f.ctx, "a")
	if err != nil || len(all) != 3 || all[0].ID != found[0].ID || all[2].ID <= all[1].ID {
		t.Errorf("CustomersByName(a) = %+v, %v; want all three, oldest first", all, err)
	}
	for _, name := range []string{"Babbage", "", "  "} {
		if none, err := f.bank.CustomersByName(f.ctx, name); err != nil || len(none) != 0 {
			t.Errorf("CustomersByName(%q) = %+v, %v; want nobody", name, none, err)
		}
	}
}

// The matches stop at the search limit: a name half the book shares is
// not a way to list the book.
func TestCustomersByNameIsCapped(t *testing.T) {
	f := open(t)
	for i := range core.CustomerSearchLimit + 1 {
		if _, err := f.bank.OpenCustomer(f.ctx, saver(fmt.Sprintf("Dup %d", i), 10_00)); err != nil {
			t.Fatal(err)
		}
	}
	found, err := f.bank.CustomersByName(f.ctx, "dup")
	if err != nil || len(found) != core.CustomerSearchLimit {
		t.Errorf("found %d, %v; want the limit of %d", len(found), err, core.CustomerSearchLimit)
	}
}
