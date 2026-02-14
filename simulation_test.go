package gobank

import (
	"testing"
	"time"
)

func TestNewSimulation(t *testing.T) {
	sim, _ := newTestSimulation(t)
	if sim == nil {
		t.Fatal("simulation is nil")
	}
	if sim.Ledger == nil {
		t.Fatal("ledger is nil")
	}
}

func TestOpenAccount(t *testing.T) {
	sim, _ := newTestSimulation(t)
	sim.RegisterAccountBehavior(SavingsAccountBehavior{})
	sim.AddCustomer(&Customer{ID: "C001", Name: "Alice"})

	ma, err := sim.OpenAccount("C001", "savings", "Liability:Savings:0001", "GBP", -2, 0.0365)
	if err != nil {
		t.Fatal(err)
	}
	if ma.Status != StatusActive {
		t.Errorf("expected Active, got %s", ma.Status)
	}
	if ma.Account.ID == 0 {
		t.Error("account ID should be non-zero")
	}
	if ma.CustomerID != "C001" {
		t.Errorf("expected customer C001, got %s", ma.CustomerID)
	}
}

func TestOpenAccountUnknownBehavior(t *testing.T) {
	sim, _ := newTestSimulation(t)
	sim.AddCustomer(&Customer{ID: "C001", Name: "Alice"})

	_, err := sim.OpenAccount("C001", "nonexistent", "Liability:Savings:0001", "GBP", -2, 0)
	if err == nil {
		t.Fatal("expected error for unknown behavior")
	}
}

func TestOpenAccountUnknownCustomer(t *testing.T) {
	sim, _ := newTestSimulation(t)
	sim.RegisterAccountBehavior(SavingsAccountBehavior{})

	_, err := sim.OpenAccount("NOBODY", "savings", "Liability:Savings:0001", "GBP", -2, 0)
	if err == nil {
		t.Fatal("expected error for unknown customer")
	}
}

