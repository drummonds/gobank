package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"math/rand"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	customers "git.bytestone.uk/hum3/gobanks-customers"
	"github.com/go-analyze/charts"
)

// RatePoint records a BoE base rate at a point in simulated time.
type RatePoint struct {
	Date time.Time
	Rate float64
}

// BalancePoint records aggregate savings/lending balances at a point in simulated time.
type BalancePoint struct {
	Date    time.Time
	Savings luca.Amount // minor units
	Lending luca.Amount // minor units
}

// CustomerPoint records the customer count at a point in simulated time.
type CustomerPoint struct {
	Date  time.Time
	Count int
}

// NIMPoint records the Net Interest Margin in basis points at a point in simulated time.
type NIMPoint struct {
	Date time.Time
	NIM  float64 // annualized basis points
}

// DemoState holds all unified state for the model bank demo.
type DemoState struct {
	mu                  sync.Mutex
	epoch               int // bumped by Reset; work planned before a reset is not applied after it (mu)
	running             bool
	cancel              context.CancelFunc
	products            []Product
	nCustomers          int // customers on the books (mu)
	currentDay          time.Time
	dayCount            int
	nextPaymentID       int
	payRunning          bool
	payCancel           context.CancelFunc
	opCostPerDay        luca.Amount // minor units per day
	rng                 *rand.Rand
	settings            Settings
	nextCustSeq         int
	piiAuthorized       bool
	boeHistory          []RatePoint
	balanceHistory      []BalancePoint
	customerHistory     []CustomerPoint
	addingCustRunning   bool
	addingCustCancel    context.CancelFunc
	addingCustProgress  int
	addingCustTarget    int
	addingCustStart     time.Time
	lastAddRate         float64 // customers/s of the last finished batch
	interestRate        interestThroughput
	progress            dayProgress      // the day being processed, for the runtime page
	now                 func() time.Time // wall clock, injectable for tests
	nimHistory          []NIMPoint
	boeAccruedNumerator int64 // BoE interest on excess reserves, numerator units over gbp.AccrualDenominator
	db                  *sql.DB
	dbBackend           string // human-readable data store description, set by initDBWithDSN
	dbIsPostgres        bool   // real PostgreSQL (pgx) rather than in-memory pglike
	ledger              *luca.SQLLedger
	custStore           *customers.SQLCustomerStore
	txLog               []TxEntry
	nextTxID            int
	sim                 *gbp.Simulation
	simClock            *gbp.SimClock
	equityAccountID     string
	accrualPosted       map[string]int64 // pence posted to AccruedInterest per account, not yet reversed (simMu)
	expenseInterestID   string           // ledger IDs for accrual movements, resolved lazily (simMu)
	incomeInterestID    string
	accrSavingsID       string
	accrLendingID       string
	incomeBoEID         string
	accrBoEID           string
	boeReservesID       string
	boePostedPence      int64       // whole pence of BoE accrual posted to the ledger, not yet applied (mu)
	boeInterestApplied  luca.Amount // cumulative BoE interest applied into Asset:BoEReserves (mu)
	simMu               sync.Mutex  // serializes mutations of ds.sim in-memory state (engine sweeps vs payments)
	memoryExceeded      bool        // true when heap > memoryLimit; simulation pauses
	memoryLimit         uint64      // auto-stop threshold, see SetMemoryLimit
	book                bookTotals  // running customer savings/lending totals, see book.go (mu)
	dayAccrualSavings   int64       // interest accrued today on savings, numerator units over gbp.AccrualDenominator (mu)
	dayAccrualLending   int64       // same for lending (mu)
}

const (
	maxTxLogEntries  = 100_000 // B3: cap txLog size
	txLogTrimPercent = 10      // trim oldest 10% when exceeded
	maxHistoryPoints = 7_300   // B4: ~20 years of daily data
	memCheckInterval = 10      // check every N sim-days
)

// capSlice returns a slice trimmed to maxLen by dropping the oldest entries.
func capSlice[T any](s []T, maxLen int) []T {
	if len(s) <= maxLen {
		return s
	}
	drop := len(s) - maxLen
	copy(s, s[drop:])
	return s[:maxLen]
}

func NewDemoState() *DemoState {
	return NewDemoStateWithDSN("")
}

// NewDemoStateWithDSN creates a DemoState backed by the given database.
// Empty dsn uses in-memory pglike; a postgres:// DSN uses real PostgreSQL.
func NewDemoStateWithDSN(dsn string) *DemoState {
	startDay := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	settings := DefaultSettings()
	settings.BoEBaseRate = lookupBoERate(startDay)
	rng := rand.New(rand.NewSource(42))

	ds := &DemoState{
		products:      AllProducts(),
		currentDay:    startDay,
		nextPaymentID: 1,
		opCostPerDay:  50_00, // £50.00/day in minor units
		rng:           rng,
		settings:      settings,
		nextCustSeq:   1,
		boeHistory:    []RatePoint{{Date: startDay, Rate: settings.BoEBaseRate}},
		now:           time.Now,
		memoryLimit:   defaultMemoryLimit,
	}
	ds.initDBWithDSN(dsn)
	ds.initLedger()
	ds.recordHistory()
	return ds
}

// initLedger creates a go-luca ledger sharing ds.db and gbp simulation.
func (ds *DemoState) initLedger() {
	ds.accrualPosted = nil
	ds.expenseInterestID = ""
	ds.incomeInterestID = ""
	ds.accrSavingsID = ""
	ds.accrLendingID = ""
	ds.incomeBoEID = ""
	ds.accrBoEID = ""
	ds.boeReservesID = ""
	ds.boePostedPence = 0
	ds.boeInterestApplied = 0
	ledger, err := luca.NewSQLLedger(ds.db)
	if err != nil {
		log.Printf("initLedger: open ledger: %v", err)
		return
	}
	ds.ledger = ledger
	ds.createLedgerViews()
	ds.simClock = gbp.NewSimClock(ds.currentDay)
	sim, err := gbp.NewSimulation(ledger, ds.simClock)
	if err != nil {
		log.Printf("initLedger: %v", err)
		return
	}
	for _, p := range ds.products {
		sim.RegisterProduct(p.Product)
	}
	equityAcct, err := ledger.CreateAccount("Equity:Capital", "GBP", -2, 0)
	if err != nil {
		log.Printf("initLedger: create equity: %v", err)
		return
	}
	ds.sim = sim
	ds.equityAccountID = equityAcct.ID
	// Create the accrual accounts now, not lazily in the day sweep: that
	// runs under simMu, and a database write there can deadlock against a
	// customer transaction waiting for simMu.
	ds.simMu.Lock()
	ds.ensureAccrualAccounts()
	ds.simMu.Unlock()

	// Pace large account sweeps so the single-threaded WASM host can yield
	// to the browser event loop during month-end interest application.
	if runtime.GOOS == "js" {
		n := 0
		sim.PaceHook = func() {
			n++
			if n%64 == 0 {
				time.Sleep(time.Millisecond)
			}
		}
	}
}

