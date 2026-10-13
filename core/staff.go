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
	RiskFreeBps      int64       // the risk-free rate in force: GBP's market rate, basis points (ADR-0009)
	BoEInterest      luca.Amount // interest earned on reserves to date
	NIMBps           float64     // latest net interest margin, annualised basis points
	DayComplete      bool        // every account has its position for Day: the start-of-day pass is done
	GL               GeneralLedger
}

// GeneralLedger is the general ledger as the bank reports it (ADR-0005):
// the last day it has closed, which the bank's figures will be as of,
// and whether that close reconciled to the sub-ledger.
type GeneralLedger struct {
	PostedThrough time.Time // zero before the first close
	Breaks        int       // products whose control disagreed with the sub-ledger at the close
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

// RatePoint is the risk-free rate in force at the end of one day, in
// basis points.
type RatePoint struct {
	Date time.Time
	Bps  int64
}

// History is the bank's daily series since it opened, oldest first.
type History struct {
	Balances    []BalancePoint
	Customers   []CustomerPoint
	NIM         []NIMPoint
	RiskFree    []RatePoint
	SavingsRate []RatePoint // the bank's savings rate from its policy, on the days it had one (story 1.8.9)
	LendingRate []RatePoint
}

// Owed is what the bank has still to do for the instant its clock is at
// (ADR-0010), read from the record of events (ADR-0011): the roots due
// by the instant whose cascades have not finished, with the days the
// clock has moved on to that the bank has not started and the accounts
// the day's pass has still to visit, whose facts are their positions.
// The bank is at rest when every count is zero, and only then may the
// simulation's clock move to the next event; the console shows the
// counts, and a drill reads them to know a deploy has settled.
type Owed struct {
	Days        int // business days the bank is behind its clock: days still to start
	Unprojected int // accounts without a position for the bank's day: the pass has them still to visit
	Events      int // root events due by the clock's instant and not done: cascades in progress
}

// AtRest reports that nothing is owed.
func (o Owed) AtRest() bool {
	return o.Days == 0 && o.Unprojected == 0 && o.Events == 0
}

// EventRecord is one event on the bank's record (ADR-0011): something
// that happened at an instant, raised by someone, whose cascade is
// pending or done.
type EventRecord struct {
	ID     string
	Kind   string // "start of day", "publication", "payment", "setting", "opening"
	At     time.Time
	Status string // "pending", "done"
	By     string // who raised it: a user, a customer, a market's name, "clock"
	DoneAt time.Time
}

// Cascade is a root event with what its cascade has written so far:
// the postings, settings and payments carrying it as their cause. The
// pass's positions are not counted: they are the day's facts, not
// records of their own.
type Cascade struct {
	Event    EventRecord
	Postings int
	Settings int
	Payments int
}

// EventQueries is the record of events as staff read it.
type EventQueries interface {
	// Cascades is the latest root events, newest first, each with what
	// its cascade has written.
	Cascades(ctx context.Context) ([]Cascade, error)
}

// BookQueries is the ledger as staff read it: the bank as a whole.
type BookQueries interface {
	Position(ctx context.Context) (Position, error)
	ProfitAndLoss(ctx context.Context) (ProfitAndLoss, error)
	BalanceSheet(ctx context.Context) (BalanceSheet, error)
	History(ctx context.Context) (History, error)
	// Owed is what the bank has still to do for the instant its clock is
	// at; at rest when every count is zero.
	Owed(ctx context.Context) (Owed, error)
}

// Movement is one ledger movement as written (ADR-0008): a signed amount
// from one account to another, the to account's balance rising by it and
// the from account's falling, with the balance each is left with, in the
// bank's view.
type Movement struct {
	ID            string
	From, To      Leg
	Amount        luca.Amount
	Currency      string
	Code          string
	ValueTime     time.Time
	KnowledgeTime time.Time
	Description   string
	// Cause is the root event a rule's posting was made under (ADR-0011)
	// and Step the posting's name within it; nil and empty for a movement
	// that is not a rule's, or from before the record.
	Cause *EventRecord
	Step  string
}

// Leg is one side of a movement: the account and the balance it was left
// with, signed as the ledger holds it.
type Leg struct {
	AccountID string
	Path      string // "Liability:Savings:cust-001:easy-access"
	Type      string // the path's first segment: Asset, Liability, Equity, Income, Expense
	Balance   luca.Amount
}

// LedgerQueries is the ledger as staff read a single movement.
type LedgerQueries interface {
	// Movement is one movement by its ID; an unknown ID is ErrNotFound.
	Movement(ctx context.Context, id string) (Movement, error)
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

// Product is one product in the catalogue with its book (ADR-0006): an
// instance of a rules version, with its own identity.
type Product struct {
	ID            string
	Name          string
	Rules         string // the rules version it runs on: "easy-access" v1
	RulesVersion  int
	Carried       bool   // whether this build carries the rules: only then do its accounts run
	Family        string // "Savings" or "Lending"
	Currency      string // ISO 4217 code of the product's money, e.g. "GBP"
	RateBps       int64  // the rate the rules publish, in basis points; zero when derived or not carried
	Parameters    []Parameter
	Description   string
	Adopted       time.Time   // the business day it went on sale
	ModuleVersion string      // the gobank-products release that carried its rules at adoption
	OnSale        bool        // whether new accounts open on it
	Accounts      int         // accounts holding it
	Balance       luca.Amount // the product's combined balance
}

// Parameter is one of a version's declared parameters as published.
type Parameter struct {
	Key     string
	Scope   string // market, bank, version or account
	Kind    string
	Value   string // the published value, or the derivation in words
	Derived bool   // derived by code from another parameter: not set, only read
}

// Setting is a parameter setting on record (ADR-0006): a value at a
// scope for its owner, effective from an instant, decided at a moment by
// someone.
type Setting struct {
	Scope     string // market, bank, version (a product's) or account
	Owner     string // the currency, "bank", the product's ID or the account's
	Key       string
	Value     string
	Effective time.Time
	Decided   time.Time
	By        string
}

// BankOwner is the owner of the bank's own parameters, its policy's, as
// the pages address them: products/bank/parameters/{key}.
const BankOwner = "bank"

// ParameterDetail is one parameter as its own page shows it (ADR-0006):
// whose it is, what its declaration says, the value in force at the
// clock's instant as known now, and the settings decided for it and not
// yet in force.
type ParameterDetail struct {
	Owner       string // the product's ID, the currency or "bank"
	Family      string // the product's family, for a product's; "" for a market's or the bank's
	Scope       string // market, bank, version or account
	Key         string
	Kind        string
	Description string
	Derived     bool   // derived by code from another parameter: not set, only read
	Derivation  string // the derivation in words, for a derived one
	InForce     string // the value effective now as the bank knows it; "" when there is none yet
	Settable    bool   // staff may set it: a carried product's own or the policy's published, not derived
	Upcoming    []Setting
}

// ProductQueries is the products component as staff read it.
type ProductQueries interface {
	Products(ctx context.Context) ([]Product, error)
	// UpcomingSettings is every setting effective after the bank's
	// clock's instant, soonest first: what is decided and not yet in
	// force, each an event the simulation's timeline carries (ADR-0010).
	UpcomingSettings(ctx context.Context) ([]Setting, error)
	// Parameter is one parameter by its owner (a product's ID, a
	// currency with a market, or "bank" for the policy's) and key; one
	// the bank does not know is ErrNotFound.
	Parameter(ctx context.Context, owner, key string) (ParameterDetail, error)
}

// Market is a currency's market as the bank knows it (ADR-0009): who
// publishes its risk-free rate, the rate in force, and the next decision
// day on the calendar the market has issued, at which it publishes a
// change or nothing.
type Market struct {
	Currency     string
	Name         string // "Bank of England"
	Rate         string // "Bank Rate"
	Page         string
	RateBps      int64     // the rate the bank's day runs on, in force by its end (as Position.RiskFreeBps); zero before the first publication
	NextDecision time.Time // the next instant the market has said it will publish; zero when its calendar runs out
}

// MarketQueries is the market component as staff read it.
type MarketQueries interface {
	Markets(ctx context.Context) ([]Market, error)
}

// Policy is the bank's policy as the bank tab shows it (ADR-0006,
// ADR-0007): its name and version, and the parameters it declares at the
// bank scope, each with its published value or its derivation in words
// and the value in force at the clock's instant.
type Policy struct {
	Name       string
	Version    int
	Parameters []PolicyParameter
}

// PolicyParameter is one of the policy's bank-scoped parameters.
type PolicyParameter struct {
	Parameter
	InForce string // the value in force at the clock's instant as the bank knows it; "" when it cannot be resolved
}

// PolicyQueries is the bank's policy as staff read it.
type PolicyQueries interface {
	Policy(ctx context.Context) (Policy, error)
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
	MarketQueries
	PolicyQueries
	TreasuryQueries
	LedgerQueries
	EventQueries
}
