package sim

import (
	"context"
	"log"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
)

// The timeline (ADR-0010): the future events the simulation knows of, in
// instant order, rebuilt from stored facts at every tick. A tick moves
// the clock to the next event's instant and raises what is due there;
// the bank then processes the event and everything it causes, and the
// clock stands until the bank is at rest.

// EventKind names what happens at an instant.
type EventKind string

const (
	EventStartOfDay  EventKind = "start of day"
	EventPublication EventKind = "publication"  // the series publishes a change
	EventDecision    EventKind = "decision day" // the market's calendar: a change or nothing is published
	EventPayment     EventKind = "payment"      // the generator's
	EventSetting     EventKind = "setting"      // a staff setting becomes effective
)

// Event is one thing that happens at an instant.
type Event struct {
	Kind EventKind
	At   time.Time
}

// nextEvent is the first event after the clock's instant: the next
// start of day, or whatever the series, the markets' calendars, the
// settings staff made and the payments generator put before it. Two
// events at one instant are one tick: raise handles everything due.
func (s *Simulation) nextEvent(ctx context.Context) Event {
	now := s.clock.Now()
	next := Event{Kind: EventStartOfDay, At: s.clock.Day().AddDate(0, 0, 1)}
	sooner := func(kind EventKind, at time.Time) {
		if at.After(now) && at.Before(next.At) {
			next = Event{Kind: kind, At: at}
		}
	}
	s.mu.Lock()
	i := s.published
	s.mu.Unlock()
	if i < len(BoERates) {
		sooner(EventPublication, BoERates[i].At)
	}
	if markets, err := s.bank.Markets(ctx); err != nil {
		log.Printf("sim: markets: %v", err)
	} else {
		for _, m := range markets {
			if !m.NextDecision.IsZero() {
				sooner(EventDecision, m.NextDecision)
			}
		}
	}
	if upcoming, err := s.bank.UpcomingSettings(ctx); err != nil {
		log.Printf("sim: upcoming settings: %v", err)
	} else if len(upcoming) > 0 {
		sooner(EventSetting, upcoming[0].Effective)
	}
	if at, ok := s.nextPayment(now); ok {
		sooner(EventPayment, at)
	}
	return next
}

// raise is the tick to an event: the clock moves to its instant, what
// is due there is raised on the bank (the series' publications, the
// generator's payments), and the bank is driven until it is at rest or
// stuck. The run is recorded once the bank has begun, so the record
// never makes the bank wait on the database.
func (s *Simulation) raise(ctx context.Context, e Event) {
	s.mu.Lock()
	s.lastTick = s.wall()
	s.mu.Unlock()
	s.clock.Set(e.At)
	s.setDayEnd(e.At)
	s.publishDue(ctx, e.At.Add(time.Nanosecond))
	s.sendDuePayments(ctx, e.At)
	s.drive(ctx)
	s.saveRun()
}

// level brings the bank level with the clock before a tick: a bank ahead
// of the clock (a stale record) pulls the clock up to its day, and
// whatever the bank owes at the instant is driven. Reports whether the
// bank was at rest on entry, so a console step that found work to do
// stops there.
func (s *Simulation) level(ctx context.Context) (wasAtRest bool) {
	pos, err := s.bank.Position(ctx)
	if err != nil {
		return false
	}
	if pos.Day.After(s.clock.Day()) {
		s.clock.Set(pos.Day)
	}
	owed, err := s.bank.Owed(ctx)
	if err != nil {
		return false
	}
	s.mu.Lock()
	s.owed = owed
	s.mu.Unlock()
	if owed.AtRest() {
		return true
	}
	s.drive(ctx)
	s.saveRun()
	return false
}

// drive has the bank follow the clock until it reports itself at rest
// (StartDay: the pass, the days, the parameter changes), or until a call
// makes no progress, when the bank is stuck (a silent market, accounts on
// rules this build lacks) and its refusal is kept for the console. Stops
// early when ctx ends, leaving the rest owed.
func (s *Simulation) drive(ctx context.Context) bool {
	var last core.Owed
	for i := 0; ctx.Err() == nil; i++ {
		_, err := s.bank.StartDay(ctx)
		s.mu.Lock()
		s.dayRefused = ""
		if err != nil && ctx.Err() == nil {
			s.dayRefused = err.Error()
		}
		s.mu.Unlock()
		owed, oerr := s.bank.Owed(ctx)
		if oerr != nil {
			log.Printf("sim: owed: %v", oerr)
			return false
		}
		s.mu.Lock()
		s.owed = owed
		s.mu.Unlock()
		if owed.AtRest() {
			return true
		}
		if err != nil || (i > 0 && owed == last) {
			return false // stuck: nothing more the bank will do at this instant
		}
		last = owed
	}
	return false
}

// atRest is the bank as the simulation last saw it.
func (s *Simulation) atRest() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.owed.AtRest()
}