// ensureLedgerAccount returns the account at path, creating it if missing.
func ensureLedgerAccount(ledger luca.Ledger, path string) (*luca.Account, error) {
	acct, err := ledger.GetAccount(path)
	if err != nil {
		return nil, err
	}
	if acct != nil {
		return acct, nil
	}
	return ledger.CreateAccount(path, "GBP", -2, 0)
}

// addCustomerToLedger registers a customer's accounts in the go-luca ledger.
// Must be called with ds.mu held.
func (ds *DemoState) addCustomerToLedger(sim *gbp.Simulation, cust *CustomerRecord) {
	if sim == nil {
		return
	}
	for i := range cust.Accounts {
		a := &cust.Accounts[i]
		pathPrefix := "Liability:Savings"
		if a.Family == gbp.FamilyLending {
			pathPrefix = "Asset:Loans"
		}
		fullPath := fmt.Sprintf("%s:%s:%s", pathPrefix, cust.ID, a.ProductID)
		params := map[string]string{
			"annual_rate": fmt.Sprintf("%f", a.Rate),
		}
		ds.simMu.Lock()
		ma, err := sim.OpenAccount(a.ProductID, fullPath, "GBP", -2, params)
		if err != nil {
			ds.simMu.Unlock()
			log.Printf("ledger: open account %s: %v", fullPath, err)
			continue
		}
		// The demo funds accounts via direct movements rather than the
		// Deposit lifecycle, so activate explicitly: the products engine
		// only accrues interest on active accounts.
		ma.Status = gbp.StatusActive
		ds.simMu.Unlock()
		a.LedgerAccountID = ma.Account.ID
	}
}

// recordHistory appends current balance/customer/NIM totals to history slices.
// Must be called with ds.mu held.
func (ds *DemoState) recordHistory() {
	savings, lending := ds.book.Savings, ds.book.Lending
	// Today's interest in minor units, from the engine's exact accrual (rate math for the NIM ratio, not storage).
	totalDepInt := float64(ds.dayAccrualSavings) / gbp.AccrualDenominator
	totalLoanInt := float64(ds.dayAccrualLending) / gbp.AccrualDenominator
	ds.balanceHistory = append(ds.balanceHistory, BalancePoint{Date: ds.currentDay, Savings: savings, Lending: lending})
	ds.customerHistory = append(ds.customerHistory, CustomerPoint{Date: ds.currentDay, Count: ds.nCustomers})

	// NIM in bps: (loan interest income + BoE interest - deposit interest expense) / total deposits * 365 * 10000
	cash := savings - lending
	requiredReserves := luca.Amount(float64(savings) * ds.settings.CapitalReserveRatio)
	excessCash := cash - requiredReserves
	dailyBoeInt := 0.0
	if excessCash > 0 {
		dailyBoeInt = float64(excessCash) * ds.settings.BoEBaseRate / 365.0
	}
	nimBps := 0.0
	if savings > 0 {
		nimBps = (totalLoanInt + dailyBoeInt - totalDepInt) / float64(savings) * 365.0 * 10000.0
	}
	ds.nimHistory = append(ds.nimHistory, NIMPoint{Date: ds.currentDay, NIM: nimBps})
}

// --- Bank simulation ---

// lendingHeadroom returns how much additional lending the bank can take on
// while maintaining the capital reserve ratio. Must be called with ds.mu held.
func (ds *DemoState) lendingHeadroom() luca.Amount {
	deposits, loans := ds.book.Savings, ds.book.Lending
	// Required reserves = ratio * deposits. Max loans = deposits - required reserves.
	maxLoans := luca.Amount(float64(deposits) * (1 - ds.settings.CapitalReserveRatio))
	return maxLoans - loans
}

