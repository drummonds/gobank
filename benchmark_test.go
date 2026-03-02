package gobank

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	luca "github.com/drummonds/go-luca"
	_ "github.com/drummonds/go-postgres"
)

func BenchmarkSimulation(b *testing.B) {
	dsn := os.Getenv("GOBANK_DSN")
	if dsn == "" {
		dsn = ":memory:"
	}

	cases := []struct {
		accounts int
		days     int
	}{
		{1_000, 3},
		{10_000, 3},
		{100_000, 3},
		{1_000, 30},
		{10_000, 30},
	}

	for _, tc := range cases {
		name := fmt.Sprintf("%dk_accts_%d_days", tc.accounts/1000, tc.days)
		b.Run(name, func(b *testing.B) {
			benchmarkAccounts(b, tc.accounts, tc.days, dsn)
		})
	}
}

func BenchmarkSimulationFile(b *testing.B) {
	cases := []struct {
		accounts int
		days     int
	}{
		{1_000, 3},
		{10_000, 3},
		{1_000, 30},
	}

	for _, tc := range cases {
		name := fmt.Sprintf("%dk_accts_%d_days", tc.accounts/1000, tc.days)
		b.Run(name, func(b *testing.B) {
			dir := b.TempDir()
			dsn := filepath.Join(dir, "bench.db")
			benchmarkAccounts(b, tc.accounts, tc.days, dsn)
		})
	}
}

func benchmarkAccounts(b *testing.B, numAccounts, numDays int, dsn string) {
	b.Helper()

	for b.Loop() {
		actualDSN := dsn
		// For file-backed DSNs, use a fresh file per iteration to avoid stale state
		if dsn != ":memory:" {
			dir := b.TempDir()
			actualDSN = filepath.Join(dir, "bench.db")
		}

		ledger, err := luca.NewLedger(actualDSN)
		if err != nil {
			b.Fatal(err)
		}

		clock := NewSimClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		sim, err := NewSimulation(ledger, clock)
		if err != nil {
			ledger.Close()
			b.Fatal(err)
		}
		sim.RegisterAccountBehavior(SavingsAccountBehavior{})

		equity, err := sim.Ledger.CreateAccount("Equity:Capital", "GBP", -2, 0)
		if err != nil {
			ledger.Close()
			b.Fatal(err)
		}

		for i := range numAccounts {
			custID := fmt.Sprintf("C%06d", i)
			sim.AddCustomer(&Customer{ID: custID, Name: fmt.Sprintf("Customer %d", i)})

			path := fmt.Sprintf("Liability:Savings:%06d", i)
			ma, err := sim.OpenAccount(custID, "savings", path, "GBP", -2, 0.0365)
			if err != nil {
				ledger.Close()
				b.Fatal(err)
			}

			jan1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			_, err = sim.RecordMovement(equity.ID, ma.Account.ID, 100000, jan1, "Initial deposit")
			if err != nil {
				ledger.Close()
				b.Fatal(err)
			}
		}

		target := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, numDays-1)
		_, err = sim.AdvanceToDate(target)
		ledger.Close()
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportMetric(float64(numAccounts), "accounts/op")
	b.ReportMetric(float64(numDays), "days_processed/op")
	elapsed := b.Elapsed()
	if elapsed > 0 && b.N > 0 {
		acctDaysPerSec := float64(numAccounts) * float64(numDays) * float64(b.N) / elapsed.Seconds()
		b.ReportMetric(acctDaysPerSec, "acct_days/sec")
	}
}
