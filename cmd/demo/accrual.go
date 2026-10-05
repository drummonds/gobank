package main

import (
	"hash/fnv"
	"log"
	"slices"
	"sync"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
)

// The products component keeps no state of its own: an account's balance
// and its accrued-but-unapplied interest are its ledger position (go-luca),
// written for the day by the start-of-day pass (pass.go) and again by every
// event that moves the balance. gobank-products holds the rules; this file
// runs them for one account.

// productsSchema: accrual_state held accrued-but-unapplied interest before
// positions did (gobank ≤ v0.10). Nothing reads or writes it now; the table
// stays one release so a rollback to v0.10 finds it, and the release after
// drops it.
var productsSchema = componentSchema{component: "products", migrations: []migration{
	{1, []string{`CREATE TABLE IF NOT EXISTS accrual_state (
		account_id VARCHAR(64) PRIMARY KEY,
		numerator BIGINT NOT NULL,
		accrued_pounds_e7 BIGINT NOT NULL DEFAULT 0, -- accrual as 7dp pounds (rounded view; numerator is canonical)
		as_of TIMESTAMP NOT NULL
	)`}},
}}

// clearAccrualLocked removes what the retired table still holds, e.g. on
// reset of a durable database. Must be called with ds.mu held.
func (ds *DemoState) clearAccrualLocked() {
	if ds.db == nil {
		return
	}
	if _, err := ds.db.Exec(`DELETE FROM accrual_state`); err != nil {
		log.Printf("clearAccrual: %v", err)
	}
}

// accountLocks serialises the work on one account. An in-day event
// (a transfer, a funding) and the daily pass each rewrite the account's
// position inside a transaction that holds its rows; one of them completes
// before the other starts, and every other account is free meanwhile.
// That is the cost of projecting one day ahead: in-day events are heavier,
// and end-of-day processing is smeared across the day. A lock is held for
// as long as its transaction holds the rows, and several are taken in a
// fixed order, so no two holders can wait on each other.
//
// The locks are striped: an account maps to one of accountLockStripes
// mutexes by a hash of its ID, so the set costs a few kilobytes however
// many accounts exist (a mutex per account would be ~150 bytes each,
// never freed). Two accounts that share a stripe take turns needlessly;
// with a handful of concurrent holders that is rare.
type accountLocks struct {
	m [accountLockStripes]sync.Mutex
}

const accountLockStripes = 4096

func accountStripe(id string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return int(h.Sum32() % accountLockStripes)
}

// lock takes the accounts' stripes in stripe order and returns the release.
func (l *accountLocks) lock(ids ...string) (unlock func()) {
	stripes := make([]int, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			stripes = append(stripes, accountStripe(id))
		}
	}
	slices.Sort(stripes)
	stripes = slices.Compact(stripes)
	for _, s := range stripes {
		l.m[s].Lock()
	}
	return func() {
		for i := len(stripes) - 1; i >= 0; i-- {
			l.m[stripes[i]].Unlock()
		}
	}
}

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
// lock by the pass and by every event that moves the balance. Yesterday is
// closed first: if the product's cycle ended on it and its position still
// holds whole pence of accrual, the application is booked at yesterday's
// last second and yesterday's position reprojected with the remainder.
// Then today is projected: the day's interest accrues on the balance as it
// stands, and the position for today is written. Both steps are the same
// however often they run — an applied position has nothing left to apply,
// and the accrual is computed from yesterday's position each time — so an
// account may be visited again after a restart, or by an event after the
// pass, and the last writer's answer is the right one. The ledger may be
// bound to the caller's transaction.
func (ds *DemoState) accountDay(ledger *luca.SQLLedger, a dayAccount, day time.Time) (dayResult, error) {
	res := dayResult{family: a.product.Family}
	yesterday := day.AddDate(0, 0, -1)
	prev, err := ledger.PositionAt(a.id, yesterday)
	if err != nil {
		return res, err
	}
	if prev == nil {
		prev = &luca.Position{AccountID: a.id, Day: yesterday} // opened today: nothing carried in
	}
	closed, postings := a.product.Apply(*prev)
	if len(postings) > 0 {
		for _, p := range postings {
			counter, err := ds.counterparty(ledger, p.Counterparty)
			if err != nil {
				return res, err
			}
			if _, err := ledger.RecordMovement(counter, a.id, p.Amount, p.Code, p.ValueTime, p.Description); err != nil {
				return res, err
			}
			res.applied += p.Amount
		}
		if _, err := ledger.Project(a.id, prev.Day, closed.Accrued); err != nil {
			return res, err
		}
	}
	balance, err := ledger.Balance(a.id)
	if err != nil {
		return res, err
	}
	next := a.product.Accrue(day, closed, balance, gbp.RateBps(a.product.Rate))
	if _, err := ledger.Project(a.id, day, next.Accrued); err != nil {
		return res, err
	}
	res.accrued = int64(balance) * gbp.RateBps(a.product.Rate)
	return res, nil
}

// counterparty is the ledger account an application posting comes from:
// the P&L accounts resolved at start, else looked up by path.
func (ds *DemoState) counterparty(ledger *luca.SQLLedger, path string) (string, error) {
	switch path {
	case "Expense:Interest":
		if ds.expenseInterestID != "" {
			return ds.expenseInterestID, nil
		}
	case "Income:Interest":
		if ds.incomeInterestID != "" {
			return ds.incomeInterestID, nil
		}
	}
	acct, err := ensureLedgerAccount(ledger, path)
	if err != nil {
		return "", err
	}
	return acct.ID, nil
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

// postEvent writes a customer event — a transfer, a funding — with
// projections, so both accounts' positions for the day move at once, then
// runs the day's rules again for the product accounts it touched: the
// projection the pass wrote was on the balance as it stood, and the day's
// interest accrues on the closing balance. The caller holds the accounts'
// locks for as long as its transaction holds their rows, so the event and
// the daily pass take turns on an account. What the rules changed in the
// bank's totals is returned for the caller to book under ds.mu.
func (ds *DemoState) postEvent(ledger *luca.SQLLedger, day time.Time, fromID, toID string, amount luca.Amount, code, description string, touched ...dayAccount) []dayResult {
	if ledger == nil || fromID == "" || toID == "" {
		return nil
	}
	if _, err := ledger.RecordMovementWithProjections(fromID, toID, amount, code, day, description); err != nil {
		log.Printf("postEvent: %v", err)
		return nil
	}
	var results []dayResult
	for _, a := range touched {
		r, err := ds.accountDay(ledger, a, day)
		if err != nil {
			log.Printf("postEvent: %s: %v", a.id, err)
			continue
		}
		r.accrued = 0 // the pass counts the day's accrual once; an event only moves it
		results = append(results, r)
	}
	return results
}
