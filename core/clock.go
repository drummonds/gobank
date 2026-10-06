package core

import "time"

// The bank reads two things from outside itself: the time and market
// data (ADR-0002, events, the clock and market data). In a real deployment
// the clock is the wall clock and the base rate is a feed; in simulation
// the clock is warped forward and the base rate is a historical series
// replayed. The bank stamps every banking fact (payments, value dates,
// join dates) from the clock and takes its business day from it;
// operational records (restarts, schema versions) keep the wall clock.

// Clock is where the bank reads the time.
type Clock interface {
	Now() time.Time
}

// ClockFunc adapts a function to Clock.
type ClockFunc func() time.Time

// Now implements Clock.
func (f ClockFunc) Now() time.Time { return f() }

// BaseRateSource is where the bank reads the Bank of England base rate,
// as an annual decimal (0.0525 is 5.25%), in effect on a day.
type BaseRateSource interface {
	BaseRate(day time.Time) float64
}

// BaseRateFunc adapts a function to BaseRateSource.
type BaseRateFunc func(day time.Time) float64

// BaseRate implements BaseRateSource.
func (f BaseRateFunc) BaseRate(day time.Time) float64 { return f(day) }

// BusinessDay is the business day an instant falls on: its UTC calendar
// day. A business day is a UTC date, whatever the location.
func BusinessDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
