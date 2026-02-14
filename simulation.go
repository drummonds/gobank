package gobank

import (
	"fmt"
	"time"

	luca "github.com/drummonds/go-luca"
)

// AccountUpdate captures the state change for one account on one day.
type AccountUpdate struct {
	Account        *ManagedAccount
	Date           time.Time
	OpeningBalance int64
	ClosingBalance int64
	InterestAmount int64
	Exponent       int
}

// DailyUpdate collects all account updates for a single processing day.
type DailyUpdate struct {
	Date     time.Time
	Accounts []AccountUpdate
}

// DailyUpdateHandler is called after each day's processing with the day's updates.
type DailyUpdateHandler func(update DailyUpdate)

// Simulation is the core engine that advances time and processes account behaviors.
type Simulation struct {
	Ledger             *luca.Ledger
	Clock              Clock
	Params             *ParameterStore
	behaviors          map[string]AccountBehavior
	accounts           map[int64]*ManagedAccount
	customers          map[string]*Customer
	startDate          time.Time
	lastProcessedDate  time.Time
	dailyUpdateHandler DailyUpdateHandler
}

// NewSimulation creates a new simulation engine.
func NewSimulation(ledger *luca.Ledger, clock Clock) (*Simulation, error) {
	if err := ledger.EnsureInterestAccounts(); err != nil {
		return nil, fmt.Errorf("ensure interest accounts: %w", err)
	}
	return &Simulation{
		Ledger:    ledger,
		Clock:     clock,
		Params:    NewParameterStore(),
		behaviors: make(map[string]AccountBehavior),
		accounts:  make(map[int64]*ManagedAccount),
		customers: make(map[string]*Customer),
		startDate: startOfDay(clock.Now()),
	}, nil
}

// OnDailyUpdate registers a handler that receives daily account updates
// after each day is processed. This is the mechanism for delivering all
// account state changes so the next day's calculations can build on them.
func (s *Simulation) OnDailyUpdate(handler DailyUpdateHandler) {
	s.dailyUpdateHandler = handler
}

// RegisterAccountBehavior registers an account behavior by its name.
func (s *Simulation) RegisterAccountBehavior(ab AccountBehavior) {
	s.behaviors[ab.Name()] = ab
}

// AddCustomer registers a customer with the simulation.
func (s *Simulation) AddCustomer(c *Customer) {
	s.customers[c.ID] = c
}

// OpenAccount creates a new managed account and calls its OnOpen hook.
func (s *Simulation) OpenAccount(customerID, behaviorName, fullPath, currency string, exponent int, annualInterestRate float64) (*ManagedAccount, error) {
	ab, ok := s.behaviors[behaviorName]
	if !ok {
		return nil, fmt.Errorf("unknown account behavior: %s", behaviorName)
	}
	if _, ok := s.customers[customerID]; !ok {
		return nil, fmt.Errorf("unknown customer: %s", customerID)
	}

	acct, err := s.Ledger.CreateAccount(fullPath, currency, exponent, annualInterestRate)
	if err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	ma := &ManagedAccount{
		Account:      acct,
		BehaviorName: behaviorName,
		CustomerID:   customerID,
		Status:       StatusPending,
		OpenedAt:     s.Clock.Now(),
	}
	s.accounts[acct.ID] = ma

	ctx := EventContext{Sim: s, Account: ma, AsOfDate: s.Clock.Now()}
	if err := ab.OnOpen(ctx); err != nil {
		return nil, fmt.Errorf("on open: %w", err)
	}

	return ma, nil
}

// GetManagedAccount returns a managed account by its ledger account ID.
func (s *Simulation) GetManagedAccount(accountID int64) (*ManagedAccount, bool) {
	ma, ok := s.accounts[accountID]
	return ma, ok
}

// RecordMovement records a movement with optional pre/post hooks on involved accounts.
func (s *Simulation) RecordMovement(fromID, toID, amount int64, valueTime time.Time, description string) (*luca.Movement, error) {
	// Fire pre-movement hooks
	for _, id := range []int64{fromID, toID} {
		if ma, ok := s.accounts[id]; ok {
			if ab, ok := s.behaviors[ma.BehaviorName]; ok {
				if hook, ok := ab.(MovementHook); ok {
					ctx := EventContext{Sim: s, Account: ma, AsOfDate: valueTime}
					if err := hook.PreMovement(ctx, fromID, toID, amount); err != nil {
						return nil, fmt.Errorf("pre-movement hook: %w", err)
					}
				}
			}
		}
	}

	mov, err := s.Ledger.RecordMovement(fromID, toID, amount, valueTime, description)
	if err != nil {
		return nil, err
	}

	// Fire post-movement hooks
	for _, id := range []int64{fromID, toID} {
		if ma, ok := s.accounts[id]; ok {
			if ab, ok := s.behaviors[ma.BehaviorName]; ok {
				if hook, ok := ab.(MovementHook); ok {
					ctx := EventContext{Sim: s, Account: ma, AsOfDate: valueTime}
					if err := hook.PostMovement(ctx, fromID, toID, amount); err != nil {
						return nil, fmt.Errorf("post-movement hook: %w", err)
					}
				}
			}
		}
	}

	return mov, nil
}

