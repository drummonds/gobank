package gl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
)

// The GL close (ADR-0005): the pass of day D closes day D-1, so once the
// pass has visited every account the GL closes D-1 behind it. The
// journal is derived from the sub-ledger after the pass completes (D-1's
// application movements grouped by product and code), each control's
// position for D-1 is projected with the sub-ledger's summed accrual, the
// control's closed balance is compared with the sum of the sub-ledger's
// positions, and the day is recorded as closed. One transaction per day:
// a close cut short leaves nothing, and the next start makes it again.
// Days close in sequence, so a catch-up after downtime skips none.

// Status is the general ledger as the bank reports it: the last day it
// has closed, and the breaks that close found.
type Status struct {
	PostedThrough time.Time // zero before the first close
	Breaks        []Break
}

// Break is a product whose control account disagrees with its sub-ledger
// accounts at a close.
type Break struct {
	Day                time.Time
	ProductID          string
	Control, SubLedger luca.Amount
}

// accruedScale is the sub-ledger's end-of-day view's accrued interest,
// in ten-millionths of a pound, as minor units: pence over this.
const accruedScale = 100_000

// Close closes every day not yet closed up to and including through, and
// returns the status after. Nothing to close is not an error.
func (g *Ledger) Close(ctx context.Context, through time.Time) (Status, error) {
	through = dayOf(through)
	next, err := g.nextToClose(ctx)
	if err != nil {
		return Status{}, err
	}
	for d := next; !d.After(through); d = d.AddDate(0, 0, 1) {
		if err := g.closeDay(ctx, d); err != nil {
			return Status{}, err
		}
	}
	return g.Status(ctx)
}

// nextToClose is the day after the last close; before any, the opening
// day, or the day before it for a GL that adopted a running bank.
func (g *Ledger) nextToClose(ctx context.Context) (time.Time, error) {
	last, ok, err := g.postedThrough(ctx)
	if err != nil {
		return time.Time{}, err
	}
	if ok {
		return last.AddDate(0, 0, 1), nil
	}
	if g.opening.adopted {
		return g.opening.day.AddDate(0, 0, -1), nil
	}
	return g.opening.day, nil
}

func (g *Ledger) postedThrough(ctx context.Context) (time.Time, bool, error) {
	var last sql.NullTime
	if err := g.q.QueryRowContext(ctx, `SELECT MAX(day) FROM gl_closes`).Scan(&last); err != nil {
		return time.Time{}, false, fmt.Errorf("gl: posted through: %w", err)
	}
	return last.Time.UTC(), last.Valid, nil
}

