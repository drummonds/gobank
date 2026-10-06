package core

import (
	"context"
	"errors"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
)

// The write side of the core (ADR-0002 stage 1): what an event makes the
// bank do. Events are a staff action in the web app, a customer's action
// through the BFF, the start of a day, and in simulation the generators'
// doings (stage 4), which enter through these same commands; the app's
// own commands come with device-bound signing.

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

// NewAccount is one account a new customer opens, with its opening money:
// a deposit into a savings product, or the amount lent on a lending
// product and disbursed to the customer. The bank lends only within its
// headroom, so a loan may be trimmed, down to nothing.
type NewAccount struct {
	ProductID string
	Opening   luca.Amount
}

// NewCustomer is a customer to open: their KYC standing, their personal
// information and the accounts they hold. The bank allocates the ID and
// dates the record on its business day.
type NewCustomer struct {
	KYC      KYC
	PII      PII
	Accounts []NewAccount
}

// CustomerCommands is the customers component's write side. OpenCustomer
// opens the customer with their accounts, funds each from the outside
// world (a deposit) or from the bank (a loan) as a payment on record, and
// returns the record as the register now holds it. No accounts, or an
// opening amount that is negative, is ErrInvalidAmount; an unknown product
// is ErrNotFound.
type CustomerCommands interface {
	OpenCustomer(ctx context.Context, c NewCustomer) (CustomerRecord, error)
}

// DayCommands is the start of a day, the one event the clock raises.
// StartDay moves the bank on to its next business day and runs the day's
// pass over every account (ADR-0002 stage 3). A pass an earlier call left
// unfinished, its ctx having ended, is resumed without the day advancing
// again. Returns the business day the bank is on.
type DayCommands interface {
	StartDay(ctx context.Context) (time.Time, error)
}

// Commands is every command the core accepts.
type Commands interface {
	CustomerCommands
	PaymentCommands
	TreasuryCommands
	DayCommands
}
