// Package sim is the simulation (ADR-0002 stage 4, ADR-0010): the
// generators that make a population and its payments, the stepped clock
// the bank reads, the market's series replayed as the feed, and the run
// loop and console knobs that drive them. The simulation is
// discrete-event: a timeline of the start of each day, the market's
// decision days and publications, the generator's payments and the
// settings staff make; the clock stands at an event's instant while the
// bank processes it and everything it causes, and moves to the next
// event only when the bank reports itself at rest (core.Owed). It is
// built on the core alone: it raises events on the bank through
// core.Commands and decides them from core.StaffQueries, and knows
// nothing else of the bank (cmd/demo's TestSimulationImportsOnlyTheCore).
// A real bank has none of this; the console that drives it is part of
// the staff web app.
package sim

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
)

// Bank is the core as the simulation sees it: commands to raise events,
// staff queries to decide them.
type Bank interface {
	core.Commands
	core.StaffQueries
}

// OpeningDay is the simulated day a fresh run begins on.
var OpeningDay = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

// Settings are the simulation console's knobs: what an operator sets on
// the settings page or in the environment. The bank's own parameters are
// the bank's, not settings.
type Settings struct {
	MaxCustomers   int
	DayLength      time.Duration // wall-clock length of a simulated day; zero is flat out
	PaymentsPerDay int           // the generator's payments a simulated day, while it runs
}

// DefaultSettings is a population cap, flat-out days and ten payments a
// day.
func DefaultSettings() Settings {
	return Settings{MaxCustomers: 1_000_000, PaymentsPerDay: 10}
}

// Run is the run's place in time as the store keeps it: the instant the
// clock stands at, how many days the bank has run, and whether the run
// loop was going, so a restart picks the run up at the same instant and
// the bank finishes the same cascade.
type Run struct {
	At       time.Time
	DayCount int
	Running  bool
}

// Store is where the run is recorded between processes; the wiring
// implements it over the simulation's own table.
type Store interface {
	SaveRun(Run)
	SaveDayLength(time.Duration)
}

// Options wire a Simulation.
type Options struct {
	Wall     func() time.Time // the wall clock; nil is time.Now
	Workers  int              // concurrent customer openings in a batch add; at least one
	Pause    func() bool      // asked before each event; true stops the run (a memory limit, say)
	Settings Settings         // the console's starting knobs; zero means DefaultSettings
	Seed     int64            // the generators' randomness
}

// Simulation drives a Bank: the clock, the timeline, the generators and
// the run loop.
type Simulation struct {
	mu       sync.Mutex
	bank     Bank
	clock    *Clock
	store    Store
	wall     func() time.Time
	workers  int
	pause    func() bool
	seed     int64
	rng      *rand.Rand // the generators' randomness (mu)
	settings settings

	running           bool
	cancel            context.CancelFunc
	loopDone          chan struct{} // closed when the run loop's goroutine has exited (mu)
	shuttingDown      bool          // the loop was stopped by Shutdown, not the operator: the run is still on (mu)
	resumedRunning    bool          // the run was going when the previous process stopped
	dayLengthRecorded bool          // the console set the day length; it is on the run and beats the environment's (mu)
	dayLengthChanged  chan struct{} // a console change of the day length, for the run loop's wait (one pending at most)
	lastTick          time.Time     // the wall time of the last tick in this process; zero before the first (mu)
	lastInstant       time.Time     // the instant the clock stood at before the last tick (mu)
	dayEndsAt         time.Time     // when the next start of day is due by the wall; zero when flat out or stopped (mu)
	next              Event         // the event the loop is waiting for (mu)
	owed              core.Owed     // the bank's owed counts as last seen (mu)

	payRunning bool // the generator's payments are on the timeline (mu)

	addingCustRunning  bool
	addingCustCancel   context.CancelFunc
	addingCustProgress int
	addingCustTarget   int
	addingCustStart    time.Time
	lastAddRate        float64 // customers/s of the last finished batch

	catalogue []core.Product // the generators' copy of the product catalogue, read once (mu)

	published  int    // the next of BoERates the feed has yet to publish; the whole series is replayed from a reset or a resume (mu)
	dayRefused string // the bank's refusal of the last StartDay asked of it, for the console (mu)
}

