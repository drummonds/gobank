// Package history is the bank's daily series: one snapshot a day of the
// book, the customers, the NIM and the risk-free rate, for the dashboard charts
// (ADR-0002 stage 2: stored truth; stage 5, story 1.5.2). A snapshot is
// written when a day begins and never rewritten, so a restart shows the
// same charts. It owns daily_snapshots and reads nothing else.
package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/bank/schema"
	"git.bytestone.uk/hum3/gobank/core"
)

// MaxPoints is how many days the series run to: the charts draw at most
// ~20 years of daily points.
const MaxPoints = 7_300

// Snapshot is the bank on one day, as the charts show it.
type Snapshot struct {
	Day         time.Time
	Savings     luca.Amount
	Lending     luca.Amount
	Customers   int
	NIMBps      float64 // annualised net interest margin, basis points (a rate, not money)
	RiskFreeBps int64   // the risk-free rate in force at the end of the day, basis points (ADR-0009)
}

// History is the daily record over the bank's database.
type History struct {
	db *sql.DB
}

// New opens the record on db.
func New(db *sql.DB) *History {
	return &History{db: db}
}

// Save stores the day's snapshot. A day already on record keeps its row:
// a restart mid-day does not rewrite the morning's figures.
func (h *History) Save(ctx context.Context, s Snapshot) error {
	// boe_rate is still written, as the decimal the older build of the
	// drill's rollback reads; it goes a release after risk_free_bps came.
	_, err := h.db.ExecContext(ctx, `INSERT INTO daily_snapshots (day, savings, lending, customers, nim_bps, boe_rate, risk_free_bps)
		VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (day) DO NOTHING`,
		s.Day, int64(s.Savings), int64(s.Lending), s.Customers, s.NIMBps, float64(s.RiskFreeBps)/10000, s.RiskFreeBps)
	if err != nil {
		return fmt.Errorf("history: save %s: %w", s.Day.Format(time.DateOnly), err)
	}
	return nil
}

// Latest is the newest day on record; ok is false with no record.
func (h *History) Latest(ctx context.Context) (s Snapshot, ok bool, err error) {
	var savings, lending int64
	err = h.db.QueryRowContext(ctx, `SELECT day, savings, lending, customers, nim_bps, `+riskFreeColumn+` FROM daily_snapshots ORDER BY day DESC LIMIT 1`).
		Scan(&s.Day, &savings, &lending, &s.Customers, &s.NIMBps, &s.RiskFreeBps)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, false, nil
		}
		return Snapshot{}, false, fmt.Errorf("history: latest: %w", err)
	}
	s.Day = s.Day.UTC()
	s.Savings, s.Lending = luca.Amount(savings), luca.Amount(lending)
	return s, true, nil
}

// Span is the first and the latest day on record: the bank's opening day
// and its current business day. ok is false with no record.
func (h *History) Span(ctx context.Context) (first, latest time.Time, ok bool, err error) {
	var lo, hi sql.NullTime
	if err := h.db.QueryRowContext(ctx, `SELECT MIN(day), MAX(day) FROM daily_snapshots`).Scan(&lo, &hi); err != nil {
		return first, latest, false, fmt.Errorf("history: span: %w", err)
	}
	if !lo.Valid || !hi.Valid {
		return first, latest, false, nil
	}
	return lo.Time.UTC(), hi.Time.UTC(), true, nil
}

// Series is the daily series the charts draw (core.History): the newest
// MaxPoints days on record, oldest first.
func (h *History) Series(ctx context.Context) (core.History, error) {
	rows, err := h.db.QueryContext(ctx, fmt.Sprintf(`SELECT day, savings, lending, customers, nim_bps, `+riskFreeColumn+` FROM (
			SELECT day, savings, lending, customers, nim_bps, boe_rate, risk_free_bps FROM daily_snapshots ORDER BY day DESC LIMIT %d
		) newest ORDER BY day`, MaxPoints))
	if err != nil {
		return core.History{}, fmt.Errorf("history: series: %w", err)
	}
	defer rows.Close()
	var out core.History
	for rows.Next() {
		var s Snapshot
		var savings, lending int64
		if err := rows.Scan(&s.Day, &savings, &lending, &s.Customers, &s.NIMBps, &s.RiskFreeBps); err != nil {
			return core.History{}, fmt.Errorf("history: series: %w", err)
		}
		s.Day = s.Day.UTC()
		s.Savings, s.Lending = luca.Amount(savings), luca.Amount(lending)
		out.Balances = append(out.Balances, core.BalancePoint{Date: s.Day, Savings: s.Savings, Lending: s.Lending})
		out.Customers = append(out.Customers, core.CustomerPoint{Date: s.Day, Count: s.Customers})
		out.NIM = append(out.NIM, core.NIMPoint{Date: s.Day, NIM: s.NIMBps})
		out.RiskFree = append(out.RiskFree, core.RatePoint{Date: s.Day, Bps: s.RiskFreeBps})
	}
	return out, rows.Err()
}

// riskFreeColumn reads the rate in basis points: rows from before v2 carry
// only the decimal, read back rounded.
const riskFreeColumn = `COALESCE(risk_free_bps, CAST(ROUND(boe_rate * 10000) AS BIGINT))`

// Schema: the daily snapshots. v2 adds the risk-free rate in basis points
// (ADR-0009) beside the decimal, which stays for the drill's rollback.
var Schema = schema.Component{Name: "history", Migrations: []schema.Migration{
	{Version: 1, Statements: []string{`CREATE TABLE IF NOT EXISTS daily_snapshots (
		day TIMESTAMP PRIMARY KEY,
		savings BIGINT NOT NULL,
		lending BIGINT NOT NULL,
		customers INTEGER NOT NULL,
		nim_bps DOUBLE PRECISION NOT NULL,
		boe_rate DOUBLE PRECISION NOT NULL
	)`}},
	{Version: 2, Statements: []string{`ALTER TABLE daily_snapshots ADD COLUMN risk_free_bps BIGINT NULL`}},
}}
