// Package payments is money moving between customer accounts and to or
// from the outside world (ADR-0002 stage 5, story 1.5.4): the payments
// table, its contract view, and the lifecycle of a payment on record.
// Payment numbers are the component's own (ADR-0004). Moving the money
// itself is the bank's: it posts to the ledger and records the payment
// here.
package payments

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/bank/refs"
	"git.bytestone.uk/hum3/gobank/bank/schema"
	"git.bytestone.uk/hum3/gobank/core"
)

// Type is how money entered or moved.
type Type int

const (
	Transfer         Type = iota // customer to customer
	Deposit                      // external money in (to savings)
	LoanDisbursement             // the bank lends to a customer (creates the loan)
)

func (t Type) String() string {
	switch t {
	case Transfer:
		return "Transfer"
	case Deposit:
		return "Deposit"
	case LoanDisbursement:
		return "Loan"
	default:
		return "Unknown"
	}
}

// Status is where a payment is in its lifecycle.
type Status int

const (
	Pending Status = iota
	Processing
	Completed
)

func (s Status) String() string {
	switch s {
	case Pending:
		return "Pending"
	case Processing:
		return "Processing"
	case Completed:
		return "Completed"
	default:
		return "Unknown"
	}
}

// Payment is one payment on record. FromID and ToID are customer IDs, or
// "EXTERNAL" and "BANK" for the outside world and the bank itself.
type Payment struct {
	ID        int
	Type      Type
	FromID    string
	ToID      string
	Amount    luca.Amount
	Status    Status
	Reference string
	CreatedAt time.Time
	SettledAt time.Time // zero until completed
}

// Core is the payment as the core publishes it.
func (p Payment) Core() core.Payment {
	return core.Payment{
		ID: p.ID, Reference: p.Reference, Type: core.PaymentType(p.Type.String()), From: p.FromID, To: p.ToID,
		Amount: p.Amount, Status: core.PaymentStatus(p.Status.String()), CreatedAt: p.CreatedAt, SettledAt: p.SettledAt,
	}
}

// Schema: the payments table.
var Schema = schema.Component{Name: "payments", Migrations: []schema.Migration{
	{Version: 1, Statements: []string{
		`CREATE TABLE IF NOT EXISTS payments (
			id INTEGER PRIMARY KEY,
			type SMALLINT NOT NULL,
			from_id VARCHAR(20) NOT NULL,
			to_id VARCHAR(20) NOT NULL,
			amount BIGINT NOT NULL,
			status SMALLINT NOT NULL,
			reference VARCHAR(20) NOT NULL UNIQUE,
			created_at TIMESTAMP NOT NULL,
			settled_at TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS payments_from_id ON payments (from_id)`,
		`CREATE INDEX IF NOT EXISTS payments_to_id ON payments (to_id)`,
	}},
}}

// CreateView (re)creates the contract view: what other components may
// read. Recreated at every start so a durable database picks up a changed
// definition.
func CreateView(db *sql.DB) error {
	for _, stmt := range []string{
		`DROP VIEW IF EXISTS contract_payments`,
		`CREATE VIEW contract_payments AS
			SELECT id, reference, type, from_id, to_id, amount, status, created_at, settled_at FROM payments`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("payments: contract_payments: %w", err)
		}
	}
	return nil
}

// Payments is the component over the bank's database.
type Payments struct {
	db  *sql.DB
	seq *refs.Sequence
}

// Open is the component on db, with its view in place and the numbering
// resumed after the newest payment on record.
func Open(db *sql.DB) (*Payments, error) {
	if err := CreateView(db); err != nil {
		return nil, err
	}
	var last int
	if err := db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM payments`).Scan(&last); err != nil {
		return nil, fmt.Errorf("payments: newest payment: %w", err)
	}
	return &Payments{db: db, seq: refs.Resume(last)}, nil
}

// New is a payment with the next number and its reference, created at
// now; a completed payment is settled then too. It is on record once
// Insert has written it.
func (p *Payments) New(t Type, fromID, toID string, amount luca.Amount, status Status, now time.Time) Payment {
	n := p.seq.Next()
	pay := Payment{ID: n, Type: t, FromID: fromID, ToID: toID, Amount: amount, Status: status, Reference: fmt.Sprintf("PAY-%06d", n), CreatedAt: now}
	if status == Completed {
		pay.SettledAt = now
	}
	return pay
}

// Execer is *sql.DB or *sql.Tx.
type Execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// Insert writes a payment row; q is the database or the transaction the
// payment shares with what it funds.
func (p *Payments) Insert(q Execer, pay Payment) error {
	_, err := q.Exec(`INSERT INTO payments (id, type, from_id, to_id, amount, status, reference, created_at, settled_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		pay.ID, int(pay.Type), pay.FromID, pay.ToID, pay.Amount, int(pay.Status), pay.Reference, pay.CreatedAt.UTC(), nullTime(pay.SettledAt))
	if err != nil {
		return fmt.Errorf("payments: insert %s: %w", pay.Reference, err)
	}
	return nil
}

func nullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t.UTC(), Valid: !t.IsZero()}
}

// SetStatus records a lifecycle transition on the payment's row;
// settledAt is written when non-zero.
func (p *Payments) SetStatus(ctx context.Context, id int, status Status, settledAt time.Time) error {
	_, err := p.db.ExecContext(ctx, `UPDATE payments SET status = $1, settled_at = COALESCE($2, settled_at) WHERE id = $3`,
		int(status), nullTime(settledAt), id)
	if err != nil {
		return fmt.Errorf("payments: status of %d: %w", id, err)
	}
	return nil
}

const columns = `id, type, from_id, to_id, amount, status, reference, created_at, settled_at`

func scan(rows *sql.Rows) ([]Payment, error) {
	defer rows.Close()
	var out []Payment
	for rows.Next() {
		var pay Payment
		var settled sql.NullTime
		if err := rows.Scan(&pay.ID, &pay.Type, &pay.FromID, &pay.ToID, &pay.Amount, &pay.Status, &pay.Reference, &pay.CreatedAt, &settled); err != nil {
			return nil, err
		}
		if settled.Valid {
			pay.SettledAt = settled.Time
		}
		out = append(out, pay)
	}
	return out, rows.Err()
}

// ByID is one payment; an unknown one is core.ErrNotFound.
func (p *Payments) ByID(ctx context.Context, id int) (Payment, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT `+columns+` FROM payments WHERE id = $1`, id)
	if err != nil {
		return Payment{}, fmt.Errorf("payments: %d: %w", id, err)
	}
	ps, err := scan(rows)
	if err != nil {
		return Payment{}, fmt.Errorf("payments: %d: %w", id, err)
	}
	if len(ps) == 0 {
		return Payment{}, core.ErrNotFound
	}
	return ps[0], nil
}

// Page is one page of payments, newest first, and the total on record.
func (p *Payments) Page(ctx context.Context, page, perPage int) ([]Payment, int, error) {
	page = max(page, 1)
	rows, err := p.db.QueryContext(ctx, `SELECT `+columns+` FROM payments ORDER BY id DESC LIMIT $1 OFFSET $2`, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: page %d: %w", page, err)
	}
	ps, err := scan(rows)
	if err != nil {
		return nil, 0, fmt.Errorf("payments: page %d: %w", page, err)
	}
	total, err := p.Count(ctx)
	return ps, total, err
}

// Of lists every payment a customer sent or received, oldest first.
func (p *Payments) Of(ctx context.Context, customerID string) ([]Payment, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT `+columns+` FROM payments WHERE from_id = $1 OR to_id = $1 ORDER BY id`, customerID)
	if err != nil {
		return nil, fmt.Errorf("payments: of %s: %w", customerID, err)
	}
	ps, err := scan(rows)
	if err != nil {
		return nil, fmt.Errorf("payments: of %s: %w", customerID, err)
	}
	return ps, nil
}

// Count is the number of payments on record.
func (p *Payments) Count(ctx context.Context) (int, error) {
	var n int
	if err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM payments`).Scan(&n); err != nil {
		return 0, fmt.Errorf("payments: count: %w", err)
	}
	return n, nil
}

// Clear removes every payment and restarts the numbering.
func (p *Payments) Clear(ctx context.Context) error {
	if _, err := p.db.ExecContext(ctx, `DELETE FROM payments`); err != nil {
		return fmt.Errorf("payments: clear: %w", err)
	}
	p.seq = refs.Resume(0)
	return nil
}
