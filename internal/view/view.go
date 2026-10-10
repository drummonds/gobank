// Package view is whose eyes a balance is shown through (ADR-0008). The
// core speaks the bank's view, in which a savings balance is a liability
// and negative; the customer's view shows savings and loans both positive.
// The staff web defaults to the bank's view and lets a session switch;
// the customer app is the customer's view.
package view

import (
	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/core"
)

// View is one of the two.
type View string

const (
	Bank     View = "bank"
	Customer View = "customer"
)

// Parse reads a view name; anything else is the bank's view.
func Parse(s string) View {
	if s == string(Customer) {
		return Customer
	}
	return Bank
}

// Label is the view as a switch names it.
func (v View) Label() string {
	if v == Customer {
		return "Customer view"
	}
	return "Bank view"
}

// Other is the view to switch to.
func (v View) Other() View {
	if v == Customer {
		return Bank
	}
	return Customer
}

// Money is a signed amount on an account of the family as the view shows
// it: the bank's as it is, the customer's with savings negated so a
// holding reads positive.
func (v View) Money(amount luca.Amount, family string) luca.Amount {
	if v == Customer && family == "Savings" {
		return -amount
	}
	return amount
}

// Balance is an account's balance as the view shows it.
func (v View) Balance(a core.Account) luca.Amount { return v.Money(a.Balance, a.Family) }

// Interest is an account's applied interest as the view shows it.
func (v View) Interest(a core.Account) luca.Amount { return v.Money(a.Interest, a.Family) }

// AccruedE7 is an account's accrual as the view shows it.
func (v View) AccruedE7(a core.Account) int64 {
	if v == Customer && a.Family == "Savings" {
		return -a.AccruedE7
	}
	return a.AccruedE7
}

// Running is a statement line's running balance as the view shows it.
func (v View) Running(tx core.Transaction) luca.Amount { return v.Money(tx.Balance, tx.Family) }

// NetPosition is a customer's position with the bank over their accounts,
// in the view's sign: in the bank's view the sum of the signed balances
// (negative when the bank owes the customer net), in the customer's the
// same negated (positive when the bank owes them).
func (v View) NetPosition(accounts []core.Account) luca.Amount {
	var net luca.Amount
	for _, a := range accounts {
		net += a.Balance
	}
	if v == Customer {
		return -net
	}
	return net
}
