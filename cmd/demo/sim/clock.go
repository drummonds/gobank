package sim

import (
	"sync"
	"time"
)

// Clock is the simulation's clock (ADR-0002 stage 4): the bank reads the
// time from it as it would from the wall clock, and the simulation warps
// it. A simulated day begins at the start of its slot and the day length
// sets how fast simulated time passes in it: an hour into a two-hour day
// is noon. Flat out, simulated time passes at the wall's pace. The clock
// stays inside its day until the next begins, however long the slot runs
// on. Its day and the slot's start are recorded with the run (Store), so
// the next process resumes the same simulated day.
type Clock struct {
	mu        sync.Mutex
	wall      func() time.Time     // the wall clock, injectable
	dayLength func() time.Duration // the slot's length; zero is flat out
	length    time.Duration        // what dayLength reads when nothing else is wired
	day       time.Time            // the simulated day in progress (UTC midnight)
	slotStart time.Time            // the wall time the day began
}

// NewClock is a clock on no day yet; BeginDay or Resume sets one.
func NewClock(wall func() time.Time) *Clock {
	c := &Clock{wall: wall}
	c.dayLength = func() time.Duration { c.mu.Lock(); defer c.mu.Unlock(); return c.length }
	return c
}

// Now implements core.Clock: the simulated time of day.
func (c *Clock) Now() time.Time {
	length := c.dayLength()
	c.mu.Lock()
	defer c.mu.Unlock()
	elapsed := max(c.wall().Sub(c.slotStart), 0)
	if length > 0 {
		elapsed = time.Duration(float64(elapsed) * float64(24*time.Hour) / float64(length))
	}
	return c.day.Add(min(elapsed, 24*time.Hour-time.Second))
}

// Day is the simulated day in progress.
func (c *Clock) Day() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.day
}

// SlotStart is the wall time the day in progress began.
func (c *Clock) SlotStart() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.slotStart
}

// BeginDay starts day now: its slot begins at this wall time.
func (c *Clock) BeginDay(day time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.day, c.slotStart = day, c.wall()
}

// RestartSlot begins the day in progress again from now: a resumed day's
// slot, or a slot the bank spends finishing the day before.
func (c *Clock) RestartSlot() {
	c.mu.Lock()
	c.slotStart = c.wall()
	c.mu.Unlock()
}

// Resume puts the clock back on a day a previous process began; a zero
// slot start (a run recorded before the clock) starts the slot now.
func (c *Clock) Resume(day, slotStart time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.day = day
	c.slotStart = slotStart
	if slotStart.IsZero() {
		c.slotStart = c.wall()
	}
}

// SetDayLength sets the slot length the clock warps by, when nothing else
// is wired to answer dayLength.
func (c *Clock) SetDayLength(d time.Duration) {
	c.mu.Lock()
	c.length = d
	c.mu.Unlock()
}
