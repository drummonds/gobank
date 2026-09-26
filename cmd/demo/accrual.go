package main

import (
	"database/sql"
	"log"
	"runtime"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
)

// The products component's persistence of engine state: accrual_state
// holds accrued-but-unapplied interest as exact integer numerators. Other
// code reaches it through the API below, never the table (ADR-0001).

// boeAccrualKey is the accrual_state row holding the bank-level BoE interest
// numerator. Ledger account IDs are UUIDs, so it can never collide.
const boeAccrualKey = "_boe"

// createAccrualTable creates accrual_state, which persists accrued-but-unapplied
// interest as exact integer numerators (minor units = numerator /
// gbp.AccrualDenominator). The ledger only sees interest at month-end
// application, so without this table the DB is missing up to a month of
// accrual per account plus the BoE accumulator.
func (ds *DemoState) createAccrualTable() {
	_, err := ds.db.Exec(`CREATE TABLE IF NOT EXISTS accrual_state (
		account_id VARCHAR(64) PRIMARY KEY,
		numerator BIGINT NOT NULL,
		accrued_pounds_e7 BIGINT NOT NULL DEFAULT 0, -- accrual as 7dp pounds (rounded view; numerator is canonical)
		as_of TIMESTAMP NOT NULL
	)`)
	if err != nil {
		log.Printf("initDB: create accrual_state: %v", err)
	}
	// Migrate tables created before the 7dp pounds column existed (durable
	// postgres DBs); errors on an already-migrated table are expected.
	ds.db.Exec(`ALTER TABLE accrual_state ADD COLUMN accrued_pounds_e7 BIGINT NOT NULL DEFAULT 0`)
}

// clearAccrualLocked removes every persisted numerator, e.g. on reset of a
// durable database. Must be called with ds.mu held.
func (ds *DemoState) clearAccrualLocked() {
	if ds.db == nil {
		return
	}
	if _, err := ds.db.Exec(`DELETE FROM accrual_state`); err != nil {
		log.Printf("clearAccrual: %v", err)
	}
}

// accrualRow is one account's accrued-interest numerator to persist.
type accrualRow struct {
	id        string
	numerator int64
}

// persistAccrualState upserts the day's accrued-interest numerators (per
// account, plus the BoE row) into accrual_state. Runs without ds.mu or
// ds.simMu held, in transactions sized to roughly targetTxTime each (the
// upserts are idempotent, so chunked commits are safe), so other writers
// interleave and no single transaction grows with the account count.
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
				tx.Rollback()
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

// loadAccrualState hydrates accrued-interest numerators (per account and BoE)
// from accrual_state into the engine and demo state. Rows for accounts the
// engine doesn't know are ignored. Must be called with ds.mu and ds.simMu held.
func (ds *DemoState) loadAccrualState() {
	if ds.db == nil || ds.sim == nil {
		return
	}
	rows, err := ds.db.Query(`SELECT account_id, numerator FROM accrual_state`)
	if err != nil {
		log.Printf("loadAccrualState: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var numerator int64
		if err := rows.Scan(&id, &numerator); err != nil {
			log.Printf("loadAccrualState: scan: %v", err)
			return
		}
		if id == boeAccrualKey {
			ds.boeAccruedNumerator = numerator
			ds.boePostedPence = numerator / gbp.AccrualDenominator
			continue
		}
		if ma, ok := ds.sim.GetManagedAccount(id); ok {
			ma.AccruedNumerator = numerator
			// Daily posting maintains posted == floor(numerator/denominator)
			// at every sync point, so the tracker is derivable on restore.
			if ds.accrualPosted == nil {
				ds.accrualPosted = make(map[string]int64)
			}
			ds.accrualPosted[id] = numerator / gbp.AccrualDenominator
		}
	}
	if err := rows.Err(); err != nil {
		log.Printf("loadAccrualState: %v", err)
	}
}
