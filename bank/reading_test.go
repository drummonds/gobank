package bank

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A reading is a figure read from the database in the background: pages
// get whatever was read last, straight away, and a read that has gone
// stale is taken again behind them rather than in front of them. Only a
// reading that has never been taken, or one a command has invalidated,
// makes a page wait.

type countedRead struct {
	mu    sync.Mutex
	n     atomic.Int32
	value int
	gate  chan struct{} // when set, reads block until it is closed
	err   error
}

func (c *countedRead) read(context.Context) (int, error) {
	c.n.Add(1)
	c.mu.Lock()
	gate, value, err := c.gate, c.value, c.err
	c.mu.Unlock()
	if gate != nil {
		<-gate
	}
	return value, err
}

func (c *countedRead) set(v int) {
	c.mu.Lock()
	c.value = v
	c.mu.Unlock()
}

func eventually(t *testing.T, want int, get func() int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if get() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("got %d, want %d", get(), want)
}

func TestReadingFirstGetWaitsForTheFirstRead(t *testing.T) {
	src := &countedRead{value: 7}
	r := newReading(src.read, time.Minute)
	if got := r.get(context.Background()); got != 7 {
		t.Fatalf("first get %d, want 7", got)
	}
	if got := r.get(context.Background()); got != 7 || src.n.Load() != 1 {
		t.Fatalf("second get %d after %d reads, want 7 from the one read", got, src.n.Load())
	}
}

func TestReadingExpiredServesTheOldValueAndRefreshesBehind(t *testing.T) {
	src := &countedRead{value: 1}
	r := newReading(src.read, time.Minute)
	r.get(context.Background())
	src.set(2)
	r.expire()
	if got := r.get(context.Background()); got != 1 {
		t.Fatalf("expired get %d, want the old 1 straight away", got)
	}
	eventually(t, 2, func() int { return r.get(context.Background()) })
}

func TestReadingInvalidatedWaitsForTheNewValue(t *testing.T) {
	src := &countedRead{value: 1}
	r := newReading(src.read, time.Minute)
	r.get(context.Background())
	src.set(2)
	r.invalidate()
	if got := r.get(context.Background()); got != 2 {
		t.Fatalf("get after invalidate %d, want 2", got)
	}
}

func TestReadingOneReadInFlightAtATime(t *testing.T) {
	gate := make(chan struct{})
	src := &countedRead{value: 1, gate: gate}
	r := newReading(src.read, time.Minute)
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() { ; r.get(context.Background()) })
	}
	eventually(t, 1, func() int { return int(src.n.Load()) })
	close(gate)
	wg.Wait()
	if n := src.n.Load(); n != 1 {
		t.Fatalf("%d reads for five concurrent first gets, want 1", n)
	}
}

func TestReadingInvalidatedDuringAReadWaitsForOneStartedAfter(t *testing.T) {
	gate := make(chan struct{})
	src := &countedRead{value: 1, gate: gate}
	r := newReading(src.read, time.Minute)
	first := make(chan int)
	go func() { first <- r.get(context.Background()) }()
	eventually(t, 1, func() int { return int(src.n.Load()) })
	// The world moves on while the first read is in flight: what it
	// returns is already out of date.
	gate2 := make(chan struct{})
	src.mu.Lock()
	src.value, src.gate = 2, gate2
	src.mu.Unlock()
	r.invalidate()
	close(gate)
	<-first // the first page gets whichever read has landed by then
	second := make(chan int)
	go func() { second <- r.get(context.Background()) }()
	select {
	case got := <-second:
		t.Fatalf("get after invalidate returned %d before a read started after it landed", got)
	case <-time.After(50 * time.Millisecond):
	}
	close(gate2)
	if got := <-second; got != 2 {
		t.Fatalf("get after invalidate %d, want 2 from a read started after it", got)
	}
	if n := src.n.Load(); n != 2 {
		t.Fatalf("%d reads, want 2", n)
	}
}

func TestReadingFreshReadsNowAndStores(t *testing.T) {
	src := &countedRead{value: 1}
	r := newReading(src.read, time.Minute)
	r.get(context.Background())
	src.set(3)
	if got := r.fresh(context.Background()); got != 3 {
		t.Fatalf("fresh %d, want 3", got)
	}
	if got := r.get(context.Background()); got != 3 || src.n.Load() != 2 {
		t.Fatalf("get after fresh %d after %d reads, want the stored 3", got, src.n.Load())
	}
}

func TestReadingFreshSupersedesAReadInFlight(t *testing.T) {
	gate := make(chan struct{})
	src := &countedRead{value: 1, gate: gate}
	r := newReading(src.read, time.Minute)
	first := make(chan int)
	go func() { first <- r.get(context.Background()) }() // the first read, gated: in flight
	eventually(t, 1, func() int { return int(src.n.Load()) })
	src.mu.Lock()
	src.value, src.gate = 2, nil
	src.mu.Unlock()
	if got := r.fresh(context.Background()); got != 2 {
		t.Fatalf("fresh %d, want 2", got)
	}
	close(gate) // the older read lands after the fresh one and must not replace it
	<-first
	if got := r.get(context.Background()); got != 2 {
		t.Fatalf("get after fresh %d, want the fresh 2 kept over the older read", got)
	}
}

func TestReadingKeepsTheLastValueWhenAReadFails(t *testing.T) {
	src := &countedRead{value: 1}
	r := newReading(src.read, time.Minute)
	r.get(context.Background())
	src.mu.Lock()
	src.err = errors.New("database away")
	src.mu.Unlock()
	r.expire()
	r.get(context.Background())
	eventually(t, 2, func() int { return int(src.n.Load()) })
	if got := r.get(context.Background()); got != 1 {
		t.Fatalf("get after a failed read %d, want the last good 1", got)
	}
}
