package sim

import (
	"context"
	"log"
	"math/rand"
	"sort"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/core"
)

// The payments generator: customers paying each other. Running, it
// schedules PaymentsPerDay payments at instants inside each simulated
// day, drawn from the seed and the day so a restart finds the same ones,
// and the run loop raises each as an event when the clock reaches it
// (ADR-0010). A payment sent from the console is an event at the
// current instant, run at once.

// SendPayment picks two customers and an amount and asks the bank to move
// it, from the sender's first savings account within its balance. A
// customer may hold no savings account (a borrower), so the draw is
// repeated a few times for a sender with a balance and a recipient with
// an account; failing that, no payment is made and the generator moves
// on. A customer the bank refused is missing from the register and is
// left as not found.
func (s *Simulation) SendPayment() {
	ctx := context.Background()
	pos, err := s.bank.Position(ctx)
	if err != nil || pos.Customers < 2 {
		return
	}
	s.mu.Lock()
	amount := luca.Amount(s.rng.Intn(99901) + 100) // 100..100000 pence
	s.mu.Unlock()
	fromID, balance, ok := s.drawSaver(ctx, pos.Customers, "", 100)
	if !ok {
		return
	}
	toID, _, ok := s.drawSaver(ctx, pos.Customers, fromID, 0)
	if !ok {
		return
	}
	amount = min(amount, balance)
	if _, err := s.bank.Transfer(ctx, core.Transfer{From: fromID, To: toID, Amount: amount}); err != nil && err != core.ErrNotFound {
		log.Printf("generator: transfer: %v", err)
	}
}

// drawAttempts is how many customers the generator looks at for one
// side of a payment before giving up.
const drawAttempts = 8

// drawSaver draws a customer other than not, holding a savings account
// with at least atLeast on it, and returns them with the balance.
func (s *Simulation) drawSaver(ctx context.Context, customers int, not string, atLeast luca.Amount) (string, luca.Amount, bool) {
	for range drawAttempts {
		s.mu.Lock()
		id := randomCustomerID(s.rng, customers)
		s.mu.Unlock()
		if id == not {
			continue
		}
		accounts, err := s.bank.Accounts(ctx, id)
		if err != nil {
			continue
		}
		for _, a := range accounts {
			if a.Family == "Savings" && -a.Balance >= atLeast { // a savings balance is a liability, negative (ADR-0008)
				return id, -a.Balance, true
			}
		}
	}
	return "", 0, false
}

// randomCustomerID picks one of the n customers on the register uniformly.
func randomCustomerID(rng *rand.Rand, n int) string {
	return core.CustomerID(1 + rng.Intn(n))
}

// StartPayments puts the generator's payments on the timeline: from now
// the run loop raises PaymentsPerDay a day. The run must be going for
// the clock to reach them.
func (s *Simulation) StartPayments() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payRunning = true
}

// StopPayments takes the generator's payments off the timeline.
func (s *Simulation) StopPayments() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payRunning = false
}

// IsPaymentsRunning returns whether the generator's payments are on the
// timeline.
func (s *Simulation) IsPaymentsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.payRunning
}

// paymentInstants is the day's payments: n instants inside day, in
// order, the same for the same seed and day.
func paymentInstants(seed int64, day time.Time, n int) []time.Time {
	rng := rand.New(rand.NewSource(seed ^ day.Unix()))
	out := make([]time.Time, n)
	for i := range out {
		out[i] = day.Add(time.Duration(1 + rng.Int63n(int64(24*time.Hour)-1)))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// nextPayment is the generator's next payment after now, within now's
// day; false when the generator is stopped or the day's are done.
func (s *Simulation) nextPayment(now time.Time) (time.Time, bool) {
	s.mu.Lock()
	running, seed := s.payRunning, s.seed
	s.mu.Unlock()
	if !running {
		return time.Time{}, false
	}
	for _, at := range paymentInstants(seed, core.BusinessDay(now), s.settings.Get().PaymentsPerDay) {
		if at.After(now) {
			return at, true
		}
	}
	return time.Time{}, false
}

// sendDuePayments raises the generator's payments due at the instant
// the clock has just reached: those of the day at or before now that are
// after the last tick. Each is one Transfer, settled when it returns.
func (s *Simulation) sendDuePayments(ctx context.Context, now time.Time) {
	s.mu.Lock()
	running, seed, since := s.payRunning, s.seed, s.lastInstant
	s.lastInstant = now
	s.mu.Unlock()
	if !running {
		return
	}
	for _, at := range paymentInstants(seed, core.BusinessDay(now), s.settings.Get().PaymentsPerDay) {
		if ctx.Err() != nil {
			return
		}
		if at.After(since) && !at.After(now) {
			s.SendPayment()
		}
	}
}
