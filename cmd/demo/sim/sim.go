// Package sim is the simulation (ADR-0002 stage 4): the generators that
// make a population and its payments, the warped clock the bank reads,
// the replayed base rate, and the run loop and console knobs that drive
// them. It is built on the core alone: it raises events on the bank
// through core.Commands and decides them from core.StaffQueries, and
// knows nothing else of the bank (cmd/demo's TestSimulationImportsOnlyTheCore).
// A real bank has none of this; the console that drives it is part of the
// staff web app.
package sim

import (
	"context"
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
// the settings page or in the environment. The bank's own parameters (BoE
// rate, reserve ratio) are the bank's, not settings.
type Settings struct {
	MaxCustomers int
	DayLength    time.Duration // wall-clock length of a simulated day; zero is flat out
}

// DefaultSettings is a population cap and flat-out days.
func DefaultSettings() Settings {
	return Settings{MaxCustomers: 1_000_000}
}

// Run is the run's place in time as the store keeps it: the simulated
// day the clock is on and when its slot began, how many days the bank has
// run, and whether the run loop was going, so a restart picks the run up
// where it was.
type Run struct {
	Day       time.Time
	DayCount  int
	Running   bool
	SlotStart time.Time
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
	Pause    func() bool      // asked before each day; true stops the run (a memory limit, say)
	Settings Settings         // the console's starting knobs; zero means DefaultSettings
	Seed     int64            // the generators' randomness
}

// Simulation drives a Bank: the clock, the generators and the run loop.
type Simulation struct {
	mu       sync.Mutex
	bank     Bank
	clock    *Clock
	store    Store
	wall     func() time.Time
	workers  int
	pause    func() bool
	rng      *rand.Rand // the generators' randomness (mu)
	settings settings

	running           bool
	cancel            context.CancelFunc
	loopDone          chan struct{} // closed when the run loop's goroutine has exited (mu)
	shuttingDown      bool          // the loop was stopped by Shutdown, not the operator: the run is still on (mu)
	resumedRunning    bool          // the run was going when the previous process stopped
	dayLengthRecorded bool          // the console set the day length; it is on the run and beats the environment's (mu)
	dayEndsAt         time.Time     // when the day in progress ends, zero when flat out or stopped (mu)
	dayLengthChanged  chan struct{} // a console change of the day length, for the run loop's idle wait (one pending at most)

	payRunning bool
	payCancel  context.CancelFunc

	addingCustRunning  bool
	addingCustCancel   context.CancelFunc
	addingCustProgress int
	addingCustTarget   int
	addingCustStart    time.Time
	lastAddRate        float64 // customers/s of the last finished batch

	catalogue []core.Product // the generators' copy of the product catalogue, read once (mu)
}

// New is a simulation over bank, its clock on OpeningDay.
func New(bank Bank, store Store, opts Options) *Simulation {
	s := &Simulation{
		bank:             bank,
		store:            store,
		wall:             opts.Wall,
		workers:          max(opts.Workers, 1),
		pause:            opts.Pause,
		rng:              rand.New(rand.NewSource(opts.Seed)),
		settings:         settings{v: opts.Settings},
		dayLengthChanged: make(chan struct{}, 1),
	}
	if s.wall == nil {
		s.wall = time.Now
	}
	if s.settings.v == (Settings{}) {
		s.settings.v = DefaultSettings()
	}
	s.clock = NewClock(func() time.Time { return s.wall() })
	s.clock.dayLength = func() time.Duration { return s.settings.Get().DayLength }
	s.clock.BeginDay(OpeningDay)
	return s
}

// Clock is the clock the bank reads: the simulation's, warped.
func (s *Simulation) Clock() *Clock { return s.clock }

// SetWall replaces the wall clock, for tests.
func (s *Simulation) SetWall(wall func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wall = wall
}

// Resume puts the simulation back where a previous process left it: the
// clock on its day, the run loop to be started again if it was going
// (ResumedRunning), and the day length the console set, if any.
func (s *Simulation) Resume(run Run, dayLength time.Duration, dayLengthSet bool) {
	s.clock.Resume(run.Day, run.SlotStart)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resumedRunning = run.Running
	s.rng = rand.New(rand.NewSource(42 + int64(run.DayCount)))
	if dayLengthSet {
		s.settings.Update(func(v *Settings) { v.DayLength = dayLength })
		s.dayLengthRecorded = true
	}
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

// Reset stops everything and puts the clock back on day for a fresh run;
// the caller resets the bank and records the run.
func (s *Simulation) Reset(day time.Time) {
	s.mu.Lock()
	if s.running {
		s.running = false
		if s.cancel != nil {
			s.cancel()
			s.cancel = nil
		}
	}
	if s.payRunning {
		s.payRunning = false
		if s.payCancel != nil {
			s.payCancel()
			s.payCancel = nil
		}
	}
	if s.addingCustRunning {
		if s.addingCustCancel != nil {
			s.addingCustCancel()
		}
		s.finishAddingLocked()
	}
	s.lastAddRate = 0
	s.rng = rand.New(rand.NewSource(42))
	s.dayEndsAt = time.Time{}
	s.shuttingDown = false
	s.resumedRunning = false
	s.mu.Unlock()
	s.clock.BeginDay(day)
}

// --- The run loop ---

// Start begins the run: a slot per simulated day. The wait after each
// day's work is what remains of the day length (the work is done at the
// start at the system's capacity and the rest of the day is idle), or
// minDayGap flat out. A day length set on the console applies to the day
// in progress: its end moves, and so does the wait; to zero means now.
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
	s.mu.Unlock()
	s.saveRun()

	go func() {
		defer close(done)
		var start time.Time // when the day in progress began; zero before the first
		wait := minDayGap
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.dayLengthChanged:
				if start.IsZero() {
					continue // no day in progress yet: the first day reads the setting when it begins
				}
				dayLength := s.settings.Get().DayLength
				s.setDayEnd(start, dayLength)
				wait = nextDayDelay(dayLength, s.wall().Sub(start))
			case <-time.After(wait):
				if s.pause != nil && s.pause() {
					s.Stop()
					return
				}
				start = s.wall()
				dayLength := s.settings.Get().DayLength
				s.setDayEnd(start, dayLength)
				s.Step(ctx)
				if ctx.Err() == nil {
					s.rollNewCustomer(ctx)
				}
				wait = nextDayDelay(dayLength, s.wall().Sub(start))
			}
		}
	}()
}