// closeDay closes one day in one transaction.
func (g *Ledger) closeDay(ctx context.Context, day time.Time) error {
	tx, err := g.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("gl: close %s: %w", day.Format(time.DateOnly), err)
	}
	defer tx.Rollback()
	t := g.WithTx(tx)
	sub, err := t.subLedger(ctx, day)
	if err != nil {
		return err
	}
	if day.Before(g.opening.day) {
		err = t.openingBalances(day, sub)
	} else {
		err = t.journal(ctx, day)
	}
	if err != nil {
		return err
	}
	if err := t.projectControls(day, sub); err != nil {
		return err
	}
	if err := t.reconcile(ctx, day, sub); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO gl_closes (day, closed_at) VALUES ($1, $2)`, day, time.Now().UTC()); err != nil {
		return fmt.Errorf("gl: close %s: %w", day.Format(time.DateOnly), err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("gl: close %s: commit: %w", day.Format(time.DateOnly), err)
	}
	return nil
}

// subPosition is what a product's customer accounts sum to on a day in
// the sub-ledger: balance in minor units, accrued interest over
// accruedScale minor units.
type subPosition struct {
	balance   luca.Amount
	accruedE7 int64
}

// subLedger sums the sub-ledger's end-of-day positions on day per product,
// through the customers' and the ledger's views. The views give money in
// major units as a NUMERIC; it is scaled to integers before summing so
// the sum is exact on both drivers.
func (g *Ledger) subLedger(ctx context.Context, day time.Time) (map[string]subPosition, error) {
	rows, err := g.q.QueryContext(ctx, fmt.Sprintf(`SELECT ca.product_id,
			COALESCE(SUM(CAST(ROUND(p.balance * %d) AS BIGINT)), 0),
			COALESCE(SUM(CAST(ROUND(p.accrued * 10000000) AS BIGINT)), 0)
		FROM contract_ledger_eod_positions p
		JOIN contract_customer_accounts ca ON ca.ledger_account_id = p.account_id
		WHERE p.day = $1
		GROUP BY ca.product_id`, ledger.Unit), day)
	if err != nil {
		return nil, fmt.Errorf("gl: sub-ledger %s: %w", day.Format(time.DateOnly), err)
	}
	defer rows.Close()
	out := map[string]subPosition{}
	for rows.Next() {
		var id string
		var p subPosition
		if err := rows.Scan(&id, &p.balance, &p.accruedE7); err != nil {
			return nil, fmt.Errorf("gl: sub-ledger %s: %w", day.Format(time.DateOnly), err)
		}
		out[id] = p
	}
	return out, rows.Err()
}

// openingBalances is an adopted GL's first close: each control takes
// the sub-ledger's balance for the day from equity, the events before
// the GL existed in one movement.
func (g *Ledger) openingBalances(day time.Time, sub map[string]subPosition) error {
	at := lastSecond(day)
	for productID, control := range g.controls {
		bal := sub[productID].balance
		from, to := g.Chart.EquityCapital, control
		if bal < 0 {
			from, to, bal = control, g.Chart.EquityCapital, -bal
		}
		if bal == 0 {
			continue
		}
		if _, err := g.RecordMovement(from, to, bal, luca.CodeOpeningBalance, at, "Opening balance from the sub-ledger"); err != nil {
			return fmt.Errorf("gl: opening balance %s: %w", productID, err)
		}
	}
	return nil
}

// journal posts the day's interest applications from the sub-ledger in
// aggregate: one movement per product and code, from the P&L account the
// applications came from to the product's control, valued at the day's
// last second as the applications are, and recorded in the journal.
func (g *Ledger) journal(ctx context.Context, day time.Time) error {
	rows, err := g.q.QueryContext(ctx, `SELECT ca.product_id, m.code, m.from_path, COALESCE(SUM(m.amount), 0)
		FROM contract_ledger_movements m
		JOIN contract_customer_accounts ca ON ca.ledger_account_id = m.to_account_id
		WHERE m.value_time >= $1 AND m.value_time < $2 AND m.from_path IN ($3, $4)
		GROUP BY ca.product_id, m.code, m.from_path`, day, day.AddDate(0, 0, 1), ledger.ExpenseInterest, ledger.IncomeInterest)
	if err != nil {
		return fmt.Errorf("gl: journal %s: %w", day.Format(time.DateOnly), err)
	}
	type line struct {
		productID, code, path string
		amount                luca.Amount
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.productID, &l.code, &l.path, &l.amount); err != nil {
			rows.Close()
			return fmt.Errorf("gl: journal %s: %w", day.Format(time.DateOnly), err)
		}
		lines = append(lines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("gl: journal %s: %w", day.Format(time.DateOnly), err)
	}
	at := lastSecond(day)
	for _, l := range lines {
		control, ok := g.controls[l.productID]
		if !ok {
			return fmt.Errorf("gl: journal %s: no control account for product %s", day.Format(time.DateOnly), l.productID)
		}
		from := g.Chart.ExpenseInterest
		if l.path == ledger.IncomeInterest {
			from = g.Chart.IncomeInterest
		}
		desc := fmt.Sprintf("Interest applied on %s for %s", l.productID, day.Format(time.DateOnly))
		m, err := g.RecordMovement(from, control, l.amount, l.code, at, desc)
		if err != nil {
			return fmt.Errorf("gl: journal %s %s: %w", day.Format(time.DateOnly), l.productID, err)
		}
		if _, err := g.q.ExecContext(ctx, `INSERT INTO gl_journal (day, product_id, code, amount, movement_id) VALUES ($1, $2, $3, $4, $5)`,
			day, l.productID, l.code, int64(l.amount), m.ID); err != nil {
			return fmt.Errorf("gl: journal %s %s: %w", day.Format(time.DateOnly), l.productID, err)
		}
	}
	return nil
}

// projectControls writes each control's position for the day: its
// balance from its movements, its accrued interest the sum of its
// accounts' in the sub-ledger.
func (g *Ledger) projectControls(day time.Time, sub map[string]subPosition) error {
	for productID, control := range g.controls {
		accrued := luca.Fraction{Num: sub[productID].accruedE7, Den: accruedScale}
		if _, err := g.Project(control, day, accrued); err != nil {
			return fmt.Errorf("gl: project %s %s: %w", productID, day.Format(time.DateOnly), err)
		}
	}
	return nil
}

// reconcile records, per product, the control's closed balance beside
// the sub-ledger's sum for the day.
func (g *Ledger) reconcile(ctx context.Context, day time.Time, sub map[string]subPosition) error {
	for productID, control := range g.controls {
		pos, err := g.PositionAt(control, day)
		if err != nil {
			return fmt.Errorf("gl: reconcile %s %s: %w", productID, day.Format(time.DateOnly), err)
		}
		var bal luca.Amount
		if pos != nil {
			bal = pos.Balance
		}
		if _, err := g.q.ExecContext(ctx, `INSERT INTO gl_reconciliations (day, product_id, control, sub_ledger) VALUES ($1, $2, $3, $4)`,
			day, productID, int64(bal), int64(sub[productID].balance)); err != nil {
			return fmt.Errorf("gl: reconcile %s %s: %w", productID, day.Format(time.DateOnly), err)
		}
	}
	return nil
}

// Status is the general ledger as it stands: the posted-through day and
// that close's breaks.
func (g *Ledger) Status(ctx context.Context) (Status, error) {
	last, ok, err := g.postedThrough(ctx)
	if err != nil || !ok {
		return Status{}, err
	}
	s := Status{PostedThrough: last}
	rows, err := g.q.QueryContext(ctx, `SELECT product_id, control, sub_ledger FROM gl_reconciliations
		WHERE day = $1 AND control <> sub_ledger ORDER BY product_id`, last)
	if err != nil {
		return Status{}, fmt.Errorf("gl: breaks: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		b := Break{Day: last}
		var control, sub int64
		if err := rows.Scan(&b.ProductID, &control, &sub); err != nil {
			return Status{}, fmt.Errorf("gl: breaks: %w", err)
		}
		b.Control, b.SubLedger = luca.Amount(control), luca.Amount(sub)
		s.Breaks = append(s.Breaks, b)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Status{}, fmt.Errorf("gl: breaks: %w", err)
	}
	return s, nil
}

// lastSecond is 23:59:59 on day, where the sub-ledger values its
// applications.
func lastSecond(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, time.UTC)
}
