package bff

import (
	"sync"
	"time"
)

// failureLimiter locks a key (customer ID or client address) after too many
// login failures inside a sliding window.
type failureLimiter struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	failures map[string][]time.Time
	now      func() time.Time
}

func newFailureLimiter(max int, window time.Duration, now func() time.Time) *failureLimiter {
	return &failureLimiter{max: max, window: window, failures: map[string][]time.Time{}, now: now}
}

// prune drops failures outside the window and returns the remaining ones.
func (l *failureLimiter) prune(key string) []time.Time {
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

// retryAfter returns how long the key stays locked, or zero if it is not.
func (l *failureLimiter) retryAfter(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := l.prune(key)
	if len(kept) < l.max {
		return 0
	}
	return kept[0].Add(l.window).Sub(l.now())
}

func (l *failureLimiter) fail(key string) {
	l.mu.Lock()
	l.prune(key)
	l.failures[key] = append(l.failures[key], l.now())
	l.mu.Unlock()
}

func (l *failureLimiter) reset(key string) {
	l.mu.Lock()
	delete(l.failures, key)
	l.mu.Unlock()
}
