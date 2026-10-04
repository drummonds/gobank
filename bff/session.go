package bff

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// Sessions is where the BFF keeps its live sessions. It issues and
// validates bearer tokens: a session ends at the absolute lifetime or
// after the idle timeout, whichever is first. Tokens are random and only
// their hash is kept, so neither a memory dump nor the database yields a
// usable token.
//
// The memory store (NewSessionStore) serves one process and forgets
// everything at restart. The SQL store (NewSQLSessions) keeps them in the
// bank's database (ADR-0002: every fact is stored), so a restart or an
// upgrade keeps customers logged in and several processes can share them.
type Sessions interface {
	// Create starts a session for a customer and returns the token and its
	// absolute expiry.
	Create(customerID string) (token string, expires time.Time)
	// Lookup returns the customer for a token and refreshes its idle timer.
	Lookup(token string) (customerID string, ok bool)
	// Revoke ends a session. Unknown tokens are ignored.
	Revoke(token string)
	// Sweep drops expired sessions. Call it periodically from a goroutine.
	Sweep()
}

// sessionPolicy is the lifetime rule both stores apply.
type sessionPolicy struct {
	ttl  time.Duration
	idle time.Duration
	now  func() time.Time
}

func newPolicy(ttl, idle time.Duration, now func() time.Time) sessionPolicy {
	if now == nil {
		now = time.Now
	}
	return sessionPolicy{ttl: ttl, idle: idle, now: now}
}

// expired reports whether a session created at created and last used at
// lastSeen is over at t.
func (p sessionPolicy) expired(created, lastSeen, t time.Time) bool {
	return t.After(created.Add(p.ttl)) || t.After(lastSeen.Add(p.idle))
}

func (p sessionPolicy) mint() string {
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

type session struct {
	customerID string
	created    time.Time
	lastSeen   time.Time
}

// SessionStore keeps sessions in memory, for one process.
type SessionStore struct {
	sessionPolicy
	mu       sync.Mutex
	sessions map[string]*session
}

// NewSessionStore returns a memory store with the given absolute and idle
// lifetimes.
func NewSessionStore(ttl, idle time.Duration, now func() time.Time) *SessionStore {
	return &SessionStore{sessionPolicy: newPolicy(ttl, idle, now), sessions: map[string]*session{}}
}

func (s *SessionStore) Create(customerID string) (token string, expires time.Time) {
	token = s.mint()
	t := s.now()
	s.mu.Lock()
	s.sessions[hashToken(token)] = &session{customerID: customerID, created: t, lastSeen: t}
	s.mu.Unlock()
	return token, t.Add(s.ttl)
}

func (s *SessionStore) Lookup(token string) (customerID string, ok bool) {
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
	return sess.customerID, true
}

func (s *SessionStore) Revoke(token string) {
	s.mu.Lock()
	delete(s.sessions, hashToken(token))
	s.mu.Unlock()
}

func (s *SessionStore) Sweep() {
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

// SessionsSchema creates the sessions table. The program that owns the
// database runs it (the demo registers it as the sessions component's
// schema); it is idempotent.
var SessionsSchema = []string{`CREATE TABLE IF NOT EXISTS sessions (
	token_hash VARCHAR(64) PRIMARY KEY,
	customer_id VARCHAR(20) NOT NULL,
	created_at TIMESTAMP NOT NULL,
	last_seen_at TIMESTAMP NOT NULL
)`}

// SQLSessions keeps sessions in a database table (SessionsSchema).
type SQLSessions struct {
	sessionPolicy
	db  *sql.DB
	log *slog.Logger
}

// NewSQLSessions returns a store over db, whose sessions table exists.
func NewSQLSessions(db *sql.DB, ttl, idle time.Duration, now func() time.Time) *SQLSessions {
	return &SQLSessions{sessionPolicy: newPolicy(ttl, idle, now), db: db, log: slog.Default()}
}

func (s *SQLSessions) Create(customerID string) (token string, expires time.Time) {
	token = s.mint()
	t := s.now().UTC()
	if _, err := s.db.Exec(`INSERT INTO sessions (token_hash, customer_id, created_at, last_seen_at) VALUES ($1, $2, $3, $4)`,
		hashToken(token), customerID, t, t); err != nil {
		s.log.Error("bff.sessions.create", "err", err)
	}
	return token, t.Add(s.ttl)
}

func (s *SQLSessions) Lookup(token string) (customerID string, ok bool) {
	if token == "" {
		return "", false
	}
	key := hashToken(token)
	t := s.now().UTC()
	var created, lastSeen time.Time
	err := s.db.QueryRow(`SELECT customer_id, created_at, last_seen_at FROM sessions WHERE token_hash = $1`, key).
		Scan(&customerID, &created, &lastSeen)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", false
	case err != nil:
		s.log.Error("bff.sessions.lookup", "err", err)
		return "", false
	}
	if s.expired(created.UTC(), lastSeen.UTC(), t) {
		s.Revoke(token)
		return "", false
	}
	if _, err := s.db.Exec(`UPDATE sessions SET last_seen_at = $1 WHERE token_hash = $2`, t, key); err != nil {
		s.log.Error("bff.sessions.touch", "err", err)
	}
	return customerID, true
}

func (s *SQLSessions) Revoke(token string) {
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = $1`, hashToken(token)); err != nil {
		s.log.Error("bff.sessions.revoke", "err", err)
	}
}

func (s *SQLSessions) Sweep() {
	t := s.now().UTC()
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE created_at < $1 OR last_seen_at < $2`, t.Add(-s.ttl), t.Add(-s.idle)); err != nil {
		s.log.Error("bff.sessions.sweep", "err", err)
	}
}
