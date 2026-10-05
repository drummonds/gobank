package main

import (
	"database/sql"
	"log"
	"runtime"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
)

// The products component keeps no state of its own: an account's
// accrued-but-unapplied interest lives on its ledger position, written by
// the day's pass (Project) and read back at start (Positions). The
// accrual_state table below is never read any more; it is still written,
// as a shadow, so the previous release can be rolled back to on the same
// database with nothing lost (ADR-0003). Story (e) drops it.

// boeAccrualKey is the accrual_state row holding the bank-level BoE interest
// numerator. Ledger account IDs are UUIDs, so it can never collide.
const boeAccrualKey = "_boe"

// productsSchema: accrual_state held accrued-but-unapplied interest before
// positions did (gobank ≤ v0.10). Shadow-written for rollback.
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

// accrualRow is one account's accrued-interest numerator to project.
type accrualRow struct {
	id        string
	numerator int64
}

// persistAccrualState is the shadow write of accrual_state for rollback:
// the day's numerators (per account, plus the BoE row) upserted in chunked
// transactions, without ds.mu or ds.simMu held. Nothing reads it.
func persistAccrualState(db *sql.DB, day time.Time, rows []accrualRow, boeNumerator int64) {
	if db == nil {
		return
	}
	rows = append(rows, accrualRow{id: boeAccrualKey, numerator: boeNumerator})
	const upsert = `INSERT INTO accrual_state (account_id, numerator, accrued_pounds_e7, as_of) VALUES ($1, $2, $3, $4)
		ON CONFLICT (account_id) DO UPDATE SET numerator = EXCLUDED.numerator,
			accrued_pounds_e7 = EXCLUDED.accrued_pounds_e7, as_of = EXCLUDED.as_of`
	n := 64
	for i := 0; i < len(rows); {
		j := min(i+n, len(rows))
		start := time.Now()
		tx, err := db.Begin()
		if err != nil {
			log.Printf("persistAccrualState: begin: %v", err)
			return
		}
		for _, r := range rows[i:j] {
			if _, err := tx.Exec(upsert, r.id, r.numerator, int64(accrualPoundsE7(r.numerator)), day); err != nil {
				log.Printf("persistAccrualState: %s: %v", r.id, err)
				_ = tx.Rollback()
				return
			}
		}
		if err := tx.Commit(); err != nil {
			log.Printf("persistAccrualState: commit: %v", err)
			return
		}
		if el := time.Since(start); el > 0 {
			n = min(max(int(float64(j-i)*float64(targetTxTime)/float64(el)), 16), 8192)
		}
		i = j
		if runtime.GOOS == "js" {
			time.Sleep(time.Millisecond) // yield to the browser event loop
		}
	}
}

// projectPositions writes the day's position of every account the pass
// touched (and the BoE reserves account), one account at a time: the
// ledger computes the balance from its movements, the row carries the
// engine's accrual numerator. The account is the unit of daily work —
// what each one needs is the product's business and will grow — so
// nothing here batches accounts together. Runs without ds.mu or ds.simMu
// held.
func projectPositions(ledger *luca.SQLLedger, day time.Time, rows []accrualRow) {
	if ledger == nil {
		return
	}
	for i, r := range rows {
		if _, err := ledger.Project(r.id, day, luca.Fraction{Num: r.numerator, Den: gbp.AccrualDenominator}); err != nil {
			log.Printf("projectPositions: %s: %v", r.id, err)
		}
		if runtime.GOOS == "js" && i%64 == 63 {
			time.Sleep(time.Millisecond) // yield to the browser event loop
		}
	}
}

// loadPositions hydrates accrued-interest numerators (per account and the
// BoE reserves) from the ledger's positions into the engine and demo state.
// Positions for accounts the engine doesn't know are ignored. Must be
// called with ds.mu and ds.simMu held, after ensureAccrualAccounts.
func (ds *DemoState) loadPositions() {
	if ds.ledger == nil || ds.sim == nil {
		return
	}
	positions, err := ds.ledger.Positions(ds.currentDay)
	if err != nil {
		log.Printf("loadPositions: %v", err)
		return
	}
	for _, p := range positions {
		if p.Accrued.Den != gbp.AccrualDenominator {
			if p.Accrued.Num != 0 {
				log.Printf("loadPositions: %s: accrual denominator %d, want %d", p.AccountID, p.Accrued.Den, gbp.AccrualDenominator)
			}
			continue
		}
		if p.AccountID == ds.boeReservesID {
			ds.boeAccruedNumerator = p.Accrued.Num
			ds.boePostedPence = p.Accrued.Num / gbp.AccrualDenominator
			continue
		}
		if ma, ok := ds.sim.GetManagedAccount(p.AccountID); ok {
			ma.AccruedNumerator = p.Accrued.Num
			// Daily posting maintains posted == floor(numerator/denominator)
			// at every sync point, so the tracker is derivable on restore.
			if ds.accrualPosted == nil {
				ds.accrualPosted = make(map[string]int64)
			}
			ds.accrualPosted[p.AccountID] = p.Accrued.Num / gbp.AccrualDenominator
		}
	}
}