// advanceDay advances one simulated day. Interest is handled by the
// gobank-products engine: exact daily accrual in memory, applied to accounts
// as ledger movements at month end. The demo additionally posts each day's
// whole accrued pence into AccruedInterest holding accounts (see
// collectAccrualMovements) and persists the exact numerators to
// accrual_state. The engine sweep runs without holding ds.mu, and all bulk
// database writes stream in ~targetTxTime transactions with no locks held,
// so dashboard reads stay responsive throughout.
// Must be called WITHOUT ds.mu held.
func (ds *DemoState) advanceDay() {
	ds.mu.Lock()
	if ds.memoryExceeded {
		ds.mu.Unlock()
		return
	}
	sim := ds.sim
	day := ds.currentDay
	ds.mu.Unlock()
	ds.progress.begin(day)
	defer ds.progress.finish()

	// Phase 1: products engine end-of-day (and month-end application) —
	// no ds.mu held; simMu serializes against payments touching sim state.
	// Only in-memory work happens under simMu; the day's accrual movements
	// are collected here and streamed to the ledger below with no locks
	// held, so customer adds and dashboard reads are never stuck behind a
	// day's worth of database writes.
	var accrualRows []accrualRow
	var newCustomer *customerPlan // today's new customer, persisted once ds.mu is released
	var accrualBatches []accrualBatch
	var appliedSavings, appliedLending luca.Amount // month-end interest applied to customer balances
	var accrualSavings, accrualLending int64       // today's accrual, numerator units
	var interestTxs []TxEntry                      // customer-facing entries for applied interest
	accrualStart := ds.now()
	movements := 0
	if sim != nil {
		engine := "products engine"
		if day.Month() != day.AddDate(0, 0, 1).Month() {
			engine += " (month-end interest application)"
		}
		ds.progress.phase(engine, 0)
		ds.simMu.Lock()
		updates, err := sim.AdvanceToDate(day)
		accrualBatches = ds.collectAccrualMovements(updates)
		// Snapshot each swept account's numerator while simMu still guards
		// engine state: on month-end days the update's recorded numerator and
		// closing balance are pre-application, so read the live
		// post-application values instead; the balance difference is the
		// interest applied to the account.
		seen := make(map[string]bool)
		for _, du := range updates {
			for _, au := range du.Accounts {
				if au.Account.Family == gbp.FamilySavings {
					accrualSavings += au.AccruedDelta
				} else {
					accrualLending += au.AccruedDelta
				}
				if applied := au.Account.CachedBalance - au.ClosingBalance; applied != 0 {
					if au.Account.Family == gbp.FamilySavings {
						appliedSavings += applied
					} else {
						appliedLending += applied
					}
					interestTxs = append(interestTxs, ds.interestTx(day, au.Account, applied))
				}
				id := au.Account.Account.ID
				if !seen[id] {
					seen[id] = true
					accrualRows = append(accrualRows, accrualRow{id: id, numerator: au.Account.AccruedNumerator})
				}
			}
		}
		ds.simMu.Unlock()
		if err != nil {
			log.Printf("advanceDay: products engine: %v", err)
		}
		for _, b := range accrualBatches {
			movements += len(b.inputs)
		}
		ds.progress.phase("accrual postings", movements)
		for _, b := range accrualBatches {
			ds.writeMovementsChunked(b)
		}
	}
	accrualElapsed := ds.now().Sub(accrualStart)

	// Phase 2: sync account mirrors from the engine and do day bookkeeping
	// under a single short lock hold.
	ds.progress.phase("bookkeeping", 0)
	ds.mu.Lock()
	ds.interestRate.record(movements, accrualElapsed)
	ds.book.Savings += appliedSavings
	ds.book.Lending += appliedLending
	ds.dayAccrualSavings, ds.dayAccrualLending = accrualSavings, accrualLending
	for _, tx := range interestTxs {
		ds.appendTx(tx)
	}
	totalDeposits, totalLoans := ds.book.Savings, ds.book.Lending

	requiredReserves := luca.Amount(float64(totalDeposits) * ds.settings.CapitalReserveRatio)
	cash := totalDeposits - totalLoans
	excessCash := cash - requiredReserves
	if excessCash > 0 {
		// Exact BoE interest accrual: numerator over gbp.AccrualDenominator.
		ds.boeAccruedNumerator += int64(excessCash) * int64(math.Round(ds.settings.BoEBaseRate*10_000))
	}
	boeMovements := ds.collectBoEInterest(day)

	ds.currentDay = ds.currentDay.AddDate(0, 0, 1)
	ds.dayCount++
	if ds.simClock != nil {
		ds.simMu.Lock() // the engine reads the clock under simMu, e.g. opening accounts
		ds.simClock.SetDate(ds.currentDay)
		ds.simMu.Unlock()
	}

	ds.settings.BoEBaseRate = lookupBoERate(ds.currentDay)
	ds.boeHistory = append(ds.boeHistory, RatePoint{Date: ds.currentDay, Rate: ds.settings.BoEBaseRate})

	if ds.nCustomers < ds.settings.MaxCustomers {
		boeRate := ds.settings.BoEBaseRate
		avgSavings := averageRate(ds.products, gbp.FamilySavings)
		avgLending := averageRate(ds.products, gbp.FamilyLending)

		savingsAttract := 0.0
		lendingAttract := 0.0
		if boeRate > 0 {
			savingsAttract = (avgSavings - boeRate) / boeRate
			lendingAttract = (boeRate - avgLending) / boeRate
		}
		attractiveness := clamp((savingsAttract+lendingAttract)/2, 0, 1)
		dailyProb := 0.10 + attractiveness*0.20

		if ds.rng.Float64() < dailyProb {
			p := ds.planCustomerLocked()
			newCustomer = &p
		}
	}

	// B3: Cap txLog
	if len(ds.txLog) > maxTxLogEntries {
		trim := len(ds.txLog) * txLogTrimPercent / 100
		copy(ds.txLog, ds.txLog[trim:])
		ds.txLog = ds.txLog[:len(ds.txLog)-trim]
	}

	ds.recordHistory()

	// B4: Cap history arrays
	ds.boeHistory = capSlice(ds.boeHistory, maxHistoryPoints)
	ds.balanceHistory = capSlice(ds.balanceHistory, maxHistoryPoints)
	ds.customerHistory = capSlice(ds.customerHistory, maxHistoryPoints)
	ds.nimHistory = capSlice(ds.nimHistory, maxHistoryPoints)

	// B2: Periodic memory check
	if ds.dayCount%memCheckInterval == 0 && heapExceeds(ds.memoryLimit) {
		ds.memoryExceeded = true
		ds.running = false
		if ds.cancel != nil {
			ds.cancel()
			ds.cancel = nil
		}
	}

	boeNumerator := ds.boeAccruedNumerator
	db, ledger := ds.db, ds.ledger
	ds.mu.Unlock()

	if ledger != nil {
		for _, m := range boeMovements {
			if _, err := ledger.RecordMovement(m.from, m.to, m.amount, m.code, m.at, m.description); err != nil {
				log.Printf("advanceDay: BoE interest: %v", err)
			}
		}
	}

	// Phase 3: persist accrual numerators off both locks so the DB alone
	// carries the accrued-but-unapplied interest state for the day.
	persistAccrualState(db, day, accrualRows, boeNumerator)
	if newCustomer != nil {
		ds.persistCustomerPlan(*newCustomer)
	}
}

// codeDailyAccrual marks daily interest accrual movements and their month-end
// reversals — distinct from the engine's application code (luca.CodeInterestAccrual).
const codeDailyAccrual = "LDAS:FTDP:ACRU"

