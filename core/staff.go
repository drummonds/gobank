package core

import (
	"context"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
)

// The staff side of the core (ADR-0002 stage 1): the whole bank as the
// staff web app sees it, one set of queries per component (ADR-0001).
// Every figure is money in integer minor units; rates are plain numbers.

// Position is the bank's position on its current business day: the book
// totals and the liquidity figures derived from them.
type Position struct {
	Day              time.Time   // the current business day
	DayCount         int         // business days since the bank opened
	Customers        int         // customers on the books
	Savings          luca.Amount // customer deposits
	Lending          luca.Amount // loans to customers
	Cash             luca.Amount // deposits less loans, held as reserves at the BoE
	RequiredReserves luca.Amount // ReserveRatio of deposits
	ReserveRatio     float64     // fraction of deposits held as reserves
	BoERate          float64     // Bank of England base rate, annual
	BoEInterest      luca.Amount // interest earned on reserves to date
	NIMBps           float64     // latest net interest margin, annualised basis points
	DayComplete      bool        // every account has its position for Day: the start-of-day pass is done
}

// ExcessCash is what the bank holds above its required reserves.
func (p Position) ExcessCash() luca.Amount { return p.Cash - p.RequiredReserves }

// ProfitAndLoss is the bank's income statement since it opened, on an
// accrual basis: interest counts as it accrues, not only when applied.
type ProfitAndLoss struct {
	DayCount               int
	LoanInterestIncome     luca.Amount
	BoEInterestIncome      luca.Amount
	DepositInterestExpense luca.Amount
	OperatingCosts         luca.Amount
}

// NetInterest is interest earned less interest paid.
func (p ProfitAndLoss) NetInterest() luca.Amount {
	return p.LoanInterestIncome + p.BoEInterestIncome - p.DepositInterestExpense
}

// NetProfit is retained after operating costs.
func (p ProfitAndLoss) NetProfit() luca.Amount { return p.NetInterest() - p.OperatingCosts }

// BalanceSheet is the bank's balance sheet on its current business day.
// Retained earnings are its only equity and so its CET1 capital; loans are
// the only risk-weighted assets (100%), gilts and cash at the BoE weigh 0%.
type BalanceSheet struct {
	DayCount           int
	Loans              luca.Amount
	Gilts              luca.Amount
	CashAtBoE          luca.Amount
	Deposits           luca.Amount
	RetainedEarnings   luca.Amount
	RiskWeightedAssets luca.Amount
}

// TotalAssets is loans plus gilts plus cash.
func (b BalanceSheet) TotalAssets() luca.Amount { return b.Loans + b.Gilts + b.CashAtBoE }

// CET1Ratio is CET1 capital over risk-weighted assets, 0 when there are none.
func (b BalanceSheet) CET1Ratio() float64 {
	if b.RiskWeightedAssets <= 0 {
		return 0
	}
	return float64(b.RetainedEarnings) / float64(b.RiskWeightedAssets)
}

// MinCET1Ratio is the Basel III minimum.
const MinCET1Ratio = 0.045

// BalancePoint is the book totals at the end of one business day.
type BalancePoint struct {
	Date    time.Time
	Savings luca.Amount
	Lending luca.Amount
}

// CustomerPoint is the customer count at the end of one business day.
type CustomerPoint struct {
	Date  time.Time
	Count int
}

// NIMPoint is the net interest margin, annualised basis points, for one day.
type NIMPoint struct {
	Date time.Time
	NIM  float64
}

// RatePoint is the BoE base rate on one day.
type RatePoint struct {
	Date time.Time
	Rate float64
}

// History is the bank's daily series since it opened, oldest first.
type History struct {
	Balances  []BalancePoint
	Customers []CustomerPoint
	NIM       []NIMPoint
	BoERate   []RatePoint
}

// BookQueries is the ledger as staff read it: the bank as a whole.
type BookQueries interface {
	Position(ctx context.Context) (Position, error)
	ProfitAndLoss(ctx context.Context) (ProfitAndLoss, error)
	BalanceSheet(ctx context.Context) (BalanceSheet, error)
	History(ctx context.Context) (History, error)
}

// KYC is a customer's Know Your Customer standing.
type KYC struct {
	Verified   bool
	LastCheck  time.Time
	RiskRating string // "Low", "Standard", "Medium"
}

// CustomerRecord is a customer as the register holds them, without PII.
type CustomerRecord struct {
	ID       string
	JoinDate time.Time
	KYC      KYC
	Accounts []Account
}

// CustomerSummary is one row of the register.
type CustomerSummary struct {
	ID       string
	Accounts int
	Savings  luca.Amount
	Lending  luca.Amount
}