// New is a simulation over bank, its clock at the start of OpeningDay.
func New(bank Bank, store Store, opts Options) *Simulation {
	s := &Simulation{
		bank:             bank,
		store:            store,
		wall:             opts.Wall,
		workers:          max(opts.Workers, 1),
		pause:            opts.Pause,
		seed:             opts.Seed,
		rng:              rand.New(rand.NewSource(opts.Seed)),
		settings:         settings{v: opts.Settings},
		dayLengthChanged: make(chan struct{}, 1),
		clock:            NewClock(),
	}
	if s.wall == nil {
		s.wall = time.Now
	}
	if s.settings.v == (Settings{}) {
		s.settings.v = DefaultSettings()
	}
	s.clock.Set(OpeningDay)
	s.lastInstant = OpeningDay
	return s
}

// Clock is the clock the bank reads: the simulation's, stepped.
func (s *Simulation) Clock() *Clock { return s.clock }

// SetWall replaces the wall clock, for tests.
func (s *Simulation) SetWall(wall func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wall = wall
}

// Resume puts the simulation back where a previous process left it: the
// clock at its instant, the run loop to be started again if it was going
// (ResumedRunning), and the day length the console set, if any.
func (s *Simulation) Resume(run Run, dayLength time.Duration, dayLengthSet bool) {
	s.clock.Set(run.At)
	s.mu.Lock()
	s.lastInstant = run.At
	s.resumedRunning = run.Running
	s.rng = rand.New(rand.NewSource(42 + int64(run.DayCount)))
	if dayLengthSet {
		s.settings.Update(func(v *Settings) { v.DayLength = dayLength })
		s.dayLengthRecorded = true
	}
	s.published = 0
	s.mu.Unlock()
}

// CatchUp is the feed catching up with the clock once the bank is open:
// every publication of the series up to the clock's instant goes to the
// bank; those ahead are events on the timeline. A run from before the
// market was a parameter has none of its publications; a replay of those
// the bank has changes nothing.
func (s *Simulation) CatchUp() {
	s.publishDue(context.Background(), s.clock.Now().Add(time.Nanosecond))
}

// ResumedRunning reports whether the run was going when the previous
// process stopped; the caller starts the loop again.
func (s *Simulation) ResumedRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resumedRunning
}

// DayLengthRecorded reports whether the console set the day length, which
// then outlives the run it was made in.
func (s *Simulation) DayLengthRecorded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dayLengthRecorded
}

// Reset stops everything and puts the clock back at the start of day for
// a fresh run; the caller resets the bank, has the feed catch up
// (CatchUp) and records the run.
func (s *Simulation) Reset(day time.Time) {
	s.mu.Lock()
	if s.running {
		s.running = false
		if s.cancel != nil {
			s.cancel()
			s.cancel = nil
		}
	}
	s.payRunning = false
	if s.addingCustRunning {
		if s.addingCustCancel != nil {
			s.addingCustCancel()
		}
		s.finishAddingLocked()
	}
	s.lastAddRate = 0
	s.rng = rand.New(rand.NewSource(42))
	s.dayEndsAt = time.Time{}
	s.lastTick = time.Time{}
	s.lastInstant = day
	s.next = Event{}
	s.owed = core.Owed{}
	s.shuttingDown = false
	s.resumedRunning = false
	s.published = 0
	s.dayRefused = ""
	s.mu.Unlock()
	s.clock.Set(day)
}

// publishDue is the feed (ADR-0009): every publication of the series
// before the instant that is not yet published goes to the bank through
// the core, in order.
func (s *Simulation) publishDue(ctx context.Context, before time.Time) {
	for {
		s.mu.Lock()
		i := s.published
		s.mu.Unlock()
		if i >= len(BoERates) || !BoERates[i].At.Before(before) {
			return
		}
		if err := s.bank.Publish(ctx, BoERates[i]); err != nil {
			log.Printf("sim: publish %+v: %v", BoERates[i], err)
			return
		}
		s.mu.Lock()
		s.published = i + 1
		s.mu.Unlock()
	}
}

// --- The run loop ---