// ensureAccrualAccounts resolves (creating on first use) the P&L and
// AccruedInterest holding accounts used by daily accrual movements.
// Must be called with ds.simMu held.
func (ds *DemoState) ensureAccrualAccounts() bool {
	if ds.expenseInterestID != "" {
		return true
	}
	for _, t := range []struct {
		path string
		dst  *string
	}{
		{"Expense:Interest", &ds.expenseInterestID},
		{"Income:Interest", &ds.incomeInterestID},
		{"Liability:AccruedInterest", &ds.accrSavingsID},
		{"Asset:AccruedInterest", &ds.accrLendingID},
		{"Income:Interest:BoE", &ds.incomeBoEID},
		{"Asset:AccruedInterest:BoE", &ds.accrBoEID},
		{"Asset:BoEReserves", &ds.boeReservesID},
	} {
		acct, err := ensureLedgerAccount(ds.sim.Ledger, t.path)
		if err != nil {
			log.Printf("ensureAccrualAccounts: %s: %v", t.path, err)
			ds.expenseInterestID = ""
			return false
		}
		*t.dst = acct.ID
	}
	return true
}

// accrualBatch is one day's accrual movements: collected in memory under
// ds.simMu, then written to the ledger with no locks held.
type accrualBatch struct {
	valueTime time.Time
	inputs    []luca.MovementInput
}

// targetTxTime is the wall-clock budget for one streamed write transaction.
// The design goal is smooth streaming under load: at the ultimate capacity of
// one real day per simulated day, writes should trickle continuously in small
// transactions that other writers can interleave with, instead of arriving as
// one monolithic burst that holds locks and demands oversized hardware.
const targetTxTime = 10 * time.Millisecond

// movementRecorder is the ledger write postMovements needs.
type movementRecorder interface {
	RecordLinkedMovements(movements []luca.MovementInput, valueTime time.Time) (string, error)
}

// dbWriters is how many connections write to the database at once — posting
// a day's accrual movements or adding customers. On PostgreSQL the writes
// spread across cores; the in-memory pglike store (and WASM) takes one
// writer at a time.
func (ds *DemoState) dbWriters() int {
	if !ds.dbIsPostgres {
		return 1
	}
	return runtime.NumCPU()
}

// writeMovementsChunked posts one day's accrual batch with no locks held, so
// customer creation, payments and dashboard reads interleave between chunks.
func (ds *DemoState) writeMovementsChunked(b accrualBatch) {
	if ds.ledger == nil {
		return
	}
	if err := postMovements(ds.ledger, b, ds.dbWriters(), ds.progress.add); err != nil {
		log.Printf("writeMovementsChunked: %v", err)
	}
}

// postMovements writes b's movements with up to workers concurrent writers,
// each in transactions sized to roughly targetTxTime, adapting its chunk
// length to its own measured cost. Every movement is posted at most once;
// after the first error no further chunks start and that error is returned.
// posted is called with the size of each chunk written.
func postMovements(rec movementRecorder, b accrualBatch, workers int, posted func(int)) error {
	var (
		mu       sync.Mutex
		next     int
		firstErr error
	)
	// take claims the next n unposted movements, or none once done or failed.
	take := func(n int) []luca.MovementInput {
		mu.Lock()
		defer mu.Unlock()
		if firstErr != nil || next >= len(b.inputs) {
			return nil
		}
		j := min(next+n, len(b.inputs))
		chunk := b.inputs[next:j]
		next = j
		return chunk
	}
	var wg sync.WaitGroup
	for range max(workers, 1) {
		wg.Go(func() {
			n := 64
			for chunk := take(n); chunk != nil; chunk = take(n) {
				start := time.Now()
				if _, err := rec.RecordLinkedMovements(chunk, b.valueTime); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
				posted(len(chunk))
				if el := time.Since(start); el > 0 {
					n = min(max(int(float64(len(chunk))*float64(targetTxTime)/float64(el)), 16), 8192)
				}
				if runtime.GOOS == "js" {
					time.Sleep(time.Millisecond) // yield to the browser event loop
				}
			}
		})
	}
	wg.Wait()
	return firstErr
}

// collectAccrualMovements builds each day's interest accrual movements:
// newly accrued whole pence move from the P&L interest account into an
// AccruedInterest holding account daily, and on month-end days the holding
// account is emptied back to P&L, because the engine's application movement
// posts the full month's interest from the same P&L account to the customer
// account. Net P&L and customer balances therefore stay exactly the engine's;
// the holding accounts expose the intra-month accrual. Sub-penny remainders
// live in accrual_state, not the ledger.
//
// Only ds.accrualPosted and in-memory batches are touched here — the caller
// writes the returned batches via writeMovementsChunked after releasing the
// locks. A later write failure leaves accrualPosted ahead of the ledger; the
// demo logs and carries on (startup begins from an empty database, and import
// rebuilds state from the file). Must be called with ds.simMu held.
func (ds *DemoState) collectAccrualMovements(updates []gbp.DailyUpdate) []accrualBatch {
	if ds.sim == nil || len(updates) == 0 || !ds.ensureAccrualAccounts() {
		return nil
	}
	if ds.accrualPosted == nil {
		ds.accrualPosted = make(map[string]int64)
	}
	var batches []accrualBatch
	for _, du := range updates {
		monthEnd := du.Date.Month() != du.Date.AddDate(0, 0, 1).Month()
		// Just before the engine's application movements at 23:59:59.
		valueTime := time.Date(du.Date.Year(), du.Date.Month(), du.Date.Day(), 23, 59, 58, 0, du.Date.Location())
		var inputs []luca.MovementInput
		for _, au := range du.Accounts {
			id := au.Account.Account.ID
			pnlID, holdID := ds.expenseInterestID, ds.accrSavingsID
			if au.Account.Family == gbp.FamilyLending {
				pnlID, holdID = ds.incomeInterestID, ds.accrLendingID
			}
			posted := ds.accrualPosted[id]
			// au.AccruedNumerator is this day's post-accrual, pre-application
			// value, so the difference is the account's unposted whole pence.
			if newPence := au.AccruedNumerator/gbp.AccrualDenominator - posted; newPence > 0 {
				inputs = append(inputs, luca.MovementInput{
					FromAccountID: pnlID, ToAccountID: holdID, Amount: luca.Amount(newPence),
					Code: codeDailyAccrual, Description: "Daily interest accrual",
				})
				posted += newPence
			}
			if monthEnd && posted > 0 {
				inputs = append(inputs, luca.MovementInput{
					FromAccountID: holdID, ToAccountID: pnlID, Amount: luca.Amount(posted),
					Code: codeDailyAccrual, Description: fmt.Sprintf("Accrual reversal on application %s", du.Date.Format("2006-01-02")),
				})
				posted = 0
			}
			ds.accrualPosted[id] = posted
		}
		if len(inputs) > 0 {
			batches = append(batches, accrualBatch{valueTime: valueTime, inputs: inputs})
		}
	}
	return batches
}

