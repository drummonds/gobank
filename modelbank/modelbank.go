// Package modelbank is the Model Bank's definition (ADR-0007): the
// customisation that makes the general banking solution this bank, as
// code. Its name; its products, each an instance of a rules version in
// gobank-products (ADR-0006 as amended); and its policy, how its own
// rates follow the market (policy/v1). A different bank is a different
// definition over the same capabilities.
package modelbank

import (
	"time"

	easyaccess "git.bytestone.uk/hum3/gobank-products/easyaccess/v1"
	easyaccessv2 "git.bytestone.uk/hum3/gobank-products/easyaccess/v2"
	fixedterm "git.bytestone.uk/hum3/gobank-products/fixedterm/v1"
	isa "git.bytestone.uk/hum3/gobank-products/isa/v1"
	mortgage "git.bytestone.uk/hum3/gobank-products/mortgage/v1"
	overdraft "git.bytestone.uk/hum3/gobank-products/overdraft/v1"
	personalloan "git.bytestone.uk/hum3/gobank-products/personalloan/v1"
	"git.bytestone.uk/hum3/gobank/bank/definition"
	"git.bytestone.uk/hum3/gobank/bank/products"
	"git.bytestone.uk/hum3/gobank/core"
	policy "git.bytestone.uk/hum3/gobank/modelbank/policy/v1"
)

// Definition is the Model Bank.
func Definition() definition.Bank {
	return definition.Bank{
		Name: "Isle of Alba Bank",
		// The products at this release, one on each v1 rules version and
		// named after it. A rules change is a new rules version, and a
		// product on it a new listing beside the old; a product comes off
		// sale by Withdrawn: true and leaves the list once no account holds
		// it. A new product on existing rules is a listing, not a release.
		Products: []products.Listing{
			{ID: "easy-access", Name: easyaccess.Version.Name(), Rules: easyaccess.Version, Description: "Instant access savings with competitive rate"},
			{ID: "easy-access-tracker", Name: easyaccessv2.Version.Name(), Rules: easyaccessv2.Version, Description: "Instant access savings tracking the bank's savings rate"},
			{ID: "fixed-term", Name: fixedterm.Version.Name(), Rules: fixedterm.Version, Description: "Higher rate for locking funds for 2 years"},
			{ID: "isa", Name: isa.Version.Name(), Rules: isa.Version, Description: "Tax-free savings up to annual ISA allowance"},
			{ID: "personal-loan", Name: personalloan.Version.Name(), Rules: personalloan.Version, Description: "Unsecured personal loan for any purpose"},
			{ID: "mortgage", Name: mortgage.Version.Name(), Rules: mortgage.Version, Description: "Residential mortgage with fixed rate period"},
			{ID: "overdraft", Name: overdraft.Version.Name(), Rules: overdraft.Version, Description: "Arranged overdraft facility on current account"},
		},
		Policy: policy.Version,
		// A 31 March year end, as UK banks commonly keep (Atom Bank's).
		YearEnd: core.YearEnd{Month: time.March, Day: 31},
	}
}
