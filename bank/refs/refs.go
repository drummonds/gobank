// Package refs allocates the human-readable references a component gives
// the things it creates: customer numbers, payment numbers (ADR-0004).
// A reference is not an identity: it is a sequential label, and the only
// thing that needs allocating. Each package owns its own Sequence; block
// allocation and per-region prefixes go behind this same seam.
package refs

import "sync"

// Sequence numbers one kind of thing, from the number after the last on
// record.
type Sequence struct {
	mu   sync.Mutex
	next int
}

// Resume is a sequence continuing after last, the highest number on
// record; zero when there are none.
func Resume(last int) *Sequence {
	return &Sequence{next: last + 1}
}

// Next allocates the next number. A number once given is used, whether
// or not the thing it was given to is ever recorded.
func (s *Sequence) Next() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.next
	s.next++
	return n
}
