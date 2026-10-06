package sim

import (
	"context"
	"log"
	"math/rand"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/core"
)

// The payments generator: customers paying each other.

// SendPayment picks two customers and an amount and asks the bank to move
// it, from the sender's first savings account within its balance. The
// sender may be missing (the bank refused them) or hold no savings
// account, and the recipient likewise; the generator just moves on.
func (s *Simulation) SendPayment() {
	ctx := context.Background()
	pos, err := s.bank.Position(ctx)
	if err != nil || pos.Customers < 2 {
		return
	}
	s.mu.Lock()
	fromID := randomCustomerID(s.rng, pos.Customers)
	toID := randomCustomerID(s.rng, pos.Customers)
	for toID == fromID {
		toID = randomCustomerID(s.rng, pos.Customers)
	}
	amount := luca.Amount(s.rng.Intn(99901) + 100) // 100..100000 pence
	s.mu.Unlock()

	accounts, err := s.bank.Accounts(ctx, fromID)
	if err != nil {
		return
	}
	var savings *core.Account
	for i := range accounts {
		if accounts[i].Family == "Savings" {
			savings = &accounts[i]
			break
		}
	}
	if savings == nil {
		return
	}
	amount = min(amount, savings.Balance)
	if amount < 100 {
		return
	}
	if _, err := s.bank.Transfer(ctx, core.Transfer{From: fromID, To: toID, Amount: amount}); err != nil && err != core.ErrNotFound {
		log.Printf("generator: transfer: %v", err)
	}
}

// randomCustomerID picks one of the n customers on the register uniformly.
func randomCustomerID(rng *rand.Rand, n int) string {
	return core.CustomerID(1 + rng.Intn(n))
}

// StartPayments begins auto-generating payments.
func (s *Simulation) StartPayments() {
	s.mu.Lock()
	if s.payRunning {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.payCancel = cancel
	s.payRunning = true
	s.mu.Unlock()

	go func() {
		// Self-pace (wait after completion) rather than a fixed-rate ticker —
		// see Start(): in WASM a ticker that can't keep up starves the JS
		// event loop and freezes the page.
		s.SendPayment()
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
				s.SendPayment()
			}
		}
	}()
}

// StopPayments halts auto-generation.
func (s *Simulation) StopPayments() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.payRunning {
		return
	}
	s.payRunning = false
	if s.payCancel != nil {
		s.payCancel()
		s.payCancel = nil
	}
}

// IsPaymentsRunning returns whether auto-generation is active.
func (s *Simulation) IsPaymentsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.payRunning
}