// SetParameter sets a parameter with optional pre/post hooks.
func (s *Simulation) SetParameter(accountID int64, key, value string, effectiveAt time.Time) error {
	if ma, ok := s.accounts[accountID]; ok {
		if ab, ok := s.behaviors[ma.BehaviorName]; ok {
			if hook, ok := ab.(ParameterHook); ok {
				ctx := EventContext{Sim: s, Account: ma, AsOfDate: effectiveAt}
				if err := hook.PreParameterChange(ctx, key, value); err != nil {
					return fmt.Errorf("pre-parameter hook: %w", err)
				}
			}
		}
	}

	s.Params.Set(accountID, key, value, effectiveAt)

	if ma, ok := s.accounts[accountID]; ok {
		if ab, ok := s.behaviors[ma.BehaviorName]; ok {
			if hook, ok := ab.(ParameterHook); ok {
				ctx := EventContext{Sim: s, Account: ma, AsOfDate: effectiveAt}
				if err := hook.PostParameterChange(ctx, key, value); err != nil {
					return fmt.Errorf("post-parameter hook: %w", err)
				}
			}
		}
	}

	return nil
}

// AdvanceToDate processes each unprocessed day up to and including targetDate.
// Returns a slice of DailyUpdate, one per day processed. Each update is also
// delivered to the registered DailyUpdateHandler (if any) as it is produced.
func (s *Simulation) AdvanceToDate(targetDate time.Time) ([]DailyUpdate, error) {
	target := startOfDay(targetDate)
	var updates []DailyUpdate

	current := s.lastProcessedDate
	if current.IsZero() {
		current = s.startDate
	} else {
		current = nextDay(current)
	}

	for !current.After(target) {
		update, err := s.processEndOfDay(current)
		if err != nil {
			return updates, fmt.Errorf("process end of day %s: %w", current.Format("2006-01-02"), err)
		}
		updates = append(updates, update)
		if s.dailyUpdateHandler != nil {
			s.dailyUpdateHandler(update)
		}
		s.lastProcessedDate = current
		current = nextDay(current)
	}

	return updates, nil
}

// processEndOfDay runs end-of-day processing for all active accounts on the given date.
// It captures opening and closing balances, runs each account's EndOfDay hook
// (which typically accrues interest), and returns the collected updates.
func (s *Simulation) processEndOfDay(date time.Time) (DailyUpdate, error) {
	eod := endOfDay(date)
	update := DailyUpdate{Date: date}

	for _, ma := range s.accounts {
		if ma.Status != StatusActive {
			continue
		}
		ab, ok := s.behaviors[ma.BehaviorName]
		if !ok {
			continue
		}

		// Balance before end-of-day processing (includes day's movements, not yet interest)
		preBalance, err := s.Ledger.BalanceAt(ma.Account.ID, eod)
		if err != nil {
			return update, fmt.Errorf("pre-balance for account %d: %w", ma.Account.ID, err)
		}

		ctx := EventContext{Sim: s, Account: ma, AsOfDate: date}
		if err := ab.EndOfDay(ctx); err != nil {
			return update, fmt.Errorf("end of day for account %d: %w", ma.Account.ID, err)
		}

		// Balance after end-of-day processing (includes interest)
		postBalance, err := s.Ledger.BalanceAt(ma.Account.ID, eod)
		if err != nil {
			return update, fmt.Errorf("post-balance for account %d: %w", ma.Account.ID, err)
		}

		acctUpdate := AccountUpdate{
			Account:        ma,
			Date:           date,
			OpeningBalance: preBalance,
			ClosingBalance: postBalance,
			InterestAmount: postBalance - preBalance,
			Exponent:       ma.Account.Exponent,
		}
		update.Accounts = append(update.Accounts, acctUpdate)
	}

	return update, nil
}

// CloseAccount transitions an account through pending closure (if supported) to closed.
func (s *Simulation) CloseAccount(accountID int64) error {
	ma, ok := s.accounts[accountID]
	if !ok {
		return fmt.Errorf("unknown account: %d", accountID)
	}
	ab, ok := s.behaviors[ma.BehaviorName]
	if !ok {
		return fmt.Errorf("unknown behavior: %s", ma.BehaviorName)
	}

	if hook, ok := ab.(PendingClosureHook); ok {
		ma.Status = StatusPendingClosure
		ctx := EventContext{Sim: s, Account: ma, AsOfDate: s.Clock.Now()}
		if err := hook.OnPendingClosure(ctx); err != nil {
			return fmt.Errorf("pending closure: %w", err)
		}
	}

	ctx := EventContext{Sim: s, Account: ma, AsOfDate: s.Clock.Now()}
	return ab.OnClose(ctx)
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func endOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
}

func nextDay(t time.Time) time.Time {
	return startOfDay(t).AddDate(0, 0, 1)
}
