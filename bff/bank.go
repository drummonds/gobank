// Package bff is the backend-for-frontend for the Model Bank apps.
//
// It is the only server the thin clients talk to. It owns sessions and login,
// and turns banking data into screen-ready trees (package screen). Apart from
// health, the public login screen and login itself, every endpoint requires a
// session; the customer identity always comes from the session, never from
// the request.
//
// The banking core is reached through the narrow Bank and Authenticator
// interfaces, the first cut of the internal API between the BFF and the core.
package bff

import (
	"context"
	"errors"

	luca "git.bytestone.uk/hum3/go-luca"
)

// Errors returned by Bank and Authenticator implementations.
var (
	ErrNotFound       = errors.New("bff: not found")
	ErrBadCredentials = errors.New("bff: bad credentials")
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
	Rate        float64
	Balance     luca.Amount
	Interest    luca.Amount
	SortCode    string
	AccountNum  string
	OpenDate    string
}

// Transaction is one ledger entry as shown to the customer.
type Transaction struct {
	ID          int
	Date        string
	ProductName string
	Type        string // "Deposit", "Interest", "Transfer In", "Transfer Out", "Loan", "Loan Interest"
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

// Bank is the read side of the internal API to the banking core. Every method
// is scoped to one customer; a customer can never see another's data because
// no method takes anything but their own ID.
type Bank interface {
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