// ledgerMovement is a movement decided under the locks and written to the
// ledger after they are released.
type ledgerMovement struct {
	from, to    string
	amount      luca.Amount
	code        string
	at          time.Time
	description string
}

// collectBoEInterest models BoE reserve interest in the ledger: newly accrued
// whole pence post daily as Income:Interest:BoE -> Asset:AccruedInterest:BoE
// (income recognised as it accrues, receivable builds up), and at month end
// the receivable moves into Asset:BoEReserves as the interest is received.
// Unlike customer interest there is no reversal — the engine is not involved,
// so income is never double-posted. Sub-penny remainders carry forward in
// boeAccruedNumerator. The movements are returned for the caller to write
// with no locks held; as with customer accruals, a later write failure is
// logged and the counters stay ahead of the ledger. Must be called with
// ds.mu held.
func (ds *DemoState) collectBoEInterest(day time.Time) []ledgerMovement {
	if ds.sim == nil {
		return nil
	}
	ds.simMu.Lock()
	ok := ds.ensureAccrualAccounts()
	ds.simMu.Unlock()
	if !ok {
		return nil
	}
	var out []ledgerMovement
	valueTime := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 58, 0, day.Location())
	if newPence := ds.boeAccruedNumerator/gbp.AccrualDenominator - ds.boePostedPence; newPence > 0 {
		out = append(out, ledgerMovement{ds.incomeBoEID, ds.accrBoEID, luca.Amount(newPence),
			codeDailyAccrual, valueTime, "Daily BoE reserve interest accrual"})
		ds.boePostedPence += newPence
	}
	if monthEnd := day.Month() != day.AddDate(0, 0, 1).Month(); monthEnd && ds.boePostedPence > 0 {
		desc := fmt.Sprintf("BoE reserve interest received for month ending %s", day.Format("2006-01-02"))
		applyTime := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, day.Location())
		out = append(out, ledgerMovement{ds.accrBoEID, ds.boeReservesID, luca.Amount(ds.boePostedPence),
			luca.CodeInterestAccrual, applyTime, desc})
		ds.boeAccruedNumerator -= ds.boePostedPence * gbp.AccrualDenominator
		ds.boeInterestApplied += luca.Amount(ds.boePostedPence)
		ds.boePostedPence = 0
	}
	return out
}

// boeInterestTotal returns cumulative BoE reserve interest earned: applied
// into Asset:BoEReserves plus accrued-but-unapplied whole pence.
// Must be called with ds.mu held.
func (ds *DemoState) boeInterestTotal() luca.Amount {
	return ds.boeInterestApplied + luca.Amount(ds.boeAccruedNumerator/gbp.AccrualDenominator)
}

func (ds *DemoState) AdvanceDay() {
	ds.advanceDay()
}

// refreshFromLedger re-primes the engine's cached balances from the ledger
// and syncs account mirrors, e.g. after an import wrote movements directly.
// Must be called with ds.mu held.
func (ds *DemoState) refreshFromLedger() {
	if ds.sim == nil {
		return
	}
	ds.simMu.Lock()
	defer ds.simMu.Unlock()
	if err := ds.sim.RefreshBalances(); err != nil {
		log.Printf("refreshFromLedger: %v", err)
		return
	}
	ds.loadAccrualState()
	ds.refreshBookTotals()
	// Applied BoE interest is derivable from its ledger account balance.
	if ds.ensureAccrualAccounts() {
		if bal, err := ds.sim.Ledger.Balance(ds.boeReservesID); err == nil {
			ds.boeInterestApplied = bal
		} else {
			log.Printf("refreshFromLedger: BoE reserves balance: %v", err)
		}
	}
}

// recordSimMovement posts a movement through the products simulation so its
// cached balances stay in sync, serialized against the daily engine sweep.
// Must be called with ds.mu held.
func (ds *DemoState) recordSimMovement(fromID, toID string, amount luca.Amount, code, description string) {
	ds.recordSimMovementOn(ds.sim, ds.currentDay, fromID, toID, amount, code, description)
}

// recordSimMovementOn is recordSimMovement against an explicit simulation —
// used with a tx-bound sim during transactional customer creation.
func (ds *DemoState) recordSimMovementOn(sim *gbp.Simulation, day time.Time, fromID, toID string, amount luca.Amount, code, description string) {
	if sim == nil || fromID == "" || toID == "" {
		return
	}
	ds.simMu.Lock()
	defer ds.simMu.Unlock()
	if _, err := sim.RecordMovement(fromID, toID, amount, code, day, description); err != nil {
		log.Printf("recordSimMovement: %v", err)
	}
}

func (ds *DemoState) Start() {
	ds.mu.Lock()
	if ds.running {
		ds.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ds.cancel = cancel
	ds.running = true
	ds.mu.Unlock()

	go func() {
		// Self-pace: wait 200ms after each day completes rather than a
		// fixed-rate ticker. In WASM a ticker deadlocks the page once
		// advanceDay takes longer than the interval: the next tick is always
		// already due, the Go scheduler never goes idle, and control never
		// returns to the JS event loop — UI frozen, days advancing flat-out
		// until the tab runs out of memory.
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(200 * time.Millisecond):
				ds.advanceDay()
			}
		}
	}()
}

func (ds *DemoState) Stop() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if !ds.running {
		return
	}
	ds.running = false
	if ds.cancel != nil {
		ds.cancel()
		ds.cancel = nil
	}
}

func (ds *DemoState) IsRunning() bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.running
}

