package gobank

import "time"

// Clock provides the current time, allowing simulation to control time progression.
type Clock interface {
	Now() time.Time
}

// WallClock uses real system time.
type WallClock struct{}

func (WallClock) Now() time.Time { return time.Now() }

// SimClock is a controllable clock for testing and simulation.
type SimClock struct {
	current time.Time
}

func NewSimClock(start time.Time) *SimClock {
	return &SimClock{current: start}
}

func (c *SimClock) Now() time.Time { return c.current }

func (c *SimClock) Advance(d time.Duration) {
	c.current = c.current.Add(d)
}

func (c *SimClock) SetDate(t time.Time) {
	c.current = t
}
