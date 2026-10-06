package products

import (
	"context"
	"fmt"
	"log"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/internal/yield"
)

// The start-of-day pass (ADR-0002 stage 3): once the date has moved on,
// every registered account is visited once, under its lock, and its
// position for the day written (AccountDay). The pass runs at the start
// of the day at the system's capacity, whatever the day length: the
// length is headroom, as the night is for a bank's overnight run, and the
// rest of the day is idle. It resumes after a restart from the
// projections already written: an account with a position for the day is
// done, whoever wrote it, so the work left is exactly the accounts without
// one. The account is the unit of daily work — what each one needs is the
// product's business and will grow — so nothing here batches accounts
// together.

// passChunk is how many unprojected accounts are fetched at a time.
const passChunk = 256

// unprojectedAccount is one account the pass has still to visit today.
type unprojectedAccount struct {
	id, productID string
}

// unprojected lists up to limit registered accounts, after cursor in
// ledger-account-ID order, that have no position for day.
func (p *Products) unprojected(ctx context.Context, day time.Time, cursor string, limit int) ([]unprojectedAccount, error) {
	rows, err := p.db.QueryContext(ctx, fmt.Sprintf(`SELECT ca.ledger_account_id, ca.product_id FROM contract_customer_accounts ca
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

// AnyUnprojected reports whether any registered account still has no
// position for day: the day's pass has not finished.
func (p *Products) AnyUnprojected(ctx context.Context, day time.Time) (bool, error) {
	accounts, err := p.unprojected(ctx, day, "", 1)
	return len(accounts) > 0, err
}

// CountUnprojected is how many accounts the day's pass has still to visit.
func (p *Products) CountUnprojected(ctx context.Context, day time.Time) (int, error) {
	var n int
	err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM contract_customer_accounts ca
		WHERE NOT EXISTS (SELECT 1 FROM contract_ledger_eod_positions p
		                  WHERE p.account_id = ca.ledger_account_id AND p.day = $1)`, day.UTC()).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count unprojected: %w", err)
	}
	return n, nil
}

// Progress is told how the pass is going: the phase it is in with the
// accounts it expects, and the accounts done since.
type Progress interface {
	Phase(name string, total int)
	Add(n int)
}

// PassResult is what the pass did: the accounts it visited and what the
// rules changed in the bank's totals, per family, in numerator units
// (Accrued) and whole pence applied (Applied).
type PassResult struct {
	Visited        int
	AccruedSavings int64
	AccruedLending int64
	AppliedSavings luca.Amount
	AppliedLending luca.Amount
}

func (r *PassResult) add(d DayResult) {
	if d.Family == gbp.FamilySavings {
		r.AccruedSavings += d.Accrued
		r.AppliedSavings += d.Applied
	} else {
		r.AccruedLending += d.Accrued
		r.AppliedLending += d.Applied
	}
}

// RunPass visits every account without a position for day and writes
// one, flat out, and returns early when ctx ends — a stop or a shutdown —
// leaving the rest for the next pass over the same day. visited, if set,
// is called after each account (tests cut a pass short through it).
func (p *Products) RunPass(ctx context.Context, books *ledger.Ledger, day time.Time, progress Progress, visited func()) PassResult {
	var res PassResult
	total, err := p.CountUnprojected(ctx, day)
	if err != nil {
		log.Printf("pass: %v", err)
		return res
	}
	progress.Phase("projecting positions", total)
	cursor := ""
	for ctx.Err() == nil {
		accounts, err := p.unprojected(ctx, day, cursor, passChunk)
		if err != nil {
			log.Printf("pass: %v", err)
			return res
		}
		if len(accounts) == 0 {
			return res
		}
		done := 0
		for _, a := range accounts {
			if ctx.Err() != nil {
				break
			}
			cursor = a.id
			product, ok := p.ByID(a.productID)
			if !ok {
				log.Printf("pass: account %s is on unknown product %s", a.id, a.productID)
				continue
			}
			unlock := books.Lock(a.id)
			r, err := p.AccountDay(books, Account{ID: a.id, Product: product}, day)
			unlock()
			if err != nil {
				log.Printf("pass: %s: %v", a.id, err)
				continue
			}
			res.add(r)
			res.Visited++
			done++
			if visited != nil {
				visited()
			}
		}
		progress.Add(done)
		yield.ToBrowser()
	}
	return res
}