// Start begins the run: event by event. Before each tick the bank is
// brought level with the clock and must be at rest; the wait before the
// tick is the day length's share of the simulated gap to the next event
// (pace), so a day length set on the console applies to the wait in
// progress, and zero means the next event comes now. A bank that is
// stuck (not at rest and making no progress) holds the clock, retried
// at the flat-out gap, with the refusal on the console.
func (s *Simulation) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.cancel = cancel
	s.loopDone = done
	s.running = true
	s.shuttingDown = false
	s.lastTick = time.Time{} // the first event comes at once
	s.mu.Unlock()
	s.saveRun()

	go func() {
		defer close(done)
		for ctx.Err() == nil {
			if s.pause != nil && s.pause() {
				s.Stop()
				return
			}
			s.level(ctx)
			if !s.atRest() {
				select {
				case <-ctx.Done():
				case <-time.After(minDayGap):
				}
				continue
			}
			next := s.nextEvent(ctx)
			wait := s.paceFor(next)
			select {
			case <-ctx.Done():
				return
			case <-s.dayLengthChanged:
				continue // the wait is recomputed from the new length
			case <-time.After(wait):
				s.raise(ctx, next)
				if next.Kind == EventStartOfDay && ctx.Err() == nil {
					s.rollNewCustomer(ctx) // the day's chance of a new customer
				}
			}
		}
	}()
}

// paceFor is the wall time to wait before the tick to next, and records
// the event as the one in waiting.
func (s *Simulation) paceFor(next Event) time.Duration {
	dayLength := s.settings.Get().DayLength
	now := s.clock.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next = next
	var spent time.Duration
	if !s.lastTick.IsZero() {
		spent = s.wall().Sub(s.lastTick)
	} else if dayLength > 0 {
		spent = dayLength // a loop just started or resumed ticks at once
	}
	return pace(dayLength, next.At.Sub(now), spent, next.Kind == EventStartOfDay)
}

// setDayEnd records when the next start of day is due by the wall, from
// a tick to at: the day length's share of what is left of the day, or
// never when flat out.
func (s *Simulation) setDayEnd(at time.Time) {
	dayLength := s.settings.Get().DayLength
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dayEndsAt = time.Time{}
	if dayLength > 0 {
		left := core.BusinessDay(at).AddDate(0, 0, 1).Sub(at)
		s.dayEndsAt = s.lastTick.Add(pace(dayLength, left, 0, true))
	}
}

// Stop ends the run: the operator's stop, recorded as such.
func (s *Simulation) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.dayEndsAt = time.Time{}
	s.mu.Unlock()
	s.saveRun()
}

// Shutdown is the process ending, not the operator stopping the run: the
// loop is told to stop and the event in progress is waited for (bounded
// by ctx), so the database is consistent for the next process, which
// finds the run still recorded as running and carries it on.
func (s *Simulation) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	done := s.loopDone
	if s.running {
		s.running = false
		s.shuttingDown = true
		if s.cancel != nil {
			s.cancel()
			s.cancel = nil
		}
	}
	s.dayEndsAt = time.Time{}
	s.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsRunning reports whether the run loop is going.
func (s *Simulation) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Step is the console's step, unless Pause says not now: the bank is
// brought level with the clock, and if it was at rest already, every
// event up to and including the next start of day is raised, flat out.
// A bank that owed work on entry is driven and left there: a resumed
// pass is finished rather than the next day started. Stops early when
// ctx ends, leaving the rest for the next.
func (s *Simulation) Step(ctx context.Context) {
	if s.pause != nil && s.pause() {
		return
	}
	if !s.level(ctx) {
		return
	}
	for ctx.Err() == nil {
		if !s.atRest() {
			return
		}
		next := s.nextEvent(ctx)
		s.raise(ctx, next)
		if next.Kind == EventStartOfDay {
			return
		}
	}
}

// AdvanceDay is the console's step to the next day, then the generators'
// roll for a new customer.
func (s *Simulation) AdvanceDay() {
	ctx := context.Background()
	s.Step(ctx)
	s.rollNewCustomer(ctx)
}

// saveRun records the run as it is now.
func (s *Simulation) saveRun() {
	if s.store == nil {
		return
	}
	pos, _ := s.bank.Position(context.Background())
	s.mu.Lock()
	running := s.running || s.shuttingDown
	s.mu.Unlock()
	s.store.SaveRun(Run{At: s.clock.Now(), DayCount: pos.DayCount, Running: running})
}

// --- Settings ---