// AddCustomersBatch starts adding n customers in the background, with one
// worker per database writer. Each worker holds ds.mu only to plan a
// customer, so other operations proceed while customers are persisted.
func (ds *DemoState) AddCustomersBatch(n int) {
	ds.mu.Lock()
	if ds.addingCustRunning {
		ds.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ds.addingCustRunning = true
	ds.addingCustCancel = cancel
	ds.addingCustProgress = 0
	ds.addingCustTarget = n
	ds.addingCustStart = ds.now()
	workers := ds.dbWriters()
	ds.mu.Unlock()

	claimed := 0 // customers planned by this batch (mu)
	go func() {
		defer cancel()
		var wg sync.WaitGroup
		for range workers {
			wg.Go(func() {
				for ctx.Err() == nil {
					ds.mu.Lock()
					if claimed >= n || ds.nCustomers >= ds.settings.MaxCustomers {
						ds.mu.Unlock()
						return
					}
					claimed++
					p := ds.planCustomerLocked()
					ds.mu.Unlock()

					ds.persistCustomerPlan(p)

					ds.mu.Lock()
					if ctx.Err() == nil {
						ds.addingCustProgress++
					}
					ds.mu.Unlock()
					if runtime.GOOS == "js" {
						time.Sleep(time.Millisecond) // yield to the browser event loop
					}
				}
			})
		}
		wg.Wait()
		ds.mu.Lock()
		if ctx.Err() == nil { // a Reset has already finished a cancelled batch
			ds.finishAddingLocked()
		}
		ds.mu.Unlock()
	}()
}

// finishAddingLocked ends a batch add, keeping its customers/s for the
// dashboard. Must be called with ds.mu held.
func (ds *DemoState) finishAddingLocked() {
	if ds.addingCustProgress > 0 {
		ds.lastAddRate = perSecond(ds.addingCustProgress, ds.now().Sub(ds.addingCustStart))
	}
	ds.addingCustRunning = false
	ds.addingCustCancel = nil
	ds.addingCustProgress = 0
	ds.addingCustTarget = 0
}

// IsAddingCustomers returns true if a batch add is in progress.
func (ds *DemoState) IsAddingCustomers() bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.addingCustRunning
}

func (ds *DemoState) Reset() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.epoch++
	if ds.running {
		ds.running = false
		if ds.cancel != nil {
			ds.cancel()
			ds.cancel = nil
		}
	}
	if ds.payRunning {
		ds.payRunning = false
		if ds.payCancel != nil {
			ds.payCancel()
			ds.payCancel = nil
		}
	}
	if ds.addingCustRunning {
		if ds.addingCustCancel != nil {
			ds.addingCustCancel()
		}
		ds.finishAddingLocked()
	}
	ds.lastAddRate = 0
	ds.interestRate = interestThroughput{}
	ds.progress.reset()
	ds.currentDay = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	ds.dayCount = 0
	ds.clearPaymentsLocked()
	ds.rng = rand.New(rand.NewSource(42))
	if ds.custStore != nil {
		ds.custStore.Reset(context.Background())
	}
	ds.settings = DefaultSettings()
	ds.settings.BoEBaseRate = lookupBoERate(ds.currentDay)
	ds.nextCustSeq = 1
	ds.nCustomers = 0
	ds.book = bookTotals{}
	ds.dayAccrualSavings, ds.dayAccrualLending = 0, 0
	ds.piiAuthorized = false
	ds.memoryExceeded = false
	ds.boeHistory = []RatePoint{{Date: ds.currentDay, Rate: ds.settings.BoEBaseRate}}
	ds.balanceHistory = nil
	ds.customerHistory = nil
	ds.nimHistory = nil
	ds.boeAccruedNumerator = 0
	ds.txLog = nil
	ds.nextTxID = 0
	// Clear persisted numerators so a durable (postgres) DB doesn't carry
	// accrual rows from before the reset.
	ds.clearRegisterLocked()
	ds.clearAccrualLocked()
	ds.initDB()
	ds.initLedger()
	ds.recordHistory()
}

// DashData holds a snapshot of all dashboard state, grabbed under one lock.
type DashData struct {
	Day                 time.Time
	DayCount            int
	Savings             luca.Amount
	Lending             luca.Amount
	Cash                luca.Amount
	RequiredReserves    luca.Amount
	CapitalReserveRatio float64
	BoeRate             float64
	BoeInterest         luca.Amount
	NIMBps              float64
	CustomerCount       int
	Running             bool
	AddingCust          bool
	AddingProgress      int
	AddingTarget        int
	CustomersPerSec     float64 // live rate of the running batch add
	LastCustomersPerSec float64 // rate of the last finished batch add
	InterestPer12h      int64   // interest movements the engine posts per 12h at its measured rate
	MemoryExceeded      bool
	BalanceHistory      []BalancePoint
	CustomerHistory     []CustomerPoint
	NIMHistory          []NIMPoint
}

// DashboardData returns a snapshot of all dashboard-relevant state.
func (ds *DemoState) DashboardData() DashData {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	savings, lending := ds.book.Savings, ds.book.Lending

	balHist := make([]BalancePoint, len(ds.balanceHistory))
	copy(balHist, ds.balanceHistory)
	custHist := make([]CustomerPoint, len(ds.customerHistory))
	copy(custHist, ds.customerHistory)
	nimHist := make([]NIMPoint, len(ds.nimHistory))
	copy(nimHist, ds.nimHistory)

	nimBps := 0.0
	if len(nimHist) > 0 {
		nimBps = nimHist[len(nimHist)-1].NIM
	}
	addRate := 0.0
	if ds.addingCustRunning {
		addRate = perSecond(ds.addingCustProgress, ds.now().Sub(ds.addingCustStart))
	}

	return DashData{
		Day:                 ds.currentDay,
		DayCount:            ds.dayCount,
		Savings:             savings,
		Lending:             lending,
		Cash:                savings - lending,
		RequiredReserves:    luca.Amount(float64(savings) * ds.settings.CapitalReserveRatio),
		CapitalReserveRatio: ds.settings.CapitalReserveRatio,
		BoeRate:             ds.settings.BoEBaseRate,
		BoeInterest:         ds.boeInterestTotal(),
		NIMBps:              nimBps,
		CustomerCount:       ds.nCustomers,
		Running:             ds.running,
		AddingCust:          ds.addingCustRunning,
		AddingProgress:      ds.addingCustProgress,
		AddingTarget:        ds.addingCustTarget,
		CustomersPerSec:     addRate,
		LastCustomersPerSec: ds.lastAddRate,
		InterestPer12h:      ds.interestRate.per(interestWindow),
		MemoryExceeded:      ds.memoryExceeded,
		BalanceHistory:      balHist,
		CustomerHistory:     custHist,
		NIMHistory:          nimHist,
	}
}

