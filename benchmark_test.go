package gobank

import (
	"fmt"
	"testing"
	"time"

	luca "github.com/drummonds/go-luca"
	_ "github.com/drummonds/go-postgres"
)

func BenchmarkSimulation1000Accounts3Days(b *testing.B) {
	for b.Loop() {
		benchmarkAccounts(b, 1000, 3)
	}
}

func BenchmarkSimulation1000Accounts30Days(b *testing.B) {
	for b.Loop() {
		benchmarkAccounts(b, 1000, 30)
	}
}

func BenchmarkSimulation10000Accounts3Days(b *testing.B) {
	for b.Loop() {
		benchmarkAccounts(b, 10000, 3)
	}
}

func benchmarkAccounts(b *testing.B, numAccounts, numDays int) {
	b.Helper()

	ledger, err := luca.NewLedger(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer ledger.Close()

	clock := NewSimClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sim, err := NewSimulation(ledger, clock)
	if err != nil {
		b.Fatal(err)
	}
	sim.RegisterAccountBehavior(SavingsAccountBehavior{})

	// Create equity account for funding
	equity, err := sim.Ledger.CreateAccount("Equity:Capital", "GBP", -2, 0)
	if err != nil {
		b.Fatal(err)
	}

	// Create customers and accounts
	for i := range numAccounts {
		custID := fmt.Sprintf("C%06d", i)
		sim.AddCustomer(&Customer{ID: custID, Name: fmt.Sprintf("Customer %d", i)})

		path := fmt.Sprintf("Liability:Savings:%06d", i)
		ma, err := sim.OpenAccount(custID, "savings", path, "GBP", -2, 0.0365)
		if err != nil {
			b.Fatal(err)
		}

		// Deposit £1000 into each account
		jan1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		_, err = sim.RecordMovement(equity.ID, ma.Account.ID, 100000, jan1, "Initial deposit")
		if err != nil {
			b.Fatal(err)
		}
	}

	// Advance through days
	target := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, numDays-1)
	_, err = sim.AdvanceToDate(target)
	if err != nil {
		b.Fatal(err)
	}
}
