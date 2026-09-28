package main

import (
	"fmt"
	"math"
	"strconv"
	"sync"
	"time"
)

// dayProgress tracks the simulated day being processed, so a day that takes
// minutes of ledger writes is visibly working rather than looking stopped.
// It has its own lock so the streaming write path never touches ds.mu, and
// its own clock so reading it doesn't perturb ds.now in tests.
type dayProgress struct {
	mu  sync.Mutex
	now func() time.Time // nil means time.Now

	active     bool
	day        time.Time
	started    time.Time
	phaseName  string
	phaseStart time.Time
	done       int // movements written in the current phase
	total      int // movements expected in the current phase; 0 if unknown
	dayDone    int // movements written across the whole day

	lastDay       time.Time
	lastDuration  time.Duration
	lastMovements int
}

// DayProgress is a snapshot of dayProgress for rendering.
type DayProgress struct {
	Active      bool
	Day         time.Time
	Phase       string
	Done, Total int
	Elapsed     time.Duration // since the day began
	Rate        float64       // movements/s within the current phase

	LastDay       time.Time
	LastDuration  time.Duration
	LastMovements int
}

func (p *dayProgress) clock() time.Time {
	if p.now == nil {
		return time.Now()
	}
	return p.now()
}

func (p *dayProgress) begin(day time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.clock()
	p.active, p.day, p.started, p.dayDone = true, day, now, 0
	p.phaseName, p.phaseStart, p.done, p.total = "", now, 0, 0
}

func (p *dayProgress) phase(name string, total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.phaseName, p.phaseStart, p.done, p.total = name, p.clock(), 0, total
}

func (p *dayProgress) add(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done += n
	p.dayDone += n
}

func (p *dayProgress) finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.active {
		return
	}
	p.active = false
	p.lastDay, p.lastDuration, p.lastMovements = p.day, p.clock().Sub(p.started), p.dayDone
}

func (p *dayProgress) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active, p.day, p.phaseName, p.done, p.total, p.dayDone = false, time.Time{}, "", 0, 0, 0
	p.lastDay, p.lastDuration, p.lastMovements = time.Time{}, 0, 0
}

func (p *dayProgress) snapshot() DayProgress {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := DayProgress{
		Active: p.active, Day: p.day, Phase: p.phaseName, Done: p.done, Total: p.total,
		LastDay: p.lastDay, LastDuration: p.lastDuration, LastMovements: p.lastMovements,
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

// runtimeRow renders the progress as a row of the runtime page's
// Simulation table, or "" before the first day has run.
func (s DayProgress) runtimeRow() string {
	switch {
	case s.Active:
		work := s.Phase
		if s.Total > 0 {
			work += fmt.Sprintf(": %s / %s (%d%%) at %s/s", groupInt(s.Done), groupInt(s.Total), s.Done*100/s.Total, groupInt(int(math.Round(s.Rate))))
		}
		return fmt.Sprintf(`<tr><th>Day in progress</th><td>%s — %s</td><td class="has-text-grey">%s elapsed</td></tr>`,
			s.Day.Format("2 Jan 2006"), work, s.Elapsed.Round(time.Second))
	case !s.LastDay.IsZero():
		return fmt.Sprintf(`<tr><th>Last day</th><td>%s: %s movements in %s</td></tr>`,
			s.LastDay.Format("2 Jan 2006"), groupInt(s.LastMovements), s.LastDuration.Round(time.Second))
	}
	return ""
}

func groupInt(n int) string { return groupThousands(strconv.Itoa(n)) }
