// Package events is the record of what happened to the bank and what it
// caused (ADR-0011): every command the bank runs is a root event, raised
// pending at the instant its work is due and marked done when its cascade
// has finished; what the cascade writes carries the root's ID as its
// cause. A restart finds the roots due and not done and raises each
// again: the effects are proven by their own facts, so a replay skips
// what was done. Continuations, events a root leaves for a later instant,
// come with the first rule that needs one.
package events

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"git.bytestone.uk/hum3/gobank/bank/schema"
)

// Kind names what happened.
type Kind string

const (
	StartOfDay  Kind = "start of day" // the clock's: the day's pass and the GL close
	Publication Kind = "publication"  // a market published a rate: the parameter changes it fires
	Payment     Kind = "payment"      // a transfer: the posting, the rules it reruns, the GL
	Setting     Kind = "setting"      // a staff decision became effective: the parameter changes it fires
	Opening     Kind = "opening"      // a customer opened: the accounts, their funding and first positions
)

// Status is where a cascade is.
type Status string

const (
	Pending Status = "pending" // raised, its cascade not finished
	Done    Status = "done"
)

// Event is one root (or, later, continuation) on record.
type Event struct {
	ID       string
	Kind     Kind
	At       time.Time // the instant the work is due, in the bank's time
	ParentID string    // empty for a root
	RootID   string    // the root of the cascade; a root's own ID
	Status   Status
	RaisedBy string    // who raised it: a user, a customer, a market's name, "clock"
	RaisedAt time.Time // wall time the row was written, operational: orders what happened at one instant
	DoneAt   time.Time // wall time, operational; zero while pending
}

// Schema: the events table.
var Schema = schema.Component{Name: "events", Migrations: []schema.Migration{
	{Version: 1, Statements: []string{
		`CREATE TABLE IF NOT EXISTS events (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			kind VARCHAR(30) NOT NULL,
			at TIMESTAMP NOT NULL,
			parent_id UUID,
			root_id UUID,
			status VARCHAR(20) NOT NULL,
			raised_by VARCHAR(100) NOT NULL,
			raised_at TIMESTAMP NOT NULL,
			done_at TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS events_status_at ON events (status, at)`,
		`CREATE INDEX IF NOT EXISTS events_at ON events (at)`,
	}},
}}

// Querier is *sql.DB or *sql.Tx.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Events is the record over the bank's database.
type Events struct {
	db  *sql.DB
	now func() time.Time // the wall clock, for done_at
}

// Open is the record on db.
func Open(db *sql.DB) *Events { return &Events{db: db, now: time.Now} }

func (e *Events) q(q Querier) Querier {
	if q == nil {
		return e.db
	}
	return q
}

// Raise records a root pending at an instant, through q, the caller's
// transaction or the database, and returns it with its ID.
func (e *Events) Raise(ctx context.Context, q Querier, kind Kind, at time.Time, by string) (Event, error) {
	var id string
	raised := e.now().UTC()
	if err := e.q(q).QueryRowContext(ctx, `INSERT INTO events (kind, at, status, raised_by, raised_at) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		string(kind), at.UTC(), string(Pending), by, raised).Scan(&id); err != nil {
		return Event{}, fmt.Errorf("events: raise %s at %s: %w", kind, at.UTC().Format(time.RFC3339), err)
	}
	if _, err := e.q(q).ExecContext(ctx, `UPDATE events SET root_id = id WHERE id = $1`, id); err != nil {
		return Event{}, fmt.Errorf("events: raise %s: root: %w", kind, err)
	}
	return Event{ID: id, Kind: kind, At: at.UTC(), RootID: id, Status: Pending, RaisedBy: by, RaisedAt: raised}, nil
}

// RaiseOnce is the root of a kind at an instant: the one on record, done
// or pending, else raised now. For a root whose identity is its instant,
// the start of a day, so a process restarted between raising it and its
// first effect finds it rather than raising a second.
func (e *Events) RaiseOnce(ctx context.Context, q Querier, kind Kind, at time.Time, by string) (Event, error) {
	rows, err := e.q(q).QueryContext(ctx, `SELECT `+columns+` FROM events WHERE kind = $1 AND at = $2 AND parent_id IS NULL ORDER BY at LIMIT 1`, string(kind), at.UTC())
	if err != nil {
		return Event{}, fmt.Errorf("events: root %s at %s: %w", kind, at.UTC().Format(time.RFC3339), err)
	}
	found, err := scan(rows)
	if err != nil {
		return Event{}, err
	}
	if len(found) == 1 {
		return found[0], nil
	}
	return e.Raise(ctx, q, kind, at, by)
}

// Done marks an event's cascade finished, at the wall's instant, through q.
func (e *Events) Done(ctx context.Context, q Querier, id string) error {
	if _, err := e.q(q).ExecContext(ctx, `UPDATE events SET status = $1, done_at = $2 WHERE id = $3`, string(Done), e.now().UTC(), id); err != nil {
		return fmt.Errorf("events: done %s: %w", id, err)
	}
	return nil
}

// Due is every event pending at or before an instant, earliest first:
// what the bank has still to do by then (ADR-0010's at rest, read from
// the record).
func (e *Events) Due(ctx context.Context, q Querier, by time.Time) ([]Event, error) {
	rows, err := e.q(q).QueryContext(ctx, `SELECT `+columns+` FROM events WHERE status = $1 AND at <= $2 ORDER BY at, id`, string(Pending), by.UTC())
	if err != nil {
		return nil, fmt.Errorf("events: due by %s: %w", by.UTC().Format(time.RFC3339), err)
	}
	return scan(rows)
}

// CountDue is how many events are pending at or before an instant.
func (e *Events) CountDue(ctx context.Context, by time.Time) (int, error) {
	var n int
	if err := e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE status = $1 AND at <= $2`, string(Pending), by.UTC()).Scan(&n); err != nil {
		return 0, fmt.Errorf("events: count due: %w", err)
	}
	return n, nil
}

// ByID is one event; false when there is none.
func (e *Events) ByID(ctx context.Context, id string) (Event, bool, error) {
	rows, err := e.db.QueryContext(ctx, `SELECT `+columns+` FROM events WHERE id = $1`, id)
	if err != nil {
		return Event{}, false, fmt.Errorf("events: %s: %w", id, err)
	}
	found, err := scan(rows)
	if err != nil || len(found) == 0 {
		return Event{}, false, err
	}
	return found[0], true, nil
}

// Recent is the latest events by instant, newest first, up to limit; of
// two at one instant, the one raised later first.
func (e *Events) Recent(ctx context.Context, limit int) ([]Event, error) {
	rows, err := e.db.QueryContext(ctx, `SELECT `+columns+` FROM events ORDER BY at DESC, raised_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("events: recent: %w", err)
	}
	return scan(rows)
}

const columns = `id, kind, at, parent_id, root_id, status, raised_by, raised_at, done_at`

func scan(rows *sql.Rows) ([]Event, error) {
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var ev Event
		var kind, status string
		var parent, root sql.NullString
		var done sql.NullTime
		if err := rows.Scan(&ev.ID, &kind, &ev.At, &parent, &root, &status, &ev.RaisedBy, &ev.RaisedAt, &done); err != nil {
			return nil, fmt.Errorf("events: scan: %w", err)
		}
		ev.Kind, ev.Status, ev.At, ev.RaisedAt = Kind(kind), Status(status), ev.At.UTC(), ev.RaisedAt.UTC()
		ev.ParentID, ev.RootID = parent.String, root.String
		if done.Valid {
			ev.DoneAt = done.Time.UTC()
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}
