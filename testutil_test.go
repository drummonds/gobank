package gobank

import (
	"testing"
	"time"

	luca "github.com/drummonds/go-luca"
	_ "github.com/drummonds/go-postgres"
)

func newTestSimulation(t *testing.T) (*Simulation, *SimClock) {
	t.Helper()
	ledger, err := luca.NewLedger(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ledger.Close() })

	clock := NewSimClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	sim, err := NewSimulation(ledger, clock)
	if err != nil {
		t.Fatal(err)
	}
	return sim, clock
}
