package main

import (
	"context"
	"sync"
	"time"
)

// The simulation's clock (ADR-0002 stage 4): the bank reads the time from
// it as it would from the wall clock, and the simulation warps it. A
// simulated day begins at the start of its slot and the day length sets
// how fast simulated time passes in it: an hour into a two-hour day is
// noon. Flat out, simulated time passes at the wall's pace. The clock
// stays inside its day until the next begins, however long the slot runs
// on. Its day and the slot's start are on the run row, so the next
// process resumes the same simulated day.

type simClock struct {
	mu        sync.Mutex
	wall      func() time.Time               // the wall clock, injectable
	dayLength func() time.Duration           // the slot's length; zero is flat out
	length    time.Duration                  // what dayLength reads when nothing else is wired
	day       time.Time                      // the simulated day in progress (UTC midnight)
	slotStart time.Time                      // the wall time the day began
	persist   func(day, slotStart time.Time) // records the day in progress and its slot start, if wired
}

// newSimClock is a clock on no day yet; beginDay or resume sets one.
func newSimClock(wall func() time.Time) *simClock {
	c := &simClock{wall: wall}
	c.dayLength = func() time.Duration { c.mu.Lock(); defer c.mu.Unlock(); return c.length }
	return c
}

// Now implements core.Clock: the simulated time of day.
func (c *simClock) Now() time.Time {
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
func (c *simClock) Day() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.day
}

// beginDay starts day now: its slot begins at this wall time.
func (c *simClock) beginDay(day time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.day, c.slotStart = day, c.wall()
}

// record writes the day in progress and its slot start, if wired.
func (c *simClock) record() {
	c.mu.Lock()
	persist, day, slotStart := c.persist, c.day, c.slotStart
	c.mu.Unlock()
	if persist != nil {
		persist(day, slotStart)
	}
}

// restartSlot begins the day in progress again from now: a resumed day's
// slot, or a slot the bank spends finishing the day before.
func (c *simClock) restartSlot() {
	c.mu.Lock()
	c.slotStart = c.wall()
	c.mu.Unlock()
}

// resume puts the clock back on a day a previous process began; a zero
// slot start (a run row from before the clock) starts the slot now.
func (c *simClock) resume(day, slotStart time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.day = day
	c.slotStart = slotStart
	if slotStart.IsZero() {
		c.slotStart = c.wall()
	}
}

// setDayLength sets the slot length the clock warps by, when nothing else
// is wired to answer dayLength.
func (c *simClock) setDayLength(d time.Duration) {
	c.mu.Lock()
	c.length = d
	c.mu.Unlock()
}

// nextDayCtx is one slot of the simulation: the clock moves on a day when
// the bank is level with it and has finished its day, else the slot
// begins again on the day in progress (a resumed pass, a bank behind the
// clock); then the bank follows the clock. Stops early when ctx ends,
// leaving the rest of the day for the next slot.
func (ds *DemoState) nextDayCtx(ctx context.Context) {
	pos, err := ds.bank.Position(ctx)
	if err != nil {
		return
	}
	if pos.DayComplete && pos.Day.Equal(ds.simClock.Day()) {
		ds.simClock.beginDay(pos.Day.AddDate(0, 0, 1))
	} else {
		ds.simClock.restartSlot()
	}
	ds.bank.StartDay(ctx) //nolint:errcheck // a cut-short pass is resumed next slot
	// Recorded once the bank has begun the day, so the record never makes
	// the day wait on the database; a restart before this point starts the
	// slot again (resume).
	ds.simClock.record()
}

// advanceDay is one slot of the simulation, run to completion.
func (ds *DemoState) advanceDay() {
	ds.nextDayCtx(context.Background())
}
