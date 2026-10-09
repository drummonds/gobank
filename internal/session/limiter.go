package session

import (
	"sync"
	"time"
)

// Limiter locks a key (a login name, a customer ID or a client address)
// after too many login failures inside a sliding window. The customer
// login and the staff web's share it.
type Limiter struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	failures map[string][]time.Time
	now      func() time.Time
}

// NewLimiter locks a key for window after max failures in it.
func NewLimiter(max int, window time.Duration, now func() time.Time) *Limiter {
	return &Limiter{max: max, window: window, failures: map[string][]time.Time{}, now: now}
}

// prune drops failures outside the window and returns the remaining ones.
func (l *Limiter) prune(key string) []time.Time {
	cutoff := l.now().Add(-l.window)
	kept := l.failures[key][:0]
	for _, t := range l.failures[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, key)
	} else {
		l.failures[key] = kept
	}
	return kept
}

// RetryAfter returns how long the key stays locked, or zero if it is not.
func (l *Limiter) RetryAfter(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := l.prune(key)
	if len(kept) < l.max {
		return 0
	}
	return kept[0].Add(l.window).Sub(l.now())
}

// Fail records a failure against the key.
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	l.prune(key)
	l.failures[key] = append(l.failures[key], l.now())
	l.mu.Unlock()
}

// Reset forgets the key's failures, on a successful login.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.failures, key)
	l.mu.Unlock()
}
