package main

import (
	"context"
	"log"
	"math/rand"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/core"
)

// The payments generator (ADR-0002 stage 4): customers paying each other.
// Simulation, not bank: it raises Transfer on the bank and reads the
// bank through its staff queries (TestGeneratorsReachTheBankOnlyThroughCommands).

// SendPayment picks two customers and an amount and asks the bank to move
// it, from the sender's first savings account within its balance. The
// sender may be missing (the bank refused them) or hold no savings
// account, and the recipient likewise; the generator just moves on.
func (ds *DemoState) SendPayment() {
	ctx := context.Background()
	pos, err := ds.bank.Position(ctx)
	if err != nil || pos.Customers < 2 {
		return
	}
	ds.mu.Lock()
	fromID := randomCustomerID(ds.rng, pos.Customers)
	toID := randomCustomerID(ds.rng, pos.Customers)
	for toID == fromID {
		toID = randomCustomerID(ds.rng, pos.Customers)
	}
	amount := luca.Amount(ds.rng.Intn(99901) + 100) // 100..100000 pence
	ds.mu.Unlock()

	accounts, err := ds.bank.Accounts(ctx, fromID)
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
	if _, err := ds.bank.Transfer(ctx, core.Transfer{From: fromID, To: toID, Amount: amount}); err != nil && err != core.ErrNotFound {
		log.Printf("generator: transfer: %v", err)
	}
}

// randomCustomerID picks one of the n customers on the register uniformly.
func randomCustomerID(rng *rand.Rand, n int) string {
	return core.CustomerID(1 + rng.Intn(n))
}

// StartPayments begins auto-generating payments.
func (ds *DemoState) StartPayments() {
	ds.mu.Lock()
	if ds.payRunning {
		ds.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ds.payCancel = cancel
	ds.payRunning = true
	ds.mu.Unlock()

	go func() {
		// Self-pace (wait after completion) rather than a fixed-rate ticker —
		// see Start(): in WASM a ticker that can't keep up starves the JS
		// event loop and freezes the page.
		ds.SendPayment()
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
				ds.SendPayment()
			}
		}
	}()
}

// StopPayments halts auto-generation.
func (ds *DemoState) StopPayments() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if !ds.payRunning {
		return
	}
	ds.payRunning = false
	if ds.payCancel != nil {
		ds.payCancel()
		ds.payCancel = nil
	}
}

// IsPaymentsRunning returns whether auto-generation is active.
func (ds *DemoState) IsPaymentsRunning() bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.payRunning
}
