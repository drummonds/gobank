package main

import (
	"database/sql"
	"errors"
	"log"
	"time"

	"git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/core"
)

// The history component: the bank's daily series, one snapshot a day, for
// the dashboard charts (ADR-0002 stage 2: stored truth). A snapshot is
// written when a day begins — the book and the customers the bank starts
// the day with, the NIM of the day just closed and the base rate for the
// day — and is never rewritten, so a restart shows the same charts. The
// migration only adds the table: the previous release runs on the same
// database and a rollback is possible.
var historySchema = componentSchema{Name: "history", Migrations: []migration{
	{Version: 1, Statements: []string{`CREATE TABLE IF NOT EXISTS daily_snapshots (
		day TIMESTAMP PRIMARY KEY,
		savings BIGINT NOT NULL,
		lending BIGINT NOT NULL,
		customers INTEGER NOT NULL,
		nim_bps DOUBLE PRECISION NOT NULL,
		boe_rate DOUBLE PRECISION NOT NULL
	)`}},
}}

// DailySnapshot is the bank on one day, as the charts show it.
type DailySnapshot struct {
	Day       time.Time
	Savings   luca.Amount
	Lending   luca.Amount
	Customers int
	NIMBps    float64 // annualised net interest margin, basis points (a rate, not money)
	BoERate   float64 // BoE base rate as a decimal
}

// saveSnapshot stores the day's snapshot. A day already on record keeps
// its row: a restart mid-day does not rewrite the morning's figures.
func saveSnapshot(db *sql.DB, s DailySnapshot) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`INSERT INTO daily_snapshots (day, savings, lending, customers, nim_bps, boe_rate)
		VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (day) DO NOTHING`,
		s.Day, int64(s.Savings), int64(s.Lending), s.Customers, s.NIMBps, s.BoERate); err != nil {
		log.Printf("saveSnapshot: %v", err)
	}
}

// latestSnapshot is the newest day on record.
func latestSnapshot(db *sql.DB) (DailySnapshot, bool) {
	if db == nil {
		return DailySnapshot{}, false
	}
	var s DailySnapshot
	var savings, lending int64
	err := db.QueryRow(`SELECT day, savings, lending, customers, nim_bps, boe_rate FROM daily_snapshots ORDER BY day DESC LIMIT 1`).
		Scan(&s.Day, &savings, &lending, &s.Customers, &s.NIMBps, &s.BoERate)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("latestSnapshot: %v", err)
		}
		return DailySnapshot{}, false
	}
	s.Day = s.Day.UTC()
	s.Savings, s.Lending = luca.Amount(savings), luca.Amount(lending)
	return s, true
}

// snapshotSpan is the first and the latest day on record: the bank's
// opening day and its current business day. ok is false with no record.
func snapshotSpan(db *sql.DB) (first, latest time.Time, ok bool) {
	if db == nil {
		return first, latest, false
	}
	var lo, hi sql.NullTime
	if err := db.QueryRow(`SELECT MIN(day), MAX(day) FROM daily_snapshots`).Scan(&lo, &hi); err != nil || !lo.Valid || !hi.Valid {
		if err != nil {
			log.Printf("snapshotSpan: %v", err)
		}
		return first, latest, false
	}
	return lo.Time.UTC(), hi.Time.UTC(), true
}

// loadSnapshots reads every snapshot, oldest first.
func loadSnapshots(db *sql.DB) []DailySnapshot {
	if db == nil {
		return nil
	}
	rows, err := db.Query(`SELECT day, savings, lending, customers, nim_bps, boe_rate FROM daily_snapshots ORDER BY day`)
	if err != nil {
		log.Printf("loadSnapshots: %v", err)
		return nil
	}
	defer rows.Close()
	var out []DailySnapshot
	for rows.Next() {
		var s DailySnapshot
		var savings, lending int64
		if err := rows.Scan(&s.Day, &savings, &lending, &s.Customers, &s.NIMBps, &s.BoERate); err != nil {
			log.Printf("loadSnapshots: %v", err)
			return out
		}
		s.Day = s.Day.UTC()
		s.Savings, s.Lending = luca.Amount(savings), luca.Amount(lending)
		out = append(out, s)
	}
	return out
}

// historyOf turns the stored snapshots into the daily series the charts
// draw (core.History), the newest maxHistoryPoints of them.
func historyOf(snaps []DailySnapshot) core.History {
	if len(snaps) > maxHistoryPoints {
		snaps = snaps[len(snaps)-maxHistoryPoints:]
	}
	h := core.History{
		Balances:  make([]BalancePoint, len(snaps)),
		Customers: make([]CustomerPoint, len(snaps)),
		NIM:       make([]NIMPoint, len(snaps)),
		BoERate:   make([]RatePoint, len(snaps)),
	}
	for i, s := range snaps {
		h.Balances[i] = BalancePoint{Date: s.Day, Savings: s.Savings, Lending: s.Lending}
		h.Customers[i] = CustomerPoint{Date: s.Day, Count: s.Customers}
		h.NIM[i] = NIMPoint{Date: s.Day, NIM: s.NIMBps}
		h.BoERate[i] = RatePoint{Date: s.Day, Rate: s.BoERate}
	}
	return h
}