// --- Dashboard HTML (shared by server + WASM) ---

// renderDashContent renders dashboard data sections as Bulma-styled HTML.
// Shared by HTTP server and WASM modes.
func renderDashContent(d DashData) string {
	var s strings.Builder

	// B5: Memory warning banner
	if d.MemoryExceeded {
		s.WriteString(`<div class="notification is-danger"><strong>Memory limit approaching.</strong> Simulation paused. Export data or reset to continue.</div>`)
	}

	// Summary stats level
	dateStr := d.Day.Format("2 Jan 2006")
	nimStr := fmt.Sprintf("%.0f bps", d.NIMBps)
	s.WriteString(`<nav class="level mb-4">`)
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Day</p><p class="title is-5">%d &mdash; %s</p></div></div>`, d.DayCount, dateStr))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Customers</p><p class="title is-5">%d</p></div></div>`, d.CustomerCount))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">NIM</p><p class="title is-5">%s</p></div></div>`, nimStr))
	if d.InterestPer12h > 0 {
		s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Interest movements / 12h</p><p class="title is-5">%s</p></div></div>`, groupThousands(strconv.FormatInt(d.InterestPer12h, 10))))
	}
	if d.AddingCust {
		s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Adding</p><p class="title is-6">%d / %d</p><p class="heading">%.0f /s</p></div></div>`, d.AddingProgress, d.AddingTarget, d.CustomersPerSec))
	} else if d.LastCustomersPerSec > 0 {
		s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Last add</p><p class="title is-6">%.0f /s</p></div></div>`, d.LastCustomersPerSec))
	}
	s.WriteString(`</nav>`)

	// Balance boxes
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="box dash-box has-background-success-light"><p class="heading">Savings (deposits)</p><p class="title is-5">%s</p></div></div>`, fmtMoney(d.Savings)))
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="box dash-box has-background-info-light"><p class="heading">Lending (loans)</p><p class="title is-5">%s</p></div></div>`, fmtMoney(d.Lending)))
	reserveClass := "has-background-warning-light"
	if d.Cash < d.RequiredReserves {
		reserveClass = "has-background-danger-light"
	}
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="box dash-box %s"><p class="heading">BoE Cash Reserve</p><p class="title is-5">%s</p><p class="subtitle is-7 mb-0">Required: %s (%.0f%%) | BoE: %.2f%%</p></div></div>`,
		reserveClass, fmtMoney(d.Cash), fmtMoney(d.RequiredReserves), d.CapitalReserveRatio*100, d.BoeRate*100))
	s.WriteString(`</div>`)

	// Balance chart
	s.WriteString(`<h3 class="title is-6 has-text-grey mt-4 mb-2">Balance History</h3>`)
	s.WriteString(buildBalanceChartSVG(d.BalanceHistory))

	// Customer chart
	s.WriteString(`<h3 class="title is-6 has-text-grey mt-4 mb-2">Customer Count</h3>`)
	s.WriteString(buildCustomerChartSVG(d.CustomerHistory))

	return s.String()
}

// BuildDashboardHTML renders the dashboard data sections as Bulma-styled HTML.
// Shared by HTTP server and WASM modes. Does not include controls.
func (ds *DemoState) BuildDashboardHTML() string {
	return renderDashContent(ds.DashboardData())
}

