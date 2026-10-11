// Package parameters is the bank's settings (ADR-0006): the stored values
// of the parameters products' rules, the bank's policy and the markets
// declare, each with the instant it is effective from and the moment it
// was decided, so a change is decided today for a future instant and the
// bank can say what it knew when. Append-only: a correction is a later
// decision with an earlier effective instant. The component resolves a
// parameter at an instant as known at a moment; a reader wanting a day's
// value asks at the end of that day (EndOfDay), so a publication at noon
// counts for the whole of its day (ADR-0009). What the parameters mean is
// the rules' business.
package parameters

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/schema"
)

// Schema: the settings. A setting belongs to a scope and an owner: a
// market (the currency, at the contract's "market" scope), the bank
// itself ("bank"), a product (its ID, at the contract's "version" scope: a
// product's parameters are its own) or one account (its ledger account
// ID).
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

// CreateView (re)creates the contract view: the settings as other
// components may read them in SQL (ADR-0001), for a query that joins
// them to its own tables. Recreated at every start so a durable database
// picks up a changed definition.
func CreateView(db *sql.DB) error {
	for _, stmt := range []string{
		`DROP VIEW IF EXISTS contract_parameter_settings`,
		`CREATE VIEW contract_parameter_settings AS
			SELECT scope, owner, key, value, effective_from, decided_at, decided_by FROM parameter_settings`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("parameters: contract_parameter_settings: %w", err)
		}
	}
	return nil
}

// OwnerBank is the bank's own settings' owner.
const OwnerBank = "bank"

// Setting is one stored value.
type Setting struct {
	Scope     gbp.Scope
	Owner     string
	Key       string
	Value     string
	Effective time.Time // the instant it applies from
	Decided   time.Time // when it was decided: what the bank knew when
	By        string    // who decided it: a user, "rule", "migration", a market's name
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

// DB is the database the component is on, for a reader with a query of
// its own.
func (p *Parameters) DB() *sql.DB { return p.db }

// Set records a setting through q, the caller's transaction or the
// database.
func (p *Parameters) Set(ctx context.Context, q Querier, s Setting) error {
	if q == nil {
		q = p.db
	}
	_, err := q.ExecContext(ctx, `INSERT INTO parameter_settings (scope, owner, key, value, effective_from, decided_at, decided_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, string(s.Scope), s.Owner, s.Key, s.Value, s.Effective.UTC(), s.Decided.UTC(), s.By)
	if err != nil {
		return fmt.Errorf("parameters: set %s %s/%s: %w", s.Scope, s.Owner, s.Key, err)
	}
	return nil
}

// Resolve is the value of a parameter at an instant as known at knownAt:
// of the settings decided by then and effective at or before the instant,
// the one with the latest effective instant, and of those the latest
// decided. False when there is none. For a day's value ask at EndOfDay.
func (p *Parameters) Resolve(ctx context.Context, q Querier, scope gbp.Scope, owner, key string, at, knownAt time.Time) (string, bool, error) {
	if q == nil {
		q = p.db
	}
	var value string
	err := q.QueryRowContext(ctx, `SELECT value FROM parameter_settings
		WHERE scope = $1 AND owner = $2 AND key = $3 AND effective_from <= $4 AND decided_at <= $5
		ORDER BY effective_from DESC, decided_at DESC LIMIT 1`, string(scope), owner, key, at.UTC(), knownAt.UTC()).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("parameters: resolve %s %s/%s: %w", scope, owner, key, err)
	}
	return value, true, nil
}

// Upcoming is every setting effective after an instant, soonest first
// (then by when decided): decided and not yet in force. Each is an event
// the simulation's timeline carries (ADR-0010).
func (p *Parameters) Upcoming(ctx context.Context, q Querier, after time.Time) ([]Setting, error) {
	if q == nil {
		q = p.db
	}
	rows, err := q.QueryContext(ctx, `SELECT scope, owner, key, value, effective_from, decided_at, decided_by FROM parameter_settings
		WHERE effective_from > $1 ORDER BY effective_from, decided_at`, after.UTC())
	if err != nil {
		return nil, fmt.Errorf("parameters: upcoming: %w", err)
	}
	defer rows.Close()
	var out []Setting
	for rows.Next() {
		var s Setting
		var scope string
		if err := rows.Scan(&scope, &s.Owner, &s.Key, &s.Value, &s.Effective, &s.Decided, &s.By); err != nil {
			return nil, fmt.Errorf("parameters: upcoming: %w", err)
		}
		s.Scope = gbp.Scope(scope)
		s.Effective, s.Decided = s.Effective.UTC(), s.Decided.UTC()
		out = append(out, s)
	}
	return out, rows.Err()
}

// EndOfDay is the last instant of t's day (UTC) at the database's
// precision: what a reader resolves at for the day's value.
func EndOfDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1).Add(-time.Microsecond)
}
