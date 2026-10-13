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
	ErrInvalidSetting    = errors.New("core: invalid setting")
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

// DayCommands is the bank following its Clock, the one event the clock
// raises. StartDay brings the bank level with the clock's instant: a pass
// the day in progress still owes is finished first, then each day the
// clock has moved on to is started and its pass run over every account
// (ADR-0002 stage 3), one day at a time; then every ParameterChange owed
// by the instant is fired (ADR-0010: a setting that became effective
// since an account's parameters were last applied). With the clock
// still where the bank is, nothing happens. A call cut short by ctx
// leaves the rest for the next; Owed says what is left. Returns the
// business day the bank is on.
type DayCommands interface {
	StartDay(ctx context.Context) (time.Time, error)
}

// Publication is a market publishing a currency's official risk-free
// rate (ADR-0009): the new rate in basis points, effective at the instant
// it was published, which is also when it was decided. GBP's market is
// the Bank of England, publishing Bank Rate at noon London time.
type Publication struct {
	Currency string
	Bps      int64
	At       time.Time
}

// MarketCommands is the feed's one command: the simulation replaying a
// historical series, or a deployment's poller on the market's feed.
// Publish refuses a currency the bank knows no market for.
type MarketCommands interface {
	Publish(ctx context.Context, p Publication) error
}

// ParameterSetting is a staff member setting one of a product's declared
// parameters, or one the bank's policy publishes, effective from an
// instant (ADR-0006): decided now, in force from then, and an event on
// the simulation's timeline until it is (ADR-0010).
type ParameterSetting struct {
	Owner     string // the product's ID, or "bank" for the policy's
	Key       string
	Value     string
	Effective time.Time
	By        string // who decided it
}

// ProductCommands is the products component's write side. SetParameter
// refuses, with ErrInvalidSetting, a product the bank does not carry, a
// key the product's rules do not declare at the product's scope or
// derive, a key the policy does not publish, a value that is not the
// parameter's kind, and an effective instant before the bank's clock; an
// unknown owner is ErrNotFound.
type ProductCommands interface {
	SetParameter(ctx context.Context, s ParameterSetting) error
}

// Commands is every command the core accepts.
type Commands interface {
	CustomerCommands
	PaymentCommands
	TreasuryCommands
	DayCommands
	MarketCommands
	ProductCommands
}
