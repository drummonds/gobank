// Package parameters is the bank's settings (ADR-0006): the stored values
// of the parameters product versions and the bank's policy declare, each
// with the day it is effective from and the moment it was decided, so a
// change is decided today for a future day and the bank can say what it
// knew when. Append-only: a correction is a later decision with an
// earlier effective day. The component resolves a parameter for a day as
// known at a moment; what the parameters mean is the version's business.
package parameters

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/schema"
)

// Schema: the settings. A setting belongs to a scope and an owner: the
// bank itself ("bank"), a product version ("easy-access/1") or one
// account (its ledger account ID).
var Schema = schema.Component{Name: "parameters", Migrations: []schema.Migration{
	{Version: 1, Statements: []string{`CREATE TABLE IF NOT EXISTS parameter_settings (
		scope VARCHAR(20) NOT NULL,
		owner VARCHAR(100) NOT NULL,
		key VARCHAR(100) NOT NULL,
		value VARCHAR(200) NOT NULL,
		effective_from TIMESTAMP NOT NULL,
		decided_at TIMESTAMP NOT NULL,
		decided_by VARCHAR(100) NOT NULL,
		PRIMARY KEY (scope, owner, key, effective_from, decided_at)
	)`}},
}}

// Owner names a version's settings: "easy-access/1".
func Owner(v gbp.Version) string { return fmt.Sprintf("%s/%d", v.Product(), v.Version()) }

// OwnerBank is the bank's own settings' owner.
const OwnerBank = "bank"

// Setting is one stored value.
type Setting struct {
	Scope     gbp.Scope
	Owner     string
	Key       string
	Value     string
	Effective time.Time // the day it applies from, midnight UTC
	Decided   time.Time // when it was decided: what the bank knew when
	By        string    // who decided it: a user, "rule", "migration"
}

// Querier is *sql.DB or *sql.Tx.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Parameters is the component over the bank's database.
type Parameters struct {
	db *sql.DB
}

// Open is the component on db.
func Open(db *sql.DB) *Parameters { return &Parameters{db: db} }

// Set records a setting through q, the caller's transaction or the
// database.
func (p *Parameters) Set(ctx context.Context, q Querier, s Setting) error {
	if q == nil {
		q = p.db
	}
	_, err := q.ExecContext(ctx, `INSERT INTO parameter_settings (scope, owner, key, value, effective_from, decided_at, decided_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, string(s.Scope), s.Owner, s.Key, s.Value, day(s.Effective), s.Decided.UTC(), s.By)
	if err != nil {
		return fmt.Errorf("parameters: set %s %s/%s: %w", s.Scope, s.Owner, s.Key, err)
	}
	return nil
}

// Resolve is the value of a parameter on day as known at knownAt: of the
// settings decided by then and effective on or before the day, the one
// with the latest effective day, and of those the latest decided. False
// when there is none.
func (p *Parameters) Resolve(ctx context.Context, q Querier, scope gbp.Scope, owner, key string, on, knownAt time.Time) (string, bool, error) {
	if q == nil {
		q = p.db
	}
	var value string
	err := q.QueryRowContext(ctx, `SELECT value FROM parameter_settings
		WHERE scope = $1 AND owner = $2 AND key = $3 AND effective_from <= $4 AND decided_at <= $5
		ORDER BY effective_from DESC, decided_at DESC LIMIT 1`, string(scope), owner, key, day(on), knownAt.UTC()).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("parameters: resolve %s %s/%s: %w", scope, owner, key, err)
	}
	return value, true, nil
}

func day(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
