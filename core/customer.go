// Package core holds the contracts of the banking core: the commands and
// queries through which everything outside the core — the BFF, the staff
// UI, the simulation's generators — reaches it (ADR-0002). The core's
// implementation lives behind these interfaces; today that is cmd/demo's
// DemoState through an adapter, and the stub in bff/stubbank.
package core

import (
	"context"
	"errors"

	luca "git.bytestone.uk/hum3/go-luca"
)

// Errors returned by the contracts.
var (
	ErrNotFound       = errors.New("core: not found")
	ErrBadCredentials = errors.New("core: bad credentials")
)

// Customer identifies a logged-in customer.
type Customer struct {
	ID   string
	Name string
}

// Account is one product held by a customer. Money is integer minor units.
type Account struct {
	Index       int // position in the customer's account list; stable for a session
	ProductName string
	Family      string // "Savings" or "Lending"
	Currency    string // ISO 4217 code of the product's money, e.g. "GBP"
	Rate        float64
	Balance     luca.Amount
	Interest    luca.Amount // applied to the balance to date
	// AccruedE7 is interest accrued but not yet applied, in ten-millionths
	// of the major unit (7 decimal places), as the ledger's position views
	// publish it.
	AccruedE7  int64
	SortCode   string
	AccountNum string
	OpenDate   string
}

// Transaction is one ledger entry as shown to the customer.
type Transaction struct {
	ID          string // the ledger movement's ID
	Date        string
	ProductName string
	Type        string // "Deposit", "Interest", "Transfer In", "Transfer Out", "Loan", "Loan Interest"
	Currency    string // the account's, ISO 4217
	Reference   string
	Amount      luca.Amount
	Balance     luca.Amount
}

// TransactionPage is one page of transactions, newest first.
type TransactionPage struct {
	Entries []Transaction
	Page    int
	PerPage int
	Total   int
}

// HasMore reports whether a later page exists.
func (p TransactionPage) HasMore() bool {
	return p.PerPage > 0 && p.Page*p.PerPage < p.Total
}

// CustomerQueries is the read side of the core as one customer sees it.
// Every method is scoped to one customer; a customer can never see
// another's data because no method takes anything but their own ID. An
// unknown customer, or an account index the customer does not hold, is
// ErrNotFound.
type CustomerQueries interface {
	Customer(ctx context.Context, customerID string) (Customer, error)
	Accounts(ctx context.Context, customerID string) ([]Account, error)
	Transactions(ctx context.Context, customerID string, page int) (TransactionPage, error)
	AccountTransactions(ctx context.Context, customerID string, index, page int) (TransactionPage, error)
}

// Authenticator checks customer credentials. It returns ErrBadCredentials
// for an unknown customer and for a wrong password alike, so the response
// cannot be used to enumerate customer IDs.
type Authenticator interface {
	Authenticate(ctx context.Context, customerID, password string) (Customer, error)
}
