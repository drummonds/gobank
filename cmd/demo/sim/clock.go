package sim

import (
	"sync"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
)

// Clock is the simulation's clock (ADR-0010): stepped, not flowing. It
// stands at an event's instant while the bank processes the event and
// everything it causes, and moves to the next event's instant only when
// the bank is at rest. The bank reads it as it would the wall (core.Clock)
// and sees the same instant until the tick, so a cascade cut short by a
// restart is finished at the same instant by the next process: the
// instant is recorded with the run (Store).
type Clock struct {
	mu sync.Mutex
	at time.Time
}

// NewClock is a clock at no instant yet; Set puts it at one.
func NewClock() *Clock { return &Clock{} }

// Now implements core.Clock: the instant the clock stands at.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

// Day is the simulated day the clock stands in.
func (c *Clock) Day() time.Time { return core.BusinessDay(c.Now()) }

// Set is the tick: the clock moves to at and stands there.
func (c *Clock) Set(at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = at
}