func TestRecordMovement(t *testing.T) {
	sim, _ := newTestSimulation(t)
	sim.RegisterAccountBehavior(SavingsAccountBehavior{})
	sim.AddCustomer(&Customer{ID: "C001", Name: "Alice"})

	savings, err := sim.OpenAccount("C001", "savings", "Liability:Savings:0001", "GBP", -2, 0.0365)
	if err != nil {
		t.Fatal(err)
	}

	// Create equity account for funding (via ledger directly — not a managed account)
	equity, err := sim.Ledger.CreateAccount("Equity:Capital", "GBP", -2, 0)
	if err != nil {
		t.Fatal(err)
	}

	jan1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	_, err = sim.RecordMovement(equity.ID, savings.Account.ID, 100000, jan1, "Initial deposit")
	if err != nil {
		t.Fatal(err)
	}

	bal, err := sim.Ledger.Balance(savings.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bal != 100000 {
		t.Errorf("expected balance 100000, got %d", bal)
	}
}

func TestInterestAccrual(t *testing.T) {
	sim, _ := newTestSimulation(t)
	sim.RegisterAccountBehavior(SavingsAccountBehavior{})
	sim.AddCustomer(&Customer{ID: "C001", Name: "Alice"})

	savings, err := sim.OpenAccount("C001", "savings", "Liability:Savings:0001", "GBP", -2, 0.0365)
	if err != nil {
		t.Fatal(err)
	}

	equity, err := sim.Ledger.CreateAccount("Equity:Capital", "GBP", -2, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Deposit £1000 on Jan 1
	jan1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	_, err = sim.RecordMovement(equity.ID, savings.Account.ID, 100000, jan1, "Initial deposit")
	if err != nil {
		t.Fatal(err)
	}

	// Process end of day for Jan 1
	updates, err := sim.AdvanceToDate(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	if len(updates) != 1 {
		t.Fatalf("expected 1 daily update, got %d", len(updates))
	}
	if len(updates[0].Accounts) != 1 {
		t.Fatalf("expected 1 account update, got %d", len(updates[0].Accounts))
	}

	// 3.65% / 365 * 100000 pence = 10 pence per day
	au := updates[0].Accounts[0]
	if au.InterestAmount != 10 {
		t.Errorf("expected interest of 10 pence, got %d", au.InterestAmount)
	}

	// Balance should be £1000.10 = 100010
	bal, err := sim.Ledger.BalanceAt(savings.Account.ID, endOfDay(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	if bal != 100010 {
		t.Errorf("expected balance 100010, got %d", bal)
	}
}

func TestMultiDayInterest(t *testing.T) {
	sim, _ := newTestSimulation(t)
	sim.RegisterAccountBehavior(SavingsAccountBehavior{})
	sim.AddCustomer(&Customer{ID: "C001", Name: "Alice"})

	savings, err := sim.OpenAccount("C001", "savings", "Liability:Savings:0001", "GBP", -2, 0.0365)
	if err != nil {
		t.Fatal(err)
	}

	equity, err := sim.Ledger.CreateAccount("Equity:Capital", "GBP", -2, 0)
	if err != nil {
		t.Fatal(err)
	}

	jan1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	_, err = sim.RecordMovement(equity.ID, savings.Account.ID, 100000, jan1, "Initial deposit")
	if err != nil {
		t.Fatal(err)
	}

	// Advance 10 days
	updates, err := sim.AdvanceToDate(time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	if len(updates) != 10 {
		t.Errorf("expected 10 daily updates, got %d", len(updates))
	}

	// After 10 days at 3.65%: ~10 pence/day with slight compounding
	bal, err := sim.Ledger.BalanceAt(savings.Account.ID, endOfDay(time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	// 10 days * 10p/day = 100p, balance should be ~100100 (100000 + 100)
	if bal < 100090 || bal > 100110 {
		t.Errorf("expected balance near 100100, got %d", bal)
	}
}

func TestDailyUpdateHandler(t *testing.T) {
	sim, _ := newTestSimulation(t)
	sim.RegisterAccountBehavior(SavingsAccountBehavior{})
	sim.AddCustomer(&Customer{ID: "C001", Name: "Alice"})

	var received []DailyUpdate
	sim.OnDailyUpdate(func(u DailyUpdate) {
		received = append(received, u)
	})

	savings, err := sim.OpenAccount("C001", "savings", "Liability:Savings:0001", "GBP", -2, 0.0365)
	if err != nil {
		t.Fatal(err)
	}

	equity, err := sim.Ledger.CreateAccount("Equity:Capital", "GBP", -2, 0)
	if err != nil {
		t.Fatal(err)
	}

	jan1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	_, err = sim.RecordMovement(equity.ID, savings.Account.ID, 100000, jan1, "Deposit")
	if err != nil {
		t.Fatal(err)
	}

	_, err = sim.AdvanceToDate(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	if len(received) != 3 {
		t.Errorf("expected 3 daily updates delivered via handler, got %d", len(received))
	}
	for _, u := range received {
		if len(u.Accounts) != 1 {
			t.Errorf("expected 1 account update per day, got %d", len(u.Accounts))
		}
	}
}

func TestFutureDatedPosting(t *testing.T) {
	sim, _ := newTestSimulation(t)
	sim.RegisterAccountBehavior(SavingsAccountBehavior{})
	sim.AddCustomer(&Customer{ID: "C001", Name: "Alice"})

	savings, err := sim.OpenAccount("C001", "savings", "Liability:Savings:0001", "GBP", -2, 0.0365)
	if err != nil {
		t.Fatal(err)
	}

	equity, err := sim.Ledger.CreateAccount("Equity:Capital", "GBP", -2, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Post a movement dated Jan 5 (future from Jan 1)
	jan5 := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
	_, err = sim.RecordMovement(equity.ID, savings.Account.ID, 50000, jan5, "Future deposit")
	if err != nil {
		t.Fatal(err)
	}

	// On Jan 3, the balance should be 0 (movement is in the future)
	jan3eod := endOfDay(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	bal, err := sim.Ledger.BalanceAt(savings.Account.ID, jan3eod)
	if err != nil {
		t.Fatal(err)
	}
	if bal != 0 {
		t.Errorf("expected 0 balance on Jan 3 (future posting), got %d", bal)
	}

	// On Jan 5, the balance should reflect the deposit
	jan5eod := endOfDay(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC))
	bal, err = sim.Ledger.BalanceAt(savings.Account.ID, jan5eod)
	if err != nil {
		t.Fatal(err)
	}
	if bal != 50000 {
		t.Errorf("expected 50000 balance on Jan 5, got %d", bal)
	}
}

func TestCloseAccount(t *testing.T) {
	sim, _ := newTestSimulation(t)
	sim.RegisterAccountBehavior(SavingsAccountBehavior{})
	sim.AddCustomer(&Customer{ID: "C001", Name: "Alice"})

	ma, err := sim.OpenAccount("C001", "savings", "Liability:Savings:0001", "GBP", -2, 0)
	if err != nil {
		t.Fatal(err)
	}

	if err := sim.CloseAccount(ma.Account.ID); err != nil {
		t.Fatal(err)
	}
	if ma.Status != StatusClosed {
		t.Errorf("expected Closed, got %s", ma.Status)
	}
}
