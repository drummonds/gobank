// Package session is where the BFF keeps who is signed in: the customer
// app's sessions and the staff web's (ADR-0002 stage 2 story f, stage 7
// story 1.7.1), and the lockout that guards both logins. It is the BFF's
// and the staff web's shared machinery, not a bank component.
package session

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Store is where the BFF keeps its live sessions: the customer app's
// and, from story 1.7.1, the staff web's. It issues and validates bearer
// tokens: a session ends at the absolute lifetime or after the idle
// timeout, whichever is first. Tokens are random and only their hash is
// kept, so neither a memory dump nor the database yields a usable token.
// The subject of a session is whose it is: a customer ID in the customer
// store, a user ID in the staff store.
//
// The memory store (NewMemory) serves one process and forgets everything
// at restart. The SQL store (NewSQL) keeps them in the
// bank's database (ADR-0002: every fact is stored), so a restart or an
// upgrade keeps everyone logged in and several processes can share them.
type Store interface {
	// Create starts a session for a subject and returns the token and its
	// absolute expiry.
	Create(subject string) (token string, expires time.Time)
	// Lookup returns the subject of a token and refreshes its idle timer.
	Lookup(token string) (subject string, ok bool)
	// Revoke ends a session. Unknown tokens are ignored.
	Revoke(token string)
	// Sweep drops expired sessions. Call it periodically from a goroutine.
	Sweep()
}

// policy is the lifetime rule both stores apply.
type policy struct {
	ttl  time.Duration
	idle time.Duration
	now  func() time.Time
}

func newPolicy(ttl, idle time.Duration, now func() time.Time) policy {
	if now == nil {
		now = time.Now
	}
	return policy{ttl: ttl, idle: idle, now: now}
}

// expired reports whether a session created at created and last used at
// lastSeen is over at t.
func (p policy) expired(created, lastSeen, t time.Time) bool {
	return t.After(created.Add(p.ttl)) || t.After(lastSeen.Add(p.idle))
}

func (p policy) mint() string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		panic("bff: crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// --- In memory ---

type entry struct {
	subject  string
	created  time.Time
	lastSeen time.Time
}

// Memory keeps sessions in memory, for one process.
type Memory struct {
	policy
	mu       sync.Mutex
	sessions map[string]*entry
}

// NewMemory returns a memory store with the given absolute and idle
// lifetimes.
func NewMemory(ttl, idle time.Duration, now func() time.Time) *Memory {
	return &Memory{policy: newPolicy(ttl, idle, now), sessions: map[string]*entry{}}
}

func (s *Memory) Create(subject string) (token string, expires time.Time) {
	token = s.mint()
	t := s.now()
	s.mu.Lock()
	s.sessions[hashToken(token)] = &entry{subject: subject, created: t, lastSeen: t}
	s.mu.Unlock()
	return token, t.Add(s.ttl)
}

func (s *Memory) Lookup(token string) (subject string, ok bool) {
	if token == "" {
		return "", false
	}
	key := hashToken(token)
	t := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, found := s.sessions[key]
	if !found {
		return "", false
	}
	if s.expired(sess.created, sess.lastSeen, t) {
		delete(s.sessions, key)
		return "", false
	}
	sess.lastSeen = t
	return sess.subject, true
}

func (s *Memory) Revoke(token string) {
	s.mu.Lock()
	delete(s.sessions, hashToken(token))
	s.mu.Unlock()
}

func (s *Memory) Sweep() {
	t := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, sess := range s.sessions {
		if s.expired(sess.created, sess.lastSeen, t) {
			delete(s.sessions, k)
		}
	}
}

// --- In the database ---

// Table is where a SQL store keeps its sessions: the table and the column
// that holds the subject. Customers' and staff's sessions are kept apart,
// so a token of one is never a session of the other.
type Table struct {
	Name    string
	Subject string
}

var (
	Customers = Table{Name: "sessions", Subject: "customer_id"}
	Staff     = Table{Name: "staff_sessions", Subject: "user_id"}
)

// Schema creates the table. The program that owns the database runs it
// (the demo registers it as the sessions component's schema); it is
// idempotent.
func (t Table) Schema() []string {
	return []string{fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
	token_hash VARCHAR(64) PRIMARY KEY,
	%s VARCHAR(64) NOT NULL,
	created_at TIMESTAMP NOT NULL,
	last_seen_at TIMESTAMP NOT NULL
)`, t.Name, t.Subject)}
}

// CustomersSchema creates the customer sessions table as the sessions
// component's first migration wrote it.
var CustomersSchema = []string{`CREATE TABLE IF NOT EXISTS sessions (
	token_hash VARCHAR(64) PRIMARY KEY,
	customer_id VARCHAR(20) NOT NULL,
	created_at TIMESTAMP NOT NULL,
	last_seen_at TIMESTAMP NOT NULL
)`}

// StaffSchema creates the staff sessions table.
var StaffSchema = Staff.Schema()

// SQL keeps sessions in a database table.
type SQL struct {
	policy
	db  func() *sql.DB
	log *slog.Logger

	insert, sel, touch, del, sweep string
}

// NewSQL returns a store over table in the database db gives,
// whose table exists. It is asked on each use: the program that owns the
// database may replace it (the demo's reset and import), and the
// sessions go with the run they belonged to.
func NewSQL(table Table, db func() *sql.DB, ttl, idle time.Duration, now func() time.Time) *SQL {
	return &SQL{
		policy: newPolicy(ttl, idle, now), db: db, log: slog.Default(),
		insert: fmt.Sprintf(`INSERT INTO %s (token_hash, %s, created_at, last_seen_at) VALUES ($1, $2, $3, $4)`, table.Name, table.Subject),
		sel:    fmt.Sprintf(`SELECT %s, created_at, last_seen_at FROM %s WHERE token_hash = $1`, table.Subject, table.Name),
		touch:  fmt.Sprintf(`UPDATE %s SET last_seen_at = $1 WHERE token_hash = $2`, table.Name),
		del:    fmt.Sprintf(`DELETE FROM %s WHERE token_hash = $1`, table.Name),
		sweep:  fmt.Sprintf(`DELETE FROM %s WHERE created_at < $1 OR last_seen_at < $2`, table.Name),
	}
}

func (s *SQL) Create(subject string) (token string, expires time.Time) {
	token = s.mint()
	t := s.now().UTC()
	if _, err := s.db().Exec(s.insert, hashToken(token), subject, t, t); err != nil {
		s.log.Error("session.create", "err", err)
	}
	return token, t.Add(s.ttl)
}

func (s *SQL) Lookup(token string) (subject string, ok bool) {
	if token == "" {
		return "", false
	}
	key := hashToken(token)
	t := s.now().UTC()
	var created, lastSeen time.Time
	err := s.db().QueryRow(s.sel, key).Scan(&subject, &created, &lastSeen)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", false
	case err != nil:
		s.log.Error("session.lookup", "err", err)
		return "", false
	}
	if s.expired(created.UTC(), lastSeen.UTC(), t) {
		s.Revoke(token)
		return "", false
	}
	if _, err := s.db().Exec(s.touch, t, key); err != nil {
		s.log.Error("session.touch", "err", err)
	}
	return subject, true
}

func (s *SQL) Revoke(token string) {
	if _, err := s.db().Exec(s.del, hashToken(token)); err != nil {
		s.log.Error("session.revoke", "err", err)
	}
}

func (s *SQL) Sweep() {
	t := s.now().UTC()
	if _, err := s.db().Exec(s.sweep, t.Add(-s.ttl), t.Add(-s.idle)); err != nil {
		s.log.Error("session.sweep", "err", err)
	}
}
