package core

import (
	"context"
	"errors"

	luca "git.bytestone.uk/hum3/go-luca"
)

// The write side of the core (ADR-0002 stage 1): what an event makes the
// bank do. Today's events are a staff action in the web app and the
// simulation's payment generator; the app's own commands come with
// device-bound signing. Opening a customer stays inside the simulation's
// generator until stage 4 separates generators from the bank.

// Errors returned by the commands, besides ErrNotFound.
var (
	ErrInvalidAmount     = errors.New("core: invalid amount")
	ErrInsufficientFunds = errors.New("core: insufficient funds")
	ErrSameCustomer      = errors.New("core: transfer to the same customer")
)

// Transfer moves money from one customer's savings to another's. Each
// side is the customer's first savings account.
type Transfer struct {
	From   string // customer ID
	To     string // customer ID
	Amount luca.Amount
}

// PaymentCommands is the payments component's write side. Transfer
// returns the payment it raised. An amount that is not positive is
// ErrInvalidAmount; From equal to To is ErrSameCustomer; a customer that
// does not exist or holds no savings account is ErrNotFound; an amount
// above the sender's balance is ErrInsufficientFunds.
type PaymentCommands interface {
	Transfer(ctx context.Context, t Transfer) (Payment, error)
}

// MinGiltPurchase is the smallest face value the bank buys.
const MinGiltPurchase luca.Amount = 1000_00

// TreasuryCommands is the treasury component's write side. BuyGilt buys
// faceValue of gilts of the given tenor at today's yield: an unknown tenor
// is ErrNotFound, a face value below MinGiltPurchase is ErrInvalidAmount.
type TreasuryCommands interface {
	BuyGilt(ctx context.Context, tenor string, faceValue luca.Amount) error
}

// Commands is every command the core accepts.
type Commands interface {
	PaymentCommands
	TreasuryCommands
}