// buildNIMChart renders a single-line chart of NIM in basis points as an SVG fragment.
func buildNIMChart(history []NIMPoint) string {
	if len(history) == 0 {
		return ""
	}
	const (
		padL   = 80
		padR   = 20
		width  = 660
		chartW = width - padL - padR
		chartH = 140
		padT   = 10
	)

	minVal := history[0].NIM
	maxVal := history[0].NIM
	for _, np := range history {
		if np.NIM < minVal {
			minVal = np.NIM
		}
		if np.NIM > maxVal {
			maxVal = np.NIM
		}
	}
	valRange := maxVal - minVal
	if valRange < 10 {
		valRange = 20
		minVal -= 10
		maxVal += 10
	} else {
		minVal -= valRange * 0.1
		maxVal += valRange * 0.1
		valRange = maxVal - minVal
	}

	var s strings.Builder
	totalH := padT + chartH + 10
	s.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg" style="width:100%%;height:auto">`, width, totalH))

	// Background
	s.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="#fafafa" stroke="#dbdbdb" stroke-width="1"/>`, padL, padT, chartW, chartH))

	// Y-axis labels
	for i := 0; i <= 4; i++ {
		val := minVal + valRange*float64(i)/4.0
		y := float64(padT+chartH) - float64(chartH)*float64(i)/4.0
		s.WriteString(fmt.Sprintf(`<line x1="%d" y1="%.0f" x2="%d" y2="%.0f" stroke="#ededed" stroke-width="1"/>`, padL, y, padL+chartW, y))
		s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" text-anchor="end" font-size="9" fill="#7a7a7a">%.0f</text>`, padL-5, y+3, val))
	}

	// Line
	if len(history) == 1 {
		v := history[0].NIM
		x := float64(padL) + float64(chartW)/2
		y := float64(padT+chartH) - float64(chartH)*(v-minVal)/valRange
		s.WriteString(fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="3" fill="#f59e0b"/>`, x, y))
	} else {
		var pts strings.Builder
		for i, np := range history {
			v := np.NIM
			x := float64(padL) + float64(chartW)*float64(i)/float64(len(history)-1)
			y := float64(padT+chartH) - float64(chartH)*(v-minVal)/valRange
			y = math.Max(float64(padT), math.Min(float64(padT+chartH), y))
			if i == 0 {
				pts.WriteString(fmt.Sprintf("%.1f,%.1f", x, y))
			} else {
				pts.WriteString(fmt.Sprintf(" %.1f,%.1f", x, y))
			}
		}
		s.WriteString(fmt.Sprintf(`<polyline points="%s" fill="none" stroke="#f59e0b" stroke-width="2"/>`, pts.String()))
	}

	s.WriteString(`</svg>`)
	return s.String()
}

// buildBalanceChartSVG renders a standalone SVG balance chart (for HTML dashboard sections).
func buildBalanceChartSVG(history []BalancePoint) string {
	if len(history) == 0 {
		return ""
	}
	const (
		padL   = 80
		padR   = 20
		width  = 660
		chartW = width - padL - padR
		chartH = 160
		padT   = 10
	)

	// Geometry in float64 minor units (display math only — money stays integer).
	minVal := float64(history[0].Savings)
	maxVal := float64(history[0].Savings)
	for _, bp := range history {
		for _, v := range []float64{float64(bp.Savings), float64(bp.Lending)} {
			if v < minVal {
				minVal = v
			}
			if v > maxVal {
				maxVal = v
			}
		}
	}
	valRange := maxVal - minVal
	if valRange < 100_00 {
		valRange = 200_00
		minVal -= 100_00
		maxVal += 100_00
	} else {
		minVal -= valRange * 0.1
		maxVal += valRange * 0.1
		valRange = maxVal - minVal
	}
	if minVal < 0 {
		minVal = 0
		valRange = maxVal - minVal
	}

	var s strings.Builder
	totalH := padT + chartH + 25
	s.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg" style="width:100%%;height:auto">`, width, totalH))
	s.WriteString(`<style>text{font-family:Arial,Helvetica,sans-serif}</style>`)

	// Background
	s.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="#fafafa" stroke="#dbdbdb" stroke-width="1"/>`, padL, padT, chartW, chartH))

	// Y-axis labels
	for i := 0; i <= 4; i++ {
		val := minVal + valRange*float64(i)/4.0
		y := float64(padT+chartH) - float64(chartH)*float64(i)/4.0
		s.WriteString(fmt.Sprintf(`<line x1="%d" y1="%.0f" x2="%d" y2="%.0f" stroke="#ededed" stroke-width="1"/>`, padL, y, padL+chartW, y))
		s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" text-anchor="end" font-size="9" fill="#7a7a7a">%s</text>`, padL-5, y+3, fmtMoney(luca.Amount(math.Round(val)))))
	}

	// Lines
	type lineSpec struct {
		color string
		vals  func(BalancePoint) float64
	}
	lines := []lineSpec{
		{"#48c78e", func(bp BalancePoint) float64 { return float64(bp.Savings) }},
		{"#3e8ed0", func(bp BalancePoint) float64 { return float64(bp.Lending) }},
	}
	for _, line := range lines {
		if len(history) == 1 {
			v := line.vals(history[0])
			x := float64(padL) + float64(chartW)/2
			y := float64(padT+chartH) - float64(chartH)*(v-minVal)/valRange
			s.WriteString(fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="3" fill="%s"/>`, x, y, line.color))
			continue
		}
		var pts strings.Builder
		for i, bp := range history {
			v := line.vals(bp)
			x := float64(padL) + float64(chartW)*float64(i)/float64(len(history)-1)
			y := float64(padT+chartH) - float64(chartH)*(v-minVal)/valRange
			y = math.Max(float64(padT), math.Min(float64(padT+chartH), y))
			if i == 0 {
				pts.WriteString(fmt.Sprintf("%.1f,%.1f", x, y))
			} else {
				pts.WriteString(fmt.Sprintf(" %.1f,%.1f", x, y))
			}
		}
		s.WriteString(fmt.Sprintf(`<polyline points="%s" fill="none" stroke="%s" stroke-width="2"/>`, pts.String(), line.color))
	}

	// Legend
	legendY := float64(padT+chartH) + 18
	s.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%.0f" r="4" fill="#48c78e"/>`, padL, legendY-3))
	s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" font-size="10" fill="#363636">Savings</text>`, padL+8, legendY))
	s.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%.0f" r="4" fill="#3e8ed0"/>`, padL+70, legendY-3))
	s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" font-size="10" fill="#363636">Lending</text>`, padL+78, legendY))

	s.WriteString(`</svg>`)
	return s.String()
}

// buildCustomerChartSVG renders a standalone SVG customer count chart using go-analyze/charts
// (local fork adding XAxisOption.CustomTicks for calendar-aware tick marks).
func buildCustomerChartSVG(history []CustomerPoint) string {
	if len(history) == 0 {
		return ""
	}

	values := make([]float64, len(history))
	minCount, maxCount := history[0].Count, history[0].Count
	for i, cp := range history {
		values[i] = float64(cp.Count)
		if cp.Count < minCount {
			minCount = cp.Count
		}
		if cp.Count > maxCount {
			maxCount = cp.Count
		}
	}
	yMin, yMax, yLabels := integerYAxis(minCount, maxCount)
	xTicks := timeAxisTicks(history[0].Date, history[len(history)-1].Date)

	p, err := charts.LineRender(
		[][]float64{values},
		charts.SVGOutputOptionFunc(),
		charts.DimensionsOptionFunc(660, 180),
		charts.LegendOptionFunc(charts.LegendOption{Show: new(false)}),
		charts.PaddingOptionFunc(charts.Box{Left: 60, Right: 40, Top: 10, Bottom: 5, IsSet: true}),
		func(opt *charts.ChartOption) {
			opt.Symbol = charts.SymbolNone
			opt.LineStrokeWidth = 2
			opt.XAxis.BoundaryGap = new(false)
			opt.XAxis.CustomTicks = xTicks
			opt.YAxis = []charts.YAxisOption{{
				Min:        new(yMin),
				Max:        new(yMax),
				LabelCount: yLabels,
				ValueFormatter: func(f float64) string {
					return strconv.Itoa(int(math.Round(f)))
				},
			}}
		},
	)
	if err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">Chart error: %v</p>`, err)
	}
	svgBytes, err := p.Bytes()
	if err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">Chart render error: %v</p>`, err)
	}
	return string(svgBytes)
}