// CustomerPage is one page of the register, oldest customer first.
type CustomerPage struct {
	Customers []CustomerSummary
	Page      int
	PerPage   int
	Total     int
}

// TotalPages is the number of pages the register fills, at least one.
func (p CustomerPage) TotalPages() int {
	if p.PerPage <= 0 {
		return 1
	}
	return max((p.Total+p.PerPage-1)/p.PerPage, 1)
}

// PII is a customer's personal information, behind the customer wall.
type PII struct {
	Name    string
	NI      string
	DOB     string
	Address string
	Email   string
	Phone   string
}

// CustomerInterest is the gross savings interest one customer has earned
// to date, applied plus accrued.
type CustomerInterest struct {
	CustomerID string
	Interest   luca.Amount
}

// CustomerRegister is the customers component as staff read it. The
// record carries no PII: CustomerName and CustomerPII are separate so a
// caller fetches them only once it is authorised to see them. An unknown
// customer is ErrNotFound.
type CustomerRegister interface {
	CustomerPage(ctx context.Context, page int) (CustomerPage, error)
	CustomerRecord(ctx context.Context, customerID string) (CustomerRecord, error)
	CustomerName(ctx context.Context, customerID string) (string, error)
	CustomerPII(ctx context.Context, customerID string) (PII, error)
	// SavingsInterestByCustomer lists every customer with savings interest
	// to date, oldest customer first, for the BBSI return.
	SavingsInterestByCustomer(ctx context.Context) ([]CustomerInterest, error)
}

// PaymentType is how money entered or moved.
type PaymentType string

// PaymentStatus is where a payment is in its lifecycle.
type PaymentStatus string

const (
	PaymentTransfer PaymentType = "Transfer" // customer to customer
	PaymentDeposit  PaymentType = "Deposit"  // external money in
	PaymentLoan     PaymentType = "Loan"     // the bank lends to a customer

	PaymentPending    PaymentStatus = "Pending"
	PaymentProcessing PaymentStatus = "Processing"
	PaymentCompleted  PaymentStatus = "Completed"
)

// Payment is one payment on record. From and To are customer IDs.
type Payment struct {
	ID        int
	Reference string
	Type      PaymentType
	From      string
	To        string
	Amount    luca.Amount
	Status    PaymentStatus
	CreatedAt time.Time
	SettledAt time.Time // zero until completed
}

// PaymentPage is one page of payments, newest first.
type PaymentPage struct {
	Payments []Payment
	Page     int
	PerPage  int
	Total    int
}

// TotalPages is the number of pages the payments fill, at least one.
func (p PaymentPage) TotalPages() int {
	if p.PerPage <= 0 {
		return 1
	}
	return max((p.Total+p.PerPage-1)/p.PerPage, 1)
}

// PaymentQueries is the payments component as staff read it. An unknown
// payment is ErrNotFound.
type PaymentQueries interface {
	PaymentPage(ctx context.Context, page int) (PaymentPage, error)
	Payment(ctx context.Context, id int) (Payment, error)
	// PaymentsOf lists every payment a customer sent or received, oldest
	// first. An unknown customer is ErrNotFound.
	PaymentsOf(ctx context.Context, customerID string) ([]Payment, error)
}

// Product is one product in the catalogue with its book.
type Product struct {
	ID          string
	Name        string
	Family      string // "Savings" or "Lending"
	Currency    string // ISO 4217 code of the product's money, e.g. "GBP"
	Rate        float64
	Terms       string
	Description string
	Accounts    int         // accounts open on the product
	Balance     luca.Amount // their combined balance
}

// ProductQueries is the products component as staff read it.
type ProductQueries interface {
	Products(ctx context.Context) ([]Product, error)
}

// GiltYield is the current yield on gilts of one tenor.
type GiltYield struct {
	Tenor string // e.g. "10Y"
	Rate  float64
}

// GiltHolding is a gilt the bank holds.
type GiltHolding struct {
	ID           int
	Tenor        string
	FaceValue    luca.Amount
	PurchaseDate time.Time
	Yield        float64
}

// TreasuryQueries is the treasury component as staff read it.
type TreasuryQueries interface {
	GiltYields(ctx context.Context) ([]GiltYield, error)
	GiltHoldings(ctx context.Context) ([]GiltHolding, error) // oldest first
}

// StaffQueries is everything the staff web app reads: the customer-facing
// queries too, since staff see what a customer sees and more.
type StaffQueries interface {
	CustomerQueries
	BookQueries
	CustomerRegister
	PaymentQueries
	ProductQueries
	TreasuryQueries
}
