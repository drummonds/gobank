// Package treasury is where the bank places its liquidity: the gilt yield
// curve it is quoted and the gilts it holds (ADR-0002 stage 5, story 1.5.1).
// It owns gilt_yields and gilt_holdings and reads nothing of the rest of
// the bank but its business day.
package treasury

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/bank/schema"
	"git.bytestone.uk/hum3/gobank/core"
)

// Treasury is the gilt desk over the bank's database.
type Treasury struct {
	db  *sql.DB
	day func() time.Time // the bank's business day, the date a purchase is booked on
}

// New opens the desk on db; day gives the bank's business day.
func New(db *sql.DB, day func() time.Time) *Treasury {
	return &Treasury{db: db, day: day}
}

// Yields is the current yield curve, by tenor.
func (t *Treasury) Yields(ctx context.Context) ([]core.GiltYield, error) {
	rows, err := t.db.QueryContext(ctx, `SELECT tenor, rate FROM gilt_yields ORDER BY tenor`)
	if err != nil {
		return nil, fmt.Errorf("treasury: yields: %w", err)
	}
	defer rows.Close()
	var yields []core.GiltYield
	for rows.Next() {
		var y core.GiltYield
		if err := rows.Scan(&y.Tenor, &y.Rate); err != nil {
			return nil, fmt.Errorf("treasury: yields: %w", err)
		}
		yields = append(yields, y)
	}
	return yields, rows.Err()
}

// Holdings is every gilt the bank holds, oldest first.
func (t *Treasury) Holdings(ctx context.Context) ([]core.GiltHolding, error) {
	rows, err := t.db.QueryContext(ctx, `SELECT id, tenor, face_value, purchase_date, yield FROM gilt_holdings ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("treasury: holdings: %w", err)
	}
	defer rows.Close()
	var holdings []core.GiltHolding
	for rows.Next() {
		var h core.GiltHolding
		if err := rows.Scan(&h.ID, &h.Tenor, &h.FaceValue, &h.PurchaseDate, &h.Yield); err != nil {
			return nil, fmt.Errorf("treasury: holdings: %w", err)
		}
		holdings = append(holdings, h)
	}
	return holdings, rows.Err()
}

// Buy is the core's BuyGilt command: it buys faceValue of gilts of the
// given tenor at today's yield, on the bank's business day, and records
// the holding. Below core.MinGiltPurchase is refused; an unquoted tenor is
// core.ErrNotFound.
func (t *Treasury) Buy(ctx context.Context, tenor string, faceValue luca.Amount) error {
	if faceValue < core.MinGiltPurchase {
		return core.ErrInvalidAmount
	}
	var rate float64
	if err := t.db.QueryRowContext(ctx, `SELECT rate FROM gilt_yields WHERE tenor = $1`, tenor).Scan(&rate); err != nil {
		return fmt.Errorf("tenor %q: %w", tenor, core.ErrNotFound)
	}
	_, err := t.db.ExecContext(ctx, `INSERT INTO gilt_holdings (tenor, face_value, purchase_date, yield) VALUES ($1, $2, $3, $4)`,
		tenor, faceValue, t.day(), rate)
	if err != nil {
		return fmt.Errorf("treasury: buy: %w", err)
	}
	return nil
}

// Schema: gilt yields (seeded once with the opening curve) and the bank's
// gilt holdings.
var Schema = schema.Component{Name: "treasury", Migrations: []schema.Migration{
	{Version: 1, Statements: []string{
		`CREATE TABLE IF NOT EXISTS gilt_yields (
			tenor VARCHAR(10) PRIMARY KEY,
			rate REAL NOT NULL,
			effective_date TIMESTAMP DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS gilt_holdings (
			id SERIAL PRIMARY KEY,
			tenor VARCHAR(10) NOT NULL,
			face_value BIGINT NOT NULL, -- minor units (pence)
			purchase_date TIMESTAMP NOT NULL,
			yield REAL NOT NULL
		)`,
		`INSERT INTO gilt_yields (tenor, rate) VALUES
			('1Y', 0.0435), ('2Y', 0.0410), ('5Y', 0.0395), ('10Y', 0.0405), ('30Y', 0.0445)
			ON CONFLICT (tenor) DO NOTHING`,
	}},
}}
