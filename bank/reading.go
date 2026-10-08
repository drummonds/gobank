package bank

import (
	"context"
	"log"
	"sync"
	"time"
)

// A reading is a figure read from the database in the background. Pages
// get the last reading straight away, however long the read takes; a
// reading older than its ttl is taken again behind the page that found
// it stale, one read in flight at a time. Only a reading never taken, or
// one a command has invalidated, makes a page wait: what follows a
// command is what the command did.
type reading[T any] struct {
	read func(context.Context) (T, error)
	ttl  time.Duration

	mu       sync.Mutex
	value    T
	at       time.Time
	valid    bool          // value may be served
	gen      uint64        // bumped by every invalidation and fresh read: an older read in flight is discarded
	inflight chan struct{} // closed when the read in flight ends; nil when none
	next     chan struct{} // closed when the read queued behind it ends; nil when none
}

func newReading[T any](read func(context.Context) (T, error), ttl time.Duration) *reading[T] {
	return &reading[T]{read: read, ttl: ttl}
}

// get is the reading as of now: the last value while it is valid, with a
// read started behind it when it has expired; otherwise the value of the
// read that makes it valid again, once that ends.
func (r *reading[T]) get(ctx context.Context) T {
	r.mu.Lock()
	if r.valid && time.Since(r.at) < r.ttl {
		defer r.mu.Unlock()
		return r.value
	}
	if r.valid {
		r.startLocked()
		defer r.mu.Unlock()
		return r.value
	}
	done := r.next
	if done == nil {
		done = r.inflight
	}
	if done == nil {
		done = r.startLocked()
	}
	r.mu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.value
}

// fresh reads now, in front of the caller, and stores what it read over
// anything in flight.
func (r *reading[T]) fresh(ctx context.Context) T {
	r.mu.Lock()
	r.gen++
	r.mu.Unlock()
	v, err := r.read(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		log.Print(err)
		return r.value
	}
	r.value, r.at, r.valid = v, time.Now(), true
	return v
}

// invalidate marks the reading as not to be served: the next get waits
// for a read started from now, which a read already in flight queues
// behind itself.
func (r *reading[T]) invalidate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.valid = false
	r.gen++
	if r.inflight == nil {
		r.startLocked()
		return
	}
	if r.next == nil {
		r.next = make(chan struct{})
	}
}

// expire marks the reading as stale: the next get serves it one last time
// and starts a new one behind.
func (r *reading[T]) expire() {
	r.mu.Lock()
	r.at = time.Time{}
	r.mu.Unlock()
}

// startLocked starts a read in the background unless one is in flight,
// and returns the channel that closes when it ends. Must be called with
// mu held.
func (r *reading[T]) startLocked() chan struct{} {
	if r.inflight != nil {
		return r.inflight
	}
	done := make(chan struct{})
	r.inflight = done
	go func() {
		for {
			r.mu.Lock()
			started := r.gen
			r.mu.Unlock()
			v, err := r.read(context.Background())
			if err != nil {
				log.Print(err)
			}
			r.mu.Lock()
			if err == nil && r.gen == started {
				r.value, r.at, r.valid = v, time.Now(), true
			}
			queued := r.next
			r.inflight, r.next = queued, nil
			r.mu.Unlock()
			close(done)
			if queued == nil {
				return
			}
			done = queued
		}
	}()
	return done
}