// setDayEnd records when the day starting now ends: a day length on, or
// never when flat out.
func (s *Simulation) setDayEnd(start time.Time, dayLength time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dayEndsAt = time.Time{}
	if dayLength > 0 {
		s.dayEndsAt = start.Add(dayLength)
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
// loop is told to stop and the day in progress is waited for (bounded by
// ctx), so the database is consistent for the next process, which finds
// the run still recorded as running and carries it on.
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

// Step is one slot of the simulation, unless Pause says not now. The
// clock moves on a day when the bank is level with it and has finished
// its day; a bank ahead of the clock (a stale record) pulls the clock up
// to its day; otherwise the slot begins again on the day in progress (a
// resumed pass). Then the bank follows the clock, and the run is recorded.
// Stops early when ctx ends, leaving the rest of the day for the next
// slot.
func (s *Simulation) Step(ctx context.Context) {
	if s.pause != nil && s.pause() {
		return
	}
	pos, err := s.bank.Position(ctx)
	if err != nil {
		return
	}
	clockDay := s.clock.Day()
	switch {
	case pos.Day.After(clockDay):
		s.clock.BeginDay(pos.Day)
	case pos.DayComplete && pos.Day.Equal(clockDay):
		s.clock.BeginDay(pos.Day.AddDate(0, 0, 1))
	default:
		s.clock.RestartSlot()
	}
	s.bank.StartDay(ctx) //nolint:errcheck // a cut-short pass is resumed next slot
	// Recorded once the bank has begun the day, so the record never makes
	// the day wait on the database; a restart before this point starts the
	// slot again (Resume).
	s.saveRun()
}

// AdvanceDay is the console's step: one slot, then the generators' roll
// for a new customer.
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
	s.store.SaveRun(Run{Day: s.clock.Day(), DayCount: pos.DayCount, Running: running, SlotStart: s.clock.SlotStart()})
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

// SetDayLength sets the wall-clock length of a simulated day, as the
// console does; zero is flat out. A negative value is refused. It applies
// to the day in progress — the run loop's idle wait is recomputed, so zero
// means the next day begins now — and is recorded with the run so the
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
	DayEndsIn           time.Duration // what is left of the day in progress; zero when flat out or stopped
	Wall                time.Time     // the wall clock now
	Clock               time.Time     // the bank's clock now: simulated time
	Warp                float64       // simulated seconds per wall second; zero when flat out
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
	if s.running && !s.dayEndsAt.IsZero() {
		dayEndsIn = max(s.dayEndsAt.Sub(now), 0)
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
	}
}

// perSecond is a rate over a span, zero for an empty span.
func perSecond(n int, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(n) / d.Seconds()
}
