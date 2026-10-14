// Package definition is the shape of a bank's own definition (ADR-0007):
// what makes the general banking solution this bank. The bank is opened
// with one; the Model Bank's lives in package modelbank, and another bank
// is another definition.
package definition

import (
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/products"
	"git.bytestone.uk/hum3/gobank/core"
)

// Bank is one bank's definition.
type Bank struct {
	Name     string             // the bank's name, as its pages show it
	Products []products.Listing // its products, each on a rules version the build then carries, in catalogue order
	Policy   gbp.Policy         // its policy: the bank's own rates and how they follow the market, as versioned code (ADR-0006, ADR-0007)
	YearEnd  core.YearEnd       // the day its financial year closes; the statements name the year by it
}