// settings holds the console settings behind Get and Update. Readers (the
// run loop, the generators, the pages) take a snapshot and never wait on
// the simulation's lock.
type settings struct {
	mu sync.Mutex
	v  Settings
}

func (s *settings) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.v
}

func (s *settings) Update(fn func(*Settings)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.v)
}

// Settings is a snapshot of the console settings.
func (s *Simulation) Settings() Settings { return s.settings.Get() }

// SetMaxCustomers sets the population cap.
func (s *Simulation) SetMaxCustomers(n int) {
	if n > 0 {
		s.settings.Update(func(v *Settings) { v.MaxCustomers = n })
	}
}

// SetPaymentsPerDay sets how many payments the generator puts on each
// simulated day while it runs; zero or less is refused.
func (s *Simulation) SetPaymentsPerDay(n int) {
	if n > 0 {
		s.settings.Update(func(v *Settings) { v.PaymentsPerDay = n })
	}
}

// SetDayLength sets the wall-clock length of a simulated day, as the
// console does; zero is flat out. A negative value is refused. It applies
// to the wait in progress — the run loop's wait is recomputed, so zero
// means the next event comes now — and is recorded with the run so the
// next process resumes at it.
func (s *Simulation) SetDayLength(d time.Duration) {
	if d < 0 {
		return
	}
	s.settings.Update(func(v *Settings) { v.DayLength = d })
	s.mu.Lock()
	s.dayLengthRecorded = true
	s.mu.Unlock()
	select {
	case s.dayLengthChanged <- struct{}{}:
	default: // one change already pending; the loop reads the setting when it wakes
	}
	if s.store != nil {
		s.store.SaveDayLength(d)
	}
}

// DefaultDayLength is the environment's day length (GOBANK_DAY_LENGTH):
// where a run starts until the console sets one. A run resumed with a
// recorded setting keeps it.
func (s *Simulation) DefaultDayLength(d time.Duration) {
	if s.DayLengthRecorded() || d < 0 {
		return
	}
	s.settings.Update(func(v *Settings) { v.DayLength = d })
}

// --- Status ---

// Status is the simulation as the console shows it.
type Status struct {
	Running             bool
	AddingCust          bool
	AddingProgress      int
	AddingTarget        int
	CustomersPerSec     float64       // live rate of the running batch add
	LastCustomersPerSec float64       // rate of the last finished batch add
	DayLength           time.Duration // wall-clock length of a simulated day; zero is flat out
	DayEndsIn           time.Duration // what is left of the day in progress by the wall; zero when flat out or stopped
	Wall                time.Time     // the wall clock now
	Clock               time.Time     // the bank's clock: the instant it stands at
	Warp                float64       // the pace: simulated seconds per wall second over a day; zero when flat out
	DayRefused          string        // why the bank would not follow the clock when last asked (a silent market, ADR-0009); empty once it does
	Owed                core.Owed     // what the bank owed when last asked (ADR-0010); at rest when all zero
	Next                Event         // the event the run loop is waiting for; zero when stopped
}

// Status is a snapshot of the simulation.
func (s *Simulation) Status() Status {
	dayLength := s.settings.Get().DayLength
	warp := 0.0
	if dayLength > 0 {
		warp = float64(24*time.Hour) / float64(dayLength)
	}
	clock := s.clock.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.wall()
	addRate := 0.0
	if s.addingCustRunning {
		addRate = perSecond(s.addingCustProgress, now.Sub(s.addingCustStart))
	}
	var dayEndsIn time.Duration
	var next Event
	if s.running {
		if !s.dayEndsAt.IsZero() {
			dayEndsIn = max(s.dayEndsAt.Sub(now), 0)
		}
		next = s.next
	}
	return Status{
		Running:             s.running,
		AddingCust:          s.addingCustRunning,
		AddingProgress:      s.addingCustProgress,
		AddingTarget:        s.addingCustTarget,
		CustomersPerSec:     addRate,
		LastCustomersPerSec: s.lastAddRate,
		DayLength:           dayLength,
		DayEndsIn:           dayEndsIn,
		Wall:                now,
		Clock:               clock,
		Warp:                warp,
		DayRefused:          s.dayRefused,
		Owed:                s.owed,
		Next:                next,
	}
}

// perSecond is a rate over a span, zero for an empty span.
func perSecond(n int, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(n) / d.Seconds()
}
