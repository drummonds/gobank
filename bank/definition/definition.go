// Package definition is the shape of a bank's own definition (ADR-0007):
// what makes the general banking solution this bank. The bank is opened
// with one; the Model Bank's lives in package modelbank, and another bank
// is another definition.
package definition

import "git.bytestone.uk/hum3/gobank/bank/products"

// Bank is one bank's definition.
type Bank struct {
	Name     string             // the bank's name, as its pages show it
	Products []products.Listing // the product versions it carries, in catalogue order
}
