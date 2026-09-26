package bff

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"sync"
	"time"
)

type session struct {
	customerID string
	created    time.Time
	lastSeen   time.Time
}

// SessionStore issues and validates bearer tokens. Tokens are random and only
// their hash is kept, so a memory dump does not yield usable tokens. A session
// ends at the absolute TTL or after the idle timeout, whichever is first.
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]*session
	ttl      time.Duration
	idle     time.Duration
	now      func() time.Time
}

// NewSessionStore returns a store with the given absolute and idle lifetimes.
func NewSessionStore(ttl, idle time.Duration, now func() time.Time) *SessionStore {
	if now == nil {
		now = time.Now
	}
	return &SessionStore{sessions: map[string]*session{}, ttl: ttl, idle: idle, now: now}
}

// Create starts a session for a customer and returns the token and its
// absolute expiry.
func (s *SessionStore) Create(customerID string) (token string, expires time.Time) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		panic("bff: crypto/rand failed: " + err.Error())
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	t := s.now()
	s.mu.Lock()
	s.sessions[hashToken(token)] = &session{customerID: customerID, created: t, lastSeen: t}
	s.mu.Unlock()
	return token, t.Add(s.ttl)
}

// Lookup returns the customer for a token and refreshes its idle timer.
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
	if t.After(sess.created.Add(s.ttl)) || t.After(sess.lastSeen.Add(s.idle)) {
		delete(s.sessions, key)
		return "", false
	}
	sess.lastSeen = t
	return sess.customerID, true
}

// Revoke ends a session. Unknown tokens are ignored.
func (s *SessionStore) Revoke(token string) {
	s.mu.Lock()
	delete(s.sessions, hashToken(token))
	s.mu.Unlock()
}

// Sweep drops expired sessions. Call it periodically from a goroutine.
func (s *SessionStore) Sweep() {
	t := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, sess := range s.sessions {
		if t.After(sess.created.Add(s.ttl)) || t.After(sess.lastSeen.Add(s.idle)) {
			delete(s.sessions, k)
		}
	}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
