package main

import (
	"sync"
	"time"
)

// AuthStore manages simulated PII authorization sessions.
type AuthStore struct {
	mu       sync.Mutex
	sessions map[string]time.Time
	ttl      time.Duration
}

func NewAuthStore(ttl time.Duration) *AuthStore {
	return &AuthStore{
		sessions: make(map[string]time.Time),
		ttl:      ttl,
	}
}

func (as *AuthStore) Authorize(sessionID string) {
	as.mu.Lock()
	as.sessions[sessionID] = time.Now().Add(as.ttl)
	as.mu.Unlock()
}

func (as *AuthStore) Revoke(sessionID string) {
	as.mu.Lock()
	delete(as.sessions, sessionID)
	as.mu.Unlock()
}

func (as *AuthStore) IsAuthorized(sessionID string) bool {
	as.mu.Lock()
	defer as.mu.Unlock()
	expiry, ok := as.sessions[sessionID]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(as.sessions, sessionID)
		return false
	}
	return true
}
