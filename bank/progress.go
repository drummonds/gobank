package bank

import (
	"sync"
	"time"
)

// progress tracks the simulated day being processed, so a day whose pass
// takes minutes at scale is visibly working rather than looking stopped.
// It has its own lock so the pass never touches mu to report, and its
// own clock so reading it does not perturb the bank's in tests.
type progress struct {
	mu  sync.Mutex
	now func() time.Time // nil means time.Now

	active     bool
	day        time.Time
	started    time.Time
	phaseName  string
	phaseStart time.Time
	done       int // accounts done in the current phase
	total      int // accounts expected in the current phase; 0 if unknown
	dayDone    int // accounts done across the whole day

	lastDay      time.Time
	lastDuration time.Duration
	lastAccounts int
}

// DayProgress is the day in progress as the runtime page shows it.
type DayProgress struct {
	Active      bool
	Day         time.Time
	Phase       string
	Done, Total int
	Elapsed     time.Duration // since the day began
	Rate        float64       // accounts/s within the current phase

	LastDay      time.Time
	LastDuration time.Duration
	LastAccounts int
}

func (p *progress) clock() time.Time {
	if p.now == nil {
		return time.Now()
	}
	return p.now()
}

func (p *progress) begin(day time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.clock()
	p.active, p.day, p.started, p.dayDone = true, day, now, 0
	p.phaseName, p.phaseStart, p.done, p.total = "", now, 0, 0
}

func (p *progress) phase(name string, total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.phaseName, p.phaseStart, p.done, p.total = name, p.clock(), 0, total
}

// Phase implements products.Progress.
func (p *progress) Phase(name string, total int) { p.phase(name, total) }

// Add implements products.Progress.
func (p *progress) Add(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done += n
	p.dayDone += n
}

// finish closes the day and returns how long it took, begin to finish; 0
// when no day is in progress.
func (p *progress) finish() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.active {
		return 0
	}
	p.active = false
	p.lastDay, p.lastDuration, p.lastAccounts = p.day, p.clock().Sub(p.started), p.dayDone
	return p.lastDuration
}

func (p *progress) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active, p.day, p.phaseName, p.done, p.total, p.dayDone = false, time.Time{}, "", 0, 0, 0
	p.lastDay, p.lastDuration, p.lastAccounts = time.Time{}, 0, 0
}

func (p *progress) snapshot() DayProgress {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := DayProgress{
		Active: p.active, Day: p.day, Phase: p.phaseName, Done: p.done, Total: p.total,
		LastDay: p.lastDay, LastDuration: p.lastDuration, LastAccounts: p.lastAccounts,
	}
	if p.active {
		now := p.clock()
		s.Elapsed = now.Sub(p.started)
		if d := now.Sub(p.phaseStart); d > 0 {
			s.Rate = float64(p.done) / d.Seconds()
		}
	}
	return s
}

// Progress is the day in progress, or the last day done.
func (b *Bank) Progress() DayProgress { return b.progress.snapshot() }

// SetProgressClock sets the clock the day's progress is timed by; tests
// freeze it.
func (b *Bank) SetProgressClock(now func() time.Time) {
	b.progress.mu.Lock()
	defer b.progress.mu.Unlock()
	b.progress.now = now
}

// --- Throughput ---

// throughputDays is how many recent simulated days the pass's rate is
// averaged over.
const throughputDays = 10

type daySample struct {
	accounts int
	elapsed  time.Duration
}

// throughput is a rolling record of how many accounts each simulated
// day's pass visited and how long the whole day took.
type throughput struct {
	samples []daySample
}

func (t *throughput) record(accounts int, elapsed time.Duration) {
	t.samples = append(t.samples, daySample{accounts, elapsed})
	if len(t.samples) > throughputDays {
		t.samples = t.samples[len(t.samples)-throughputDays:]
	}
}

// per returns the accounts the pass would visit in window at the rate
// measured over the recent days, or 0 with nothing measured.
func (t *throughput) per(window time.Duration) int64 {
	var accounts int
	var elapsed time.Duration
	for _, s := range t.samples {
		accounts += s.accounts
		elapsed += s.elapsed
	}
	if elapsed <= 0 {
		return 0
	}
	return int64(float64(accounts) * float64(window) / float64(elapsed))
}

// AccountDaysPer is the accounts the pass would project in window at the
// rate measured over the recent whole days — "account days per 12h" says
// whether the pass could project every account's day overnight — or 0
// before the first day has run.
func (b *Bank) AccountDaysPer(window time.Duration) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.throughput.per(window)
}
