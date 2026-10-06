package main

import (
	"log"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
)

// The products component keeps no state of its own: an account's balance
// and its accrued-but-unapplied interest are its ledger position (go-luca),
// written for the day by the start-of-day pass (pass.go) and again by every
// event that moves the balance. gobank-products holds the rules; this file
// runs them for one account.

// productsSchema: the component owns no table. accrual_state held
// accrued-but-unapplied interest before positions did (gobank ≤ v0.10);
// v0.11 stopped writing it and version 2 drops it.
var productsSchema = componentSchema{Name: "products", Migrations: []migration{
	{Version: 1, Statements: []string{`CREATE TABLE IF NOT EXISTS accrual_state (
		account_id VARCHAR(64) PRIMARY KEY,
		numerator BIGINT NOT NULL,
		accrued_pounds_e7 BIGINT NOT NULL DEFAULT 0,
		as_of TIMESTAMP NOT NULL
	)`}},
	{Version: 2, Statements: []string{`DROP TABLE IF EXISTS accrual_state`}},
}}

// dayAccount is an account the day's rules run for: its ledger account
// and the product whose rules apply.
type dayAccount struct {
	id      string
	product Product
}

// dayResult is what one account's daily work changed in the bank's
// totals: the interest applied to its balance (yesterday's cycle end,
// booked today) and the day's accrual, in numerator units.
type dayResult struct {
	family  gbp.ProductFamily
	applied luca.Amount
	accrued int64
}

// accountDay is one account's share of the day, run under the account's
// lock (ledger.Lock) by the pass and by every event that moves the balance. Yesterday is
// closed first: if the product's cycle ended on it and its position still
// holds whole pence of accrual, the application is booked at yesterday's
// last second and yesterday's position reprojected with the remainder.
// Then today is projected: the day's interest accrues on the balance as it
// stands, and the position for today is written. Both steps are the same
// however often they run — an applied position has nothing left to apply,
// and the accrual is computed from yesterday's position each time — so an
// account may be visited again after a restart, or by an event after the
// pass, and the last writer's answer is the right one. The books may be
// bound to the caller's transaction.
func (ds *DemoState) accountDay(books *ledger.Ledger, a dayAccount, day time.Time) (dayResult, error) {
	res := dayResult{family: a.product.Family}
	yesterday := day.AddDate(0, 0, -1)
	prev, err := books.PositionAt(a.id, yesterday)
	if err != nil {
		return res, err
	}
	if prev == nil {
		prev = &luca.Position{AccountID: a.id, Day: yesterday} // opened today: nothing carried in
	}
	closed, postings := a.product.Apply(*prev)
	if len(postings) > 0 {
		for _, p := range postings {
			// The posting names its counterparty by path: a P&L account on
			// the ledger's chart.
			counter, err := books.Account(p.Counterparty)
			if err != nil {
				return res, err
			}
			if _, err := books.RecordMovement(counter, a.id, p.Amount, p.Code, p.ValueTime, p.Description); err != nil {
				return res, err
			}
			res.applied += p.Amount
		}
		if _, err := books.Project(a.id, prev.Day, closed.Accrued); err != nil {
			return res, err
		}
	}
	balance, err := books.Balance(a.id)
	if err != nil {
		return res, err
	}
	next := a.product.Accrue(day, closed, balance, gbp.RateBps(a.product.Rate))
	if _, err := books.Project(a.id, day, next.Accrued); err != nil {
		return res, err
	}
	res.accrued = int64(balance) * gbp.RateBps(a.product.Rate)
	return res, nil
}

// bookResultLocked puts what an account's day changed into the running
// totals. Must be called with ds.mu held.
func (ds *DemoState) bookResultLocked(r dayResult) {
	ds.addToBook(r.family, r.applied)
	if r.family == gbp.FamilySavings {
		ds.dayAccrualSavings += r.accrued
	} else {
		ds.dayAccrualLending += r.accrued
	}
}

// postEvent posts a customer event — a transfer, a funding — to the
// ledger (ledger.Post: both accounts' positions for the day move at
// once), then runs the day's rules again for the product accounts it
// touched: the projection the pass wrote was on the balance as it stood,
// and the day's interest accrues on the closing balance. The caller holds
// the accounts' locks for as long as its transaction holds their rows, so
// the event and the daily pass take turns on an account. What the rules
// changed in the bank's totals is returned for the caller to book under
// ds.mu.
func (ds *DemoState) postEvent(books *ledger.Ledger, day time.Time, fromID, toID string, amount luca.Amount, code, description string, touched ...dayAccount) []dayResult {
	if books == nil || fromID == "" || toID == "" {
		return nil
	}
	if err := books.Post(fromID, toID, amount, code, day, description); err != nil {
		log.Print(err)
		return nil
	}
	var results []dayResult
	for _, a := range touched {
		r, err := ds.accountDay(books, a, day)
		if err != nil {
			log.Printf("postEvent: %s: %v", a.id, err)
			continue
		}
		r.accrued = 0 // the pass counts the day's accrual once; an event only moves it
		results = append(results, r)
	}
	return results
}
