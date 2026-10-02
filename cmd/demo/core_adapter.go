package main

import (
	"context"
	"crypto/subtle"

	"git.bytestone.uk/hum3/gobank/core"
)

// coreAdapter is the demo's implementation of the core contracts
// (ADR-0002 stage 1): the BFF reaches DemoState only through it. Figures
// come from where the demo keeps them today — the register and the
// products engine for accounts, txLog for transactions — so transaction
// history is as partial as txLog is until stage 2 replaces it with a
// ledger projection.
type coreAdapter struct {
	ds *DemoState
	// password is the one app password every customer logs in with
	// (GOBANK_APP_PASSWORD); empty means app login is off. Generated
	// customers have no credentials of their own until the identity
	// component arrives with stored sessions in stage 2.
	password string
}

func newCoreAdapter(ds *DemoState, password string) *coreAdapter {
	return &coreAdapter{ds: ds, password: password}
}

// Authenticate implements core.Authenticator: the shared password, for a
// customer that exists. Unknown customers and wrong passwords fail alike.
func (a *coreAdapter) Authenticate(_ context.Context, customerID, password string) (core.Customer, error) {
	ok := a.password != "" && subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) == 1
	cust, exists := a.ds.customerByID(customerID)
	if !ok || !exists {
		return core.Customer{}, core.ErrBadCredentials
	}
	return core.Customer{ID: cust.ID, Name: a.ds.lookupName(cust.ID)}, nil
}

// Customer implements core.CustomerQueries.
func (a *coreAdapter) Customer(_ context.Context, customerID string) (core.Customer, error) {
	cust, ok := a.ds.customerByID(customerID)
	if !ok {
		return core.Customer{}, core.ErrNotFound
	}
	return core.Customer{ID: cust.ID, Name: a.ds.lookupName(cust.ID)}, nil
}

// Accounts implements core.CustomerQueries, in register order.
func (a *coreAdapter) Accounts(_ context.Context, customerID string) ([]core.Account, error) {
	cust, ok := a.ds.customerByID(customerID)
	if !ok {
		return nil, core.ErrNotFound
	}
	accts := make([]core.Account, len(cust.Accounts))
	for i, acc := range cust.Accounts {
		accts[i] = core.Account{
			Index:       i,
			ProductName: acc.ProductName,
			Family:      string(acc.Family),
			Rate:        acc.Rate,
			Balance:     acc.Balance,
			Interest:    acc.Interest,
			SortCode:    acc.SortCode,
			AccountNum:  acc.AccountNum,
			OpenDate:    acc.OpenDate.Format("2006-01-02"),
		}
	}
	return accts, nil
}

// Transactions implements core.CustomerQueries from txLog, newest first.
func (a *coreAdapter) Transactions(_ context.Context, customerID string, page int) (core.TransactionPage, error) {
	if !a.ds.customerExists(customerID) {
		return core.TransactionPage{}, core.ErrNotFound
	}
	page = max(page, 1)
	entries, total := a.ds.CustomerTransactions(customerID, page, txPerPage)
	return txPage(entries, page, total), nil
}

// AccountTransactions implements core.CustomerQueries for one account by
// its register index.
func (a *coreAdapter) AccountTransactions(_ context.Context, customerID string, index, page int) (core.TransactionPage, error) {
	cust, ok := a.ds.customerByID(customerID)
	if !ok || index < 0 || index >= len(cust.Accounts) {
		return core.TransactionPage{}, core.ErrNotFound
	}
	page = max(page, 1)
	entries, total := a.ds.ProductTransactions(customerID, index, page, txPerPage)
	return txPage(entries, page, total), nil
}

func txPage(entries []TxEntry, page, total int) core.TransactionPage {
	p := core.TransactionPage{Page: page, PerPage: txPerPage, Total: total}
	for _, tx := range entries {
		p.Entries = append(p.Entries, core.Transaction{
			ID:          tx.ID,
			Date:        tx.Date.Format("2006-01-02"),
			ProductName: tx.ProductName,
			Type:        tx.Type.String(),
			Reference:   tx.Reference,
			Amount:      tx.Amount,
			Balance:     tx.Balance,
		})
	}
	return p
}

var (
	_ core.CustomerQueries = (*coreAdapter)(nil)
	_ core.Authenticator   = (*coreAdapter)(nil)
)
