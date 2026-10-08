// Package products is the bank's product catalogue and the interest rules
// run for one account at a time (ADR-0002 stage 5, story 1.5.4). The
// component keeps no state of its own: an account's balance and its
// accrued-but-unapplied interest are its ledger position (go-luca),
// written for the day by the start-of-day pass (pass.go) and again by
// every event that moves the balance. gobank-products holds the rules;
// this package runs them.
package products

import (
	"database/sql"
	"fmt"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/bank/schema"
)

// Product is a gobank-products product as the bank sells it.
type Product struct {
	*gbp.Product
	Currency    string  // ISO 4217 code of the product's money; every product is GBP today
	Rate        float64 // annual rate as a decimal
	Terms       string
	Description string
}

// Catalogue is every product the bank offers.
func Catalogue() []Product {
	return []Product{
		{Product: gbp.EasyAccess(), Currency: "GBP", Rate: 0.015, Terms: "No notice", Description: "Instant access savings with competitive rate"},
		{Product: gbp.FixedTerm(), Currency: "GBP", Rate: 0.040, Terms: "2 year fixed", Description: "Higher rate for locking funds for 2 years"},
		{Product: gbp.ISA(), Currency: "GBP", Rate: 0.035, Terms: "Annual allowance", Description: "Tax-free savings up to annual ISA allowance"},
		{Product: gbp.PersonalLoan(), Currency: "GBP", Rate: 0.069, Terms: "1-5 years", Description: "Unsecured personal loan for any purpose"},
		{Product: gbp.Mortgage(), Currency: "GBP", Rate: 0.045, Terms: "25 year", Description: "Residential mortgage with fixed rate period"},
		{Product: gbp.Overdraft(), Currency: "GBP", Rate: 0.159, Terms: "Revolving", Description: "Arranged overdraft facility on current account"},
	}
}

// Products is the catalogue over the bank's database.
type Products struct {
	db        *sql.DB
	catalogue []Product
}

// New is the catalogue on db.
func New(db *sql.DB) *Products {
	return &Products{db: db, catalogue: Catalogue()}
}

// All is the catalogue.
func (p *Products) All() []Product { return p.catalogue }

// ByID looks a product up in the catalogue.
func (p *Products) ByID(id string) (Product, bool) {
	for _, prod := range p.catalogue {
		if prod.ID == id {
			return prod, true
		}
	}
	return Product{}, false
}

// Schema: the component owns no table. accrual_state held
// accrued-but-unapplied interest before positions did (gobank ≤ v0.10);
// v0.11 stopped writing it and version 2 drops it.
var Schema = schema.Component{Name: "products", Migrations: []schema.Migration{
	{Version: 1, Statements: []string{`CREATE TABLE IF NOT EXISTS accrual_state (
		account_id VARCHAR(64) PRIMARY KEY,
		numerator BIGINT NOT NULL,
		accrued_pounds_e7 BIGINT NOT NULL DEFAULT 0,
		as_of TIMESTAMP NOT NULL
	)`}},
	{Version: 2, Statements: []string{`DROP TABLE IF EXISTS accrual_state`}},
}}

// Account is an account the day's rules run for: its ledger account and
// the product whose rules apply.
type Account struct {
	ID      string
	Product Product
}

// DayResult is what one account's daily work changed in the bank's
// totals: the interest applied to its balance (yesterday's cycle end,
// booked today) and the day's accrual, in numerator units over
// gbp.AccrualDenominator.
type DayResult struct {
	Family  gbp.ProductFamily
	Applied luca.Amount
	Accrued int64
}

// AccountDay is one account's share of the day, run under the account's
// lock (ledger.Lock) by the pass and by every event that moves the
// balance. Yesterday is closed first: if the product's cycle ended on it
// and its position still holds whole pence of accrual, the application is
// booked at yesterday's last second and yesterday's position reprojected
// with the remainder. Then today is projected: the day's interest accrues
// on the balance as it stands, and the position for today is written.
// Both steps are the same however often they run — an applied position
// has nothing left to apply, and the accrual is computed from yesterday's
// position each time — so an account may be visited again after a
// restart, or by an event after the pass, and the last writer's answer is
// the right one. The books may be bound to the caller's transaction.
func (p *Products) AccountDay(books *ledger.Ledger, a Account, day time.Time) (DayResult, error) {
	res := DayResult{Family: a.Product.Family}
	yesterday := day.AddDate(0, 0, -1)
	prev, err := books.PositionAt(a.ID, yesterday)
	if err != nil {
		return res, err
	}
	if prev == nil {
		prev = &luca.Position{AccountID: a.ID, Day: yesterday} // opened today: nothing carried in
	}
	closed, postings := a.Product.Apply(*prev)
	if len(postings) > 0 {
		for _, posting := range postings {
			// The posting names its counterparty by path: a P&L account on
			// the ledger's chart.
			counter, err := books.Account(posting.Counterparty)
			if err != nil {
				return res, err
			}
			if _, err := books.RecordMovement(counter, a.ID, posting.Amount, posting.Code, posting.ValueTime, posting.Description); err != nil {
				return res, err
			}
			res.Applied += posting.Amount
		}
		if _, err := books.Project(a.ID, prev.Day, closed.Accrued); err != nil {
			return res, err
		}
	}
	balance, err := books.Balance(a.ID)
	if err != nil {
		return res, err
	}
	next := a.Product.Accrue(day, closed, balance, gbp.RateBps(a.Product.Rate))
	if _, err := books.Project(a.ID, day, next.Accrued); err != nil {
		return res, err
	}
	res.Accrued = int64(balance) * gbp.RateBps(a.Product.Rate)
	return res, nil
}

// PostEvent posts a customer event — a transfer, a funding — to the
// ledger (ledger.Post: both accounts' positions for the day move at
// once), then runs the day's rules again for the product accounts it
// touched: the projection the pass wrote was on the balance as it stood,
// and the day's interest accrues on the closing balance. The caller holds
// the accounts' locks for as long as its transaction holds their rows, so
// the event and the daily pass take turns on an account. What the rules
// changed in the bank's totals is returned; the day's accrual is counted
// by the pass alone, so an event's results carry none.
func (p *Products) PostEvent(books *ledger.Ledger, day time.Time, fromID, toID string, amount luca.Amount, code, description string, touched ...Account) ([]DayResult, error) {
	if err := books.Post(fromID, toID, amount, code, day, description); err != nil {
		return nil, err
	}
	var results []DayResult
	for _, a := range touched {
		r, err := p.AccountDay(books, a, day)
		if err != nil {
			return results, fmt.Errorf("products: %s after %s: %w", a.ID, description, err)
		}
		r.Accrued = 0
		results = append(results, r)
	}
	return results, nil
}
