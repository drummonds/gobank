package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// The start-of-day pass (ADR-0002 stage 3): once the date has moved on,
// every registered account is visited once, under its lock, and its
// position for the day written (accountDay). The pass is paced over the
// day's length, flat out when there is none, and it resumes after a
// restart from the projections already written: an account with a
// position for the day is done, whoever wrote it, so the work left is
// exactly the accounts without one. The account is the unit of daily work
// — what each one needs is the product's business and will grow — so
// nothing here batches accounts together.

// passChunk is how many unprojected accounts are fetched at a time.
const passChunk = 256

// unprojectedAccount is one account the pass has still to visit today.
type unprojectedAccount struct {
	id, productID string
}

// unprojected lists up to limit registered accounts, after cursor in
// ledger-account-ID order, that have no position for day.
func unprojected(db *sql.DB, day time.Time, cursor string, limit int) ([]unprojectedAccount, error) {
	rows, err := db.Query(fmt.Sprintf(`SELECT ca.ledger_account_id, ca.product_id FROM contract_customer_accounts ca
		WHERE ca.ledger_account_id > $1
		  AND NOT EXISTS (SELECT 1 FROM contract_ledger_eod_positions p
		                  WHERE p.account_id = ca.ledger_account_id AND p.day = $2)
		ORDER BY ca.ledger_account_id LIMIT %d`, limit), cursor, day.UTC())
	if err != nil {
		return nil, fmt.Errorf("unprojected accounts: %w", err)
	}
	defer rows.Close()
	var out []unprojectedAccount
	for rows.Next() {
		var a unprojectedAccount
		if err := rows.Scan(&a.id, &a.productID); err != nil {
			return nil, fmt.Errorf("unprojected accounts: scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// anyUnprojected reports whether any registered account still has no
// position for day: the day's pass has not finished.
func anyUnprojected(db *sql.DB, day time.Time) (bool, error) {
	if db == nil {
		return false, nil
	}
	accounts, err := unprojected(db, day, "", 1)
	return len(accounts) > 0, err
}

// countUnprojected is how many accounts the day's pass has still to visit.
func countUnprojected(db *sql.DB, day time.Time) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM contract_customer_accounts ca
		WHERE NOT EXISTS (SELECT 1 FROM contract_ledger_eod_positions p
		                  WHERE p.account_id = ca.ledger_account_id AND p.day = $1)`, day.UTC()).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count unprojected: %w", err)
	}
	return n, nil
}

// runPass visits every account without a position for day and writes
// one, paced so that the share done tracks the share of the day elapsed
// (dayEnd zero: flat out), and returns early when ctx ends — a stop or a
// shutdown — leaving the rest for the next pass over the same day. Runs
// without ds.mu held; ds.mu is taken briefly per chunk to book what the
// rules changed. Returns how many accounts it visited.
func (ds *DemoState) runPass(ctx context.Context, day, dayEnd time.Time) int {
	if ds.db == nil || ds.ledger == nil {
		return 0
	}
	total, err := countUnprojected(ds.db, day)
	if err != nil {
		log.Printf("pass: %v", err)
		return 0
	}
	ds.progress.phase("projecting positions", total)
	start := ds.now()
	visited, cursor := 0, ""
	for ctx.Err() == nil {
		accounts, err := unprojected(ds.db, day, cursor, passChunk)
		if err != nil {
			log.Printf("pass: %v", err)
			return visited
		}
		if len(accounts) == 0 {
			return visited
		}
		var results []dayResult
		for _, a := range accounts {
			if ctx.Err() != nil {
				break
			}
			cursor = a.id
			product, ok := ds.productByID(a.productID)
			if !ok {
				log.Printf("pass: account %s is on unknown product %s", a.id, a.productID)
				continue
			}
			unlock := ds.accountLocks.lock(a.id)
			r, err := ds.accountDay(ds.ledger, dayAccount{id: a.id, product: product}, day)
			unlock()
			if err != nil {
				log.Printf("pass: %s: %v", a.id, err)
				continue
			}
			results = append(results, r)
			visited++
			if ds.passHook != nil {
				ds.passHook()
			}
		}
		ds.mu.Lock()
		for _, r := range results {
			ds.bookResultLocked(r)
		}
		ds.mu.Unlock()
		ds.progress.add(len(results))
		ds.pace(ctx, start, dayEnd, visited, total)
		yieldToBrowser()
	}
	return visited
}

// pace waits until the share of the day elapsed catches up with the share
// of accounts done, so the pass is spread over the day rather than run at
// its start. No wait when the day has no length, or when ctx ends.
func (ds *DemoState) pace(ctx context.Context, start, dayEnd time.Time, done, total int) {
	if dayEnd.IsZero() || total <= 0 || done <= 0 {
		return
	}
	target := start.Add(time.Duration(float64(dayEnd.Sub(start)) * float64(done) / float64(total)))
	wait := target.Sub(ds.now())
	if wait <= 0 {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(wait):
	}
}
