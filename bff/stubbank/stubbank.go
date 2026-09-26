// Package stubbank is an in-memory Bank and Authenticator for developing the
// BFF and the apps before the banking core is extracted from cmd/demo. It
// holds a few fixed customers with generated history. Never deploy it.
package stubbank

import (
	"context"
	"crypto/subtle"
	"fmt"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/bff"
)

// PerPage is the transaction page size.
const PerPage = 20

type customer struct {
	bff.Customer
	password string
	accounts []bff.Account
	txs      []bff.Transaction // newest first
}

// Bank is the stub. It implements bff.Bank and bff.Authenticator.
type Bank struct {
	customers map[string]*customer
}

// New returns a stub with two customers: cust-001 (Alice Example, savings and
// a loan) and cust-002 (Bob Example, one savings account). Every password is
// "password".
func New() *Bank {
	b := &Bank{customers: map[string]*customer{}}
	b.add("cust-001", "Alice Example", []bff.Account{
		{ProductName: "Easy Saver", Family: "Savings", Rate: 0.035, SortCode: "04-00-04", AccountNum: "10000001", OpenDate: "2025-01-06"},
		{ProductName: "Fixed Rate Bond", Family: "Savings", Rate: 0.0425, SortCode: "04-00-04", AccountNum: "10000002", OpenDate: "2025-03-03"},
		{ProductName: "Personal Loan", Family: "Lending", Rate: 0.069, SortCode: "04-00-04", AccountNum: "20000001", OpenDate: "2025-05-12"},
	})
	b.add("cust-002", "Bob Example", []bff.Account{
		{ProductName: "Easy Saver", Family: "Savings", Rate: 0.035, SortCode: "04-00-04", AccountNum: "10000003", OpenDate: "2025-07-01"},
	})
	return b
}

func (b *Bank) add(id, name string, accts []bff.Account) {
	c := &customer{Customer: bff.Customer{ID: id, Name: name}, password: "password"}
	start := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	var all []bff.Transaction
	txID := 1
	for i := range accts {
		a := &accts[i]
		a.Index = i
		var bal luca.Amount
		post := func(day int, typ, ref string, amt luca.Amount) {
			bal += amt
			if typ == "Transfer Out" || typ == "Loan Interest" {
				amt = -amt
			}
			all = append(all, bff.Transaction{ID: txID, Date: start.AddDate(0, 0, day).Format("2006-01-02"),
				ProductName: a.ProductName, Type: typ, Reference: ref, Amount: abs(amt), Balance: bal})
			txID++
		}
		if a.Family == "Lending" {
			post(0, "Loan", "Drawdown", 500000)
			for m := 1; m <= 6; m++ {
				post(m*30, "Loan Interest", "Monthly interest", 2875)
				post(m*30+1, "Transfer In", "Repayment", -25000)
			}
		} else {
			post(0, "Deposit", "Opening deposit", 250000*luca.Amount(i+1))
			for m := 1; m <= 6; m++ {
				post(m*30, "Interest", "Monthly interest", 729*luca.Amount(i+1))
				if m%2 == 0 {
					post(m*30+3, "Transfer Out", "To current account", 10000)
				}
			}
		}
		a.Balance = bal
		a.Interest = 12 * luca.Amount(i+1)
	}
	// newest first
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	c.accounts = accts
	c.txs = all
	b.customers[id] = c
}

func abs(a luca.Amount) luca.Amount {
	if a < 0 {
		return -a
	}
	return a
}

// Authenticate implements bff.Authenticator.
func (b *Bank) Authenticate(_ context.Context, customerID, password string) (bff.Customer, error) {
	c, ok := b.customers[customerID]
	if !ok {
		// Compare anyway so timing does not reveal whether the ID exists.
		subtle.ConstantTimeCompare([]byte(password), []byte("password"))
		return bff.Customer{}, bff.ErrBadCredentials
	}
	if subtle.ConstantTimeCompare([]byte(password), []byte(c.password)) != 1 {
		return bff.Customer{}, bff.ErrBadCredentials
	}
	return c.Customer, nil
}

// Customer implements bff.Bank.
func (b *Bank) Customer(_ context.Context, id string) (bff.Customer, error) {
	c, ok := b.customers[id]
	if !ok {
		return bff.Customer{}, bff.ErrNotFound
	}
	return c.Customer, nil
}

// Accounts implements bff.Bank.
func (b *Bank) Accounts(_ context.Context, id string) ([]bff.Account, error) {
	c, ok := b.customers[id]
	if !ok {
		return nil, bff.ErrNotFound
	}
	return append([]bff.Account(nil), c.accounts...), nil
}

// Transactions implements bff.Bank.
func (b *Bank) Transactions(_ context.Context, id string, page int) (bff.TransactionPage, error) {
	c, ok := b.customers[id]
	if !ok {
		return bff.TransactionPage{}, bff.ErrNotFound
	}
	return paginate(c.txs, page), nil
}

// AccountTransactions implements bff.Bank.
func (b *Bank) AccountTransactions(_ context.Context, id string, index, page int) (bff.TransactionPage, error) {
	c, ok := b.customers[id]
	if !ok || index < 0 || index >= len(c.accounts) {
		return bff.TransactionPage{}, bff.ErrNotFound
	}
	name := c.accounts[index].ProductName
	var txs []bff.Transaction
	for _, t := range c.txs {
		if t.ProductName == name {
			txs = append(txs, t)
		}
	}
	return paginate(txs, page), nil
}

func paginate(txs []bff.Transaction, page int) bff.TransactionPage {
	if page < 1 {
		page = 1
	}
	from := (page - 1) * PerPage
	to := min(from+PerPage, len(txs))
	p := bff.TransactionPage{Page: page, PerPage: PerPage, Total: len(txs)}
	if from < len(txs) {
		p.Entries = append([]bff.Transaction(nil), txs[from:to]...)
	}
	return p
}

var _ bff.Bank = (*Bank)(nil)
var _ bff.Authenticator = (*Bank)(nil)

// String describes the stub for the startup log.
func (b *Bank) String() string { return fmt.Sprintf("stubbank (%d customers)", len(b.customers)) }
