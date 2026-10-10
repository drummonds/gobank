// Package modelbank is the Model Bank's definition (ADR-0007): the
// customisation that makes the general banking solution this bank, as
// code. Today that is its name and the product versions it carries; the
// bank's policy (its savings and lending rates) joins it in story 1.8.3,
// and the package moves to a module of its own once it does. A different
// bank is a different definition over the same capabilities.
package modelbank

import (
	easyaccess "git.bytestone.uk/hum3/gobank-products/easyaccess/v1"
	fixedterm "git.bytestone.uk/hum3/gobank-products/fixedterm/v1"
	isa "git.bytestone.uk/hum3/gobank-products/isa/v1"
	mortgage "git.bytestone.uk/hum3/gobank-products/mortgage/v1"
	overdraft "git.bytestone.uk/hum3/gobank-products/overdraft/v1"
	personalloan "git.bytestone.uk/hum3/gobank-products/personalloan/v1"
	"git.bytestone.uk/hum3/gobank/bank/definition"
	"git.bytestone.uk/hum3/gobank/bank/products"
)

// Definition is the Model Bank.
func Definition() definition.Bank {
	return definition.Bank{
		Name: "Model Bank",
		// The catalogue at this release. A rule change to a product is a
		// new version listed beside the old; a version comes off sale by
		// Withdrawn: true and leaves the list once no account runs on it.
		Products: []products.Listing{
			{Version: easyaccess.Version, Description: "Instant access savings with competitive rate"},
			{Version: fixedterm.Version, Description: "Higher rate for locking funds for 2 years"},
			{Version: isa.Version, Description: "Tax-free savings up to annual ISA allowance"},
			{Version: personalloan.Version, Description: "Unsecured personal loan for any purpose"},
			{Version: mortgage.Version, Description: "Residential mortgage with fixed rate period"},
			{Version: overdraft.Version, Description: "Arranged overdraft facility on current account"},
		},
	}
}
