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
	"git.bytestone.uk/hum3/gobank/bank/history"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/bank/treasury"
	"git.bytestone.uk/hum3/gobank/cmd/demo/sim"
	"git.bytestone.uk/hum3/gobank/core"
	customers "git.bytestone.uk/hum3/gobanks-customers"
	"git.bytestone.uk/hum3/gogal"
)

// The daily series are the core's history types (ADR-0002 stage 1),
// stored as daily snapshots (bank/history).
type (
	RatePoint     = core.RatePoint
	BalancePoint  = core.BalancePoint
	CustomerPoint = core.CustomerPoint
	NIMPoint      = core.NIMPoint
)

// DemoState is the demo application: the bank (its state and the day's
// rules, ADR-0002 stages 1 to 3), wired to the simulation that drives it
// (package sim, stage 4) and to the console. The simulation reaches the
// bank only through the core; the console's methods on DemoState forward
// to it (console.go). Stage 5 moves the bank's components into packages.
type DemoState struct {
	mu                  sync.Mutex
	epoch               int    // bumped by Reset; work planned before a reset is not applied after it (mu)
	restartID           int64  // this process's row in the restart record (mu)
	dsn                 string // the database this state was opened on, "" for in-memory
	products            []Product
	nCustomers          int // customers on the books (mu)
	currentDay          time.Time
	dayCount            int
	nextPaymentID       int
	opCostPerDay        luca.Amount // minor units per day
	rng                 *rand.Rand  // the bank's randomness: account numbers
	boeRate             float64     // BoE base rate as a decimal, e.g. 0.0525; moves with the historical series (mu)
	reserveRatio        float64     // fraction of deposits held as BoE reserves, e.g. 0.15 (mu)
	nextCustSeq         int
	piiAuthorized       bool
	passRate            passThroughput
	progress            dayProgress         // the day being processed, for the runtime page
	passHook            func()              // called after each account the pass visits; tests only
	sim                 *sim.Simulation     // the simulation driving this bank through the core (wiring; console.go forwards to it)
	clock               core.Clock          // where the bank reads the time (ADR-0002 stage 4)
	rates               core.BaseRateSource // where the bank reads the base rate
	dayComplete         bool                // every account has its position for currentDay (mu)
	nimBps              float64             // the latest snapshot's NIM, for the position (mu)
	boeAccruedNumerator int64               // BoE interest on excess reserves, numerator units over gbp.AccrualDenominator
	db                  *sql.DB
	dbBackend           string             // human-readable data store description, set by initDBWithDSN
	dbIsPostgres        bool               // real PostgreSQL (pgx) rather than in-memory pglike
	ledger              *ledger.Ledger     // the books of account (bank/ledger), opened on db with the chart resolved
	treasury            *treasury.Treasury // the gilt desk (bank/treasury), opened on db
	history             *history.History   // the daily snapshots (bank/history), opened on db
	custStore           *customers.SQLCustomerStore
	boePostedPence      int64       // whole pence of BoE accrual posted to the ledger, not yet applied (mu)
	boeInterestApplied  luca.Amount // cumulative BoE interest applied into Asset:BoEReserves (mu)
	memoryExceeded      bool        // true when heap > memoryLimit; simulation pauses
	memoryLimit         uint64      // auto-stop threshold, see SetMemoryLimit
	book                bookTotals  // running customer savings/lending totals, see book.go (mu)
	dayAccrualSavings   int64       // interest accrued today on savings, numerator units over gbp.AccrualDenominator (mu)
	dayAccrualLending   int64       // same for lending (mu)
}

func NewDemoState() *DemoState {
	return NewDemoStateWithDSN("")
}

// NewDemoStateWithDSN creates a DemoState backed by the given database.
// Empty dsn uses in-memory pglike; a postgres:// DSN uses real PostgreSQL,
// and the run recorded in it, if any, resumes.
func NewDemoStateWithDSN(dsn string) *DemoState {
	return newDemoState().openOn(openDB(dsn), dsn)
}

// newDemoStateOn is a DemoState over an already open database: the next
// process over the same database, in tests.
func newDemoStateOn(db *sql.DB, dsn string) *DemoState {
	return newDemoState().openOn(db, dsn)
}

// openOn takes db as the state's database and resumes the run recorded in
// it, or records the start of a fresh one.
func (ds *DemoState) openOn(db *sql.DB, dsn string) *DemoState {
	ds.attachDB(db, dsn)
	ds.wireSimulation()
	run, resumed := loadRun(ds.db)
	if resumed {
		// The simulation resumes its clock's day; the bank's business day
		// is the latest on its own record, the daily snapshots.
		ds.sim.Resume(sim.Run{Day: run.Day, DayCount: run.DayCount, Running: run.Running, SlotStart: run.SlotStart}, run.DayLength, run.DayLengthSet)
		ds.currentDay, ds.dayCount = run.Day, run.DayCount
		if first, latest, ok, err := ds.history.Span(context.Background()); err != nil {
			log.Printf("resume: %v", err)
		} else if ok {
			ds.currentDay, ds.dayCount = latest, int(latest.Sub(first).Hours()/24)
		}
		ds.boeRate = ds.rates.BaseRate(ds.currentDay)
		ds.rng = rand.New(rand.NewSource(42 + int64(ds.dayCount)))
	}
	ds.initLedger()
	if resumed {
		ds.resumeBooks()
		if pending, err := anyUnprojected(ds.db, ds.currentDay); err == nil {
			ds.dayComplete = !pending
		}
	} else {
		saveRun(ds.db, runState{Day: ds.currentDay})
		saveSlotStart(ds.db, time.Now())
	}
	ds.saveSnapshot(ds.recordHistory())
	if latest, ok, err := ds.history.Latest(context.Background()); err != nil {
		log.Printf("resume: %v", err)
	} else if ok {
		ds.nimBps = latest.NIMBps // the day's NIM is on record; this process has no accrual yet
	}
	ds.mu.Lock()
	ds.restartID = recordStart(ds.db, version, ds.dayCount, ds.nCustomers, run.SavedAt)
	ds.mu.Unlock()
	return ds
}

// businessDay is the day the bank is on, which a component books its
// facts against.
func (ds *DemoState) businessDay() time.Time {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.currentDay
}

// resumeBooks rebuilds the in-memory picture of the bank from the
// database: the book totals, the BoE reserve's position and the interest
// applied are read back, and the sequence numbers continue from the rows
// on record. Every account's balance and accrued interest is its ledger
// position, so nothing per account is loaded.
func (ds *DemoState) resumeBooks() {
	if ds.db == nil || ds.ledger == nil {
		return
	}
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.syncFromLedgerLocked()
	ds.nCustomers = ds.custStoreCount()
	ds.nextCustSeq = ds.lastCustomerSeq() + 1
	ds.nextPaymentID = lastPaymentID(ds.db) + 1
	log.Printf("resume: day %d (%s), %d customers, savings %s, lending %s",
		ds.dayCount, ds.currentDay.Format("2006-01-02"), ds.nCustomers, fmtMoney(ds.book.Savings), fmtMoney(ds.book.Lending))
}

// newDemoState is the state of a bank on its first day, before it has a
// database.
func newDemoState() *DemoState {
	return &DemoState{
		products:      AllProducts(),
		nextPaymentID: 1,
		opCostPerDay:  50_00, // £50.00/day in minor units
		rng:           rand.New(rand.NewSource(42)),
		reserveRatio:  defaultReserveRatio,
		nextCustSeq:   1,
		memoryLimit:   defaultMemoryLimit,
		dayComplete:   true,
	}
}

// wireSimulation puts the simulation on the bank (stage 4): the bank reads
// the time and the base rate from the simulation's clock and its replayed
// series, and the simulation drives the bank through the core, with the
// run recorded in the simulation component's table. A real deployment
// would wire the wall clock and a rate feed here, and no simulation.
func (ds *DemoState) wireSimulation() {
	ds.sim = sim.New(newCoreAdapter(ds, ""), runStore{ds}, sim.Options{
		Workers: ds.dbWriters(),
		Pause:   ds.memoryPause,
		Seed:    42,
	})
	ds.clock = ds.sim.Clock()
	ds.rates = sim.BoERates
	ds.currentDay = core.BusinessDay(ds.clock.Now())
	ds.boeRate = ds.rates.BaseRate(ds.currentDay)
}

// initLedger opens the books (bank/ledger) on ds.db, with the chart of
// accounts the day's rules post against resolved.
func (ds *DemoState) initLedger() {
	ds.boePostedPence = 0
	ds.boeInterestApplied = 0
	ds.ledger = nil
	books, err := ledger.Open(ds.db)
	if err != nil {
		log.Printf("initLedger: %v", err)
		return
	}
	ds.ledger = books
}

// addCustomerToLedger opens a customer's accounts on the ledger's chart.
// The products engine is not involved: an account is its ledger account
// and the product named on the register. Must be called with ds.mu held.
func (ds *DemoState) addCustomerToLedger(books *ledger.Ledger, cust *CustomerRecord) {
	if books == nil {
		return
	}
	for i := range cust.Accounts {
		a := &cust.Accounts[i]
		id, err := books.OpenCustomerAccount(cust.ID, a.ProductID, a.Family)
		if err != nil {
			log.Print(err)
			continue
		}
		a.LedgerAccountID = id
	}
}

// saveSnapshot stores a day's snapshot on the bank's record and logs a
// refusal; the record itself is the component's (bank/history).
func (ds *DemoState) saveSnapshot(s history.Snapshot) {
	if ds.history == nil {
		return
	}
	if err := ds.history.Save(context.Background(), s); err != nil {
		log.Print(err)
	}
}

// recordHistory takes the day's snapshot — the book, the customers, the
// NIM and the base rate — for the caller to store (saveSnapshot, off the
// lock). Must be called with ds.mu held.
func (ds *DemoState) recordHistory() history.Snapshot {
	savings, lending := ds.book.Savings, ds.book.Lending
	// Today's interest in minor units, from the engine's exact accrual (rate math for the NIM ratio, not storage).
	totalDepInt := float64(ds.dayAccrualSavings) / gbp.AccrualDenominator
	totalLoanInt := float64(ds.dayAccrualLending) / gbp.AccrualDenominator

	// NIM in bps: (loan interest income + BoE interest - deposit interest expense) / total deposits * 365 * 10000
	cash := savings - lending
	requiredReserves := luca.Amount(float64(savings) * ds.reserveRatio)
	excessCash := cash - requiredReserves
	dailyBoeInt := 0.0
	if excessCash > 0 {
		dailyBoeInt = float64(excessCash) * ds.boeRate / 365.0
	}
	nimBps := 0.0
	if savings > 0 {
		nimBps = (totalLoanInt + dailyBoeInt - totalDepInt) / float64(savings) * 365.0 * 10000.0
	}
	ds.nimBps = nimBps
	return history.Snapshot{Day: ds.currentDay, Savings: savings, Lending: lending, Customers: ds.nCustomers, NIMBps: nimBps, BoERate: ds.boeRate}
}

// --- Bank simulation ---

// defaultReserveRatio is the fraction of deposits the bank holds as BoE
// reserves until a treasury policy sets it.
const defaultReserveRatio = 0.15

// lendingHeadroom returns how much additional lending the bank can take on
// while maintaining the capital reserve ratio. Must be called with ds.mu held.
func (ds *DemoState) lendingHeadroom() luca.Amount {
	deposits, loans := ds.book.Savings, ds.book.Lending
	// Required reserves = ratio * deposits. Max loans = deposits - required reserves.
	maxLoans := luca.Amount(float64(deposits) * (1 - ds.reserveRatio))
	return maxLoans - loans
}

// The start-of-day workflow
// (ADR-0002 stage 3). Yesterday's bank-level books are closed and the date
// moves on, then the pass projects every account's position for the new
// day (pass.go) at the system's capacity. A day whose pass did not
// finish — a restart, a stop — is resumed instead: the date stays and the
// pass carries on from the accounts still without a position. Reads never
// wait on the pass: an account's position for the day is written when the
// pass reaches it, and until then its latest position plus the day's
// movements is the answer. Must be called WITHOUT ds.mu held.
// advanceDayCtx is the bank following its clock (core.DayCommands.StartDay):
// a pass the day in progress still owes is finished first, then every day
// the clock has moved on to is started and its pass run, one day at a
// time, until the bank is on the clock's day. Stops early when ctx ends,
// leaving the rest for the next call. Nothing happens while the clock is
// still on the bank's day.
func (ds *DemoState) advanceDayCtx(ctx context.Context) {
	for ctx.Err() == nil {
		ds.mu.Lock()
		db, ledger := ds.db, ds.ledger
		day := ds.currentDay
		ds.mu.Unlock()

		pending, err := anyUnprojected(db, day)
		if err != nil {
			log.Printf("advanceDay: %v", err)
			return
		}
		if pending {
			ds.progress.begin(day) // resuming the day's pass
		} else if day.Before(core.BusinessDay(ds.clock.Now())) {
			day = ds.startDay(ledger)
		} else {
			return // on the clock's day, with the day's work done
		}
		visited := ds.runPass(ctx, day)

		// The throughput the dashboard quotes is the accounts the pass visited
		// over the whole day, begin to finish: the span the runtime page
		// reports as the last day's duration.
		elapsed := ds.progress.finish()
		left, err := anyUnprojected(db, day)
		ds.mu.Lock()
		ds.passRate.record(visited, elapsed)
		ds.dayComplete = err == nil && !left
		ds.mu.Unlock()
	}
}

// startDay closes yesterday's bank-level books and begins the next day:
// BoE interest accrues on the excess reserves yesterday closed with and is
// projected on the reserve account; the date and the base rate move on;
// the day's snapshot is written, so a restart from here resumes this
// day. Returns the new day. Who joins the bank on the new day
// is the simulation's business (rollNewCustomer), not the bank's. Must be
// called WITHOUT ds.mu held.
func (ds *DemoState) startDay(books *ledger.Ledger) time.Time {
	ds.mu.Lock()
	closed := ds.currentDay
	totalDeposits, totalLoans := ds.book.Savings, ds.book.Lending
	requiredReserves := luca.Amount(float64(totalDeposits) * ds.reserveRatio)
	cash := totalDeposits - totalLoans
	excessCash := cash - requiredReserves
	if excessCash > 0 {
		// Exact BoE interest accrual: numerator over gbp.AccrualDenominator.
		ds.boeAccruedNumerator += int64(excessCash) * gbp.RateBps(ds.boeRate)
	}
	boeMovements := ds.collectBoEInterest(closed)
	boeNumerator := ds.boeAccruedNumerator

	ds.currentDay = closed.AddDate(0, 0, 1)
	ds.dayCount++
	ds.dayComplete = false
	ds.boeRate = ds.rates.BaseRate(ds.currentDay)
	day := ds.currentDay

	// The snapshot is the day the bank starts, with the NIM of the day just
	// closed; the pass accrues the new day from zero.
	snapshot := ds.recordHistory()
	ds.dayAccrualSavings, ds.dayAccrualLending = 0, 0
	ds.mu.Unlock()

	ds.progress.begin(day)
	ds.progress.phase("closing yesterday", 0)
	if books != nil {
		for _, m := range boeMovements {
			if _, err := books.RecordMovement(m.from, m.to, m.amount, m.code, m.at, m.description); err != nil {
				log.Printf("advanceDay: BoE interest: %v", err)
			}
		}
		// The BoE reserves account's accrual is a position like any other.
		reserves := books.Chart.BoEReserves
		unlock := books.Lock(reserves)
		if _, err := books.Project(reserves, closed, luca.Fraction{Num: boeNumerator, Den: gbp.AccrualDenominator}); err != nil {
			log.Printf("advanceDay: BoE reserves position: %v", err)
		}
		unlock()
	}
	// The day's snapshot is the bank's record of having begun it: a
	// restart from here resumes it.
	ds.saveSnapshot(snapshot)
	return day
}

// codeDailyAccrual marks the BoE reserve's daily interest accrual movements
// (income recognised as it accrues) — distinct from the application code
// (luca.CodeInterestAccrual). Customer accounts carry their accrual on
// their positions and post nothing daily.
const codeDailyAccrual = "LDAS:FTDP:ACRU"

// dbWriters is how many connections write to the database at once when
// adding customers. On PostgreSQL the writes spread across cores; the
// in-memory pglike store (and WASM) takes one writer at a time.
func (ds *DemoState) dbWriters() int {
	if !ds.dbIsPostgres {
		return 1
	}
	return runtime.NumCPU()
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
// Sub-penny remainders carry forward in boeAccruedNumerator. The movements
// are returned for the caller to write with no locks held; a later write
// failure is logged and the counters stay ahead of the ledger. Must be
// called with ds.mu held.
func (ds *DemoState) collectBoEInterest(day time.Time) []ledgerMovement {
	if ds.ledger == nil {
		return nil
	}
	chart := ds.ledger.Chart
	var out []ledgerMovement
	valueTime := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 58, 0, day.Location())
	if newPence := ds.boeAccruedNumerator/gbp.AccrualDenominator - ds.boePostedPence; newPence > 0 {
		out = append(out, ledgerMovement{chart.IncomeBoE, chart.AccruedBoE, luca.Amount(newPence),
			codeDailyAccrual, valueTime, "Daily BoE reserve interest accrual"})
		ds.boePostedPence += newPence
	}
	if monthEnd := day.Month() != day.AddDate(0, 0, 1).Month(); monthEnd && ds.boePostedPence > 0 {
		desc := fmt.Sprintf("BoE reserve interest received for month ending %s", day.Format("2006-01-02"))
		applyTime := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, day.Location())
		out = append(out, ledgerMovement{chart.AccruedBoE, chart.BoEReserves, luca.Amount(ds.boePostedPence),
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

// refreshFromLedger reads the bank's position back from the database,
// e.g. after an import wrote movements directly. Must be called with ds.mu
// held.
func (ds *DemoState) refreshFromLedger() {
	if ds.ledger == nil {
		return
	}
	ds.syncFromLedgerLocked()
}

// syncFromLedgerLocked reads back what the database holds of the bank's
// position: the BoE reserve's accrual from its position, the book totals
// and the BoE interest applied. Accounts need nothing: each one's balance
// and accrual is its ledger position, read where it is shown. Must be
// called with ds.mu held.
func (ds *DemoState) syncFromLedgerLocked() {
	if ds.ledger == nil {
		return
	}
	reserves := ds.ledger.Chart.BoEReserves
	if p, err := ds.ledger.PositionAt(reserves, ds.currentDay); err != nil {
		log.Printf("refreshFromLedger: BoE reserves position: %v", err)
	} else if p != nil && p.Accrued.Den == gbp.AccrualDenominator {
		ds.boeAccruedNumerator = p.Accrued.Num
		// Daily posting maintains posted == floor(numerator/denominator)
		// at every sync point, so the tracker is derivable on restore.
		ds.boePostedPence = p.Accrued.Num / gbp.AccrualDenominator
	}
	ds.refreshBookTotals()
	// Applied BoE interest is derivable from its ledger account balance.
	if bal, err := ds.ledger.Balance(reserves); err == nil {
		ds.boeInterestApplied = bal
	} else {
		log.Printf("refreshFromLedger: BoE reserves balance: %v", err)
	}
}

// Reset is a fresh run on the same database: the simulation stops and its
// clock goes back to the opening day, the bank's tables go and its state
// is that of a bank on its first day.
func (ds *DemoState) Reset() {
	ds.sim.Reset(sim.OpeningDay)
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.epoch++
	ds.passRate = passThroughput{}
	ds.progress.reset()
	ds.currentDay = core.BusinessDay(ds.clock.Now())
	ds.dayCount = 0
	ds.dayComplete = true
	ds.clearPaymentsLocked()
	ds.rng = rand.New(rand.NewSource(42))
	if ds.custStore != nil {
		ds.custStore.Reset(context.Background())
	}
	ds.boeRate = ds.rates.BaseRate(ds.currentDay)
	ds.nextCustSeq = 1
	ds.nCustomers = 0
	ds.book = bookTotals{}
	ds.dayAccrualSavings, ds.dayAccrualLending = 0, 0
	ds.piiAuthorized = false
	ds.memoryExceeded = false
	ds.nimBps = 0
	ds.boeAccruedNumerator = 0
	// Clear persisted numerators so a durable (postgres) DB doesn't carry
	// accrual rows from before the reset.
	ds.clearRegisterLocked()
	if ds.dbIsPostgres {
		// A fresh run on the same database: every table goes and the
		// schema is migrated again from nothing.
		dropAllPublicTables(ds.db)
		ds.attachDB(ds.db, ds.dsn)
	} else {
		ds.initDB()
	}
	ds.initLedger()
	saveRun(ds.db, runState{Day: ds.currentDay})
	saveSlotStart(ds.db, time.Now())
	if ds.sim.DayLengthRecorded() { // the console's setting outlives the run it was made in
		saveDayLength(ds.db, ds.sim.Settings().DayLength)
	}
	ds.saveSnapshot(ds.recordHistory())
}

// position is the bank's position (core.Position) read under one lock.
func (ds *DemoState) position() core.Position {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.positionLocked()
}

// positionLocked is position with ds.mu held.
func (ds *DemoState) positionLocked() core.Position {
	savings, lending := ds.book.Savings, ds.book.Lending
	nimBps := ds.nimBps
	return core.Position{
		Day:              ds.currentDay,
		DayCount:         ds.dayCount,
		Customers:        ds.nCustomers,
		Savings:          savings,
		Lending:          lending,
		Cash:             savings - lending,
		RequiredReserves: luca.Amount(float64(savings) * ds.reserveRatio),
		ReserveRatio:     ds.reserveRatio,
		BoERate:          ds.boeRate,
		BoEInterest:      ds.boeInterestTotal(),
		NIMBps:           nimBps,
		DayComplete:      ds.dayComplete,
	}
}

// profitAndLoss is the bank's P&L to date (core.ProfitAndLoss). Customer
// interest is read from the ledger P&L accounts with no lock held.
func (ds *DemoState) profitAndLoss() core.ProfitAndLoss {
	ds.mu.Lock()
	dayCount := ds.dayCount
	opCosts := ds.opCostPerDay * luca.Amount(ds.dayCount)
	boeInterest := ds.boeInterestTotal()
	ds.mu.Unlock()
	loanIncome, depositExpense := ds.interestTotals()
	return core.ProfitAndLoss{
		DayCount:               dayCount,
		LoanInterestIncome:     loanIncome,
		BoEInterestIncome:      boeInterest,
		DepositInterestExpense: depositExpense,
		OperatingCosts:         opCosts,
	}
}

// SimStatus is the simulation console's own state: what is running and
// how fast. It is not part of the bank (ADR-0002: the console drives the
// generators) and so not a core query.
type SimStatus struct {
	Running             bool
	AddingCust          bool
	AddingProgress      int
	AddingTarget        int
	CustomersPerSec     float64 // live rate of the running batch add
	LastCustomersPerSec float64 // rate of the last finished batch add
	AccountDaysPer12h   int64   // accounts the pass would project in 12h at the measured whole-day rate
	MemoryExceeded      bool
	DayLength           time.Duration // wall-clock length of a simulated day; zero is flat out
	DayEndsIn           time.Duration // what is left of the day in progress; zero when flat out or stopped
	Wall                time.Time     // the wall clock now
	Clock               time.Time     // the bank's clock now: simulated time
	Warp                float64       // simulated seconds per wall second; zero when flat out
}

// SimStatus returns the console state under one lock.
// SimStatus is the simulation's status with the bank's pass rate and the
// process's memory state alongside, for the console.
func (ds *DemoState) SimStatus() SimStatus {
	st := ds.sim.Status()
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return SimStatus{
		Running: st.Running, AddingCust: st.AddingCust, AddingProgress: st.AddingProgress, AddingTarget: st.AddingTarget,
		CustomersPerSec: st.CustomersPerSec, LastCustomersPerSec: st.LastCustomersPerSec,
		DayLength: st.DayLength, DayEndsIn: st.DayEndsIn, Wall: st.Wall, Clock: ds.clock.Now(), Warp: st.Warp,
		AccountDaysPer12h: ds.passRate.per(passWindow),
		MemoryExceeded:    ds.memoryExceeded,
	}
}

// DashData is what the dashboard shows: the bank's position and history
// through the core, and the console's own state.
type DashData struct {
	Sim     SimStatus
	Bank    core.Position
	History core.History
}

// dashboardData gathers the dashboard's data: the bank through the core
// queries, the console from the simulation.
func dashboardData(q core.BookQueries, ds *DemoState) DashData {
	pos, _ := q.Position(context.Background())
	hist, _ := q.History(context.Background())
	return DashData{Sim: ds.SimStatus(), Bank: pos, History: hist}
}

// --- Dashboard HTML (shared by server + WASM) ---

// renderDashContent renders dashboard data sections as Bulma-styled HTML.
// Shared by HTTP server and WASM modes.
func renderDashContent(d DashData) string {
	var s strings.Builder

	// B5: Memory warning banner
	if d.Sim.MemoryExceeded {
		s.WriteString(`<div class="notification is-danger"><strong>Memory limit approaching.</strong> Simulation paused. Export data or reset to continue.</div>`)
	}

	// Summary stats level
	dateStr := d.Bank.Day.Format("2 Jan 2006")
	nimStr := fmt.Sprintf("%.0f bps", d.Bank.NIMBps)
	s.WriteString(`<nav class="level mb-4">`)
	countdown := ""
	if d.Sim.DayEndsIn > 0 {
		countdown = fmt.Sprintf(`<p class="heading">ends in %s</p>`, d.Sim.DayEndsIn.Round(time.Second))
	}
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Day</p><p class="title is-5">%d &mdash; %s</p>%s</div></div>`, d.Bank.DayCount, dateStr, countdown))
	// The two clocks side by side give a feel for the rate simulated time
	// passes at.
	warp := `<p class="heading">flat out</p>`
	if d.Sim.Warp > 0 {
		warp = fmt.Sprintf(`<p class="heading">&times;%s wall pace</p>`, strconv.FormatFloat(d.Sim.Warp, 'f', -1, 64))
	}
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Wall clock</p><p class="title is-5">%s</p></div></div>`, d.Sim.Wall.UTC().Format("15:04:05")))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Sim clock</p><p class="title is-5">%s</p>%s</div></div>`, d.Sim.Clock.UTC().Format("2 Jan 2006 15:04:05"), warp))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Customers</p><p class="title is-5">%d</p></div></div>`, d.Bank.Customers))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">NIM</p><p class="title is-5">%s</p></div></div>`, nimStr))
	if d.Sim.DayLength > 0 {
		s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Day length</p><p class="title is-5">%s</p></div></div>`, d.Sim.DayLength))
	}
	if d.Sim.AccountDaysPer12h > 0 {
		s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Account days / 12h</p><p class="title is-5">%s</p></div></div>`, groupThousands(strconv.FormatInt(d.Sim.AccountDaysPer12h, 10))))
	}
	if d.Sim.AddingCust {
		s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Adding</p><p class="title is-6">%d / %d</p><p class="heading">%.0f /s</p></div></div>`, d.Sim.AddingProgress, d.Sim.AddingTarget, d.Sim.CustomersPerSec))
	} else if d.Sim.LastCustomersPerSec > 0 {
		s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Last add</p><p class="title is-6">%.0f /s</p></div></div>`, d.Sim.LastCustomersPerSec))
	}
	s.WriteString(`</nav>`)

	// Balance boxes
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="box dash-box has-background-success-light"><p class="heading">Savings (deposits)</p><p class="title is-5">%s</p></div></div>`, fmtMoney(d.Bank.Savings)))
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="box dash-box has-background-info-light"><p class="heading">Lending (loans)</p><p class="title is-5">%s</p></div></div>`, fmtMoney(d.Bank.Lending)))
	reserveClass := "has-background-warning-light"
	if d.Bank.Cash < d.Bank.RequiredReserves {
		reserveClass = "has-background-danger-light"
	}
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="box dash-box %s"><p class="heading">BoE Cash Reserve</p><p class="title is-5">%s</p><p class="subtitle is-7 mb-0">Required: %s (%.0f%%) | BoE: %.2f%%</p></div></div>`,
		reserveClass, fmtMoney(d.Bank.Cash), fmtMoney(d.Bank.RequiredReserves), d.Bank.ReserveRatio*100, d.Bank.BoERate*100))
	s.WriteString(`</div>`)

	// Balance chart
	s.WriteString(`<h3 class="title is-6 has-text-grey mt-4 mb-2">Balance History</h3>`)
	s.WriteString(buildBalanceChartSVG(d.History.Balances))

	// Customer chart
	s.WriteString(`<h3 class="title is-6 has-text-grey mt-4 mb-2">Customer Count</h3>`)
	s.WriteString(buildCustomerChartSVG(d.History.Customers))

	return s.String()
}

// buildDashboardHTML renders the dashboard data sections as Bulma-styled
// HTML. Shared by HTTP server and WASM modes. Does not include controls.
func buildDashboardHTML(q core.BookQueries, ds *DemoState) string {
	return renderDashContent(dashboardData(q, ds))
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

// buildCustomerChartSVG renders a standalone SVG customer count chart with
// gogal: whole-number counts, the first and last day labelled at the ends of
// the time axis, calendar dates between.
func buildCustomerChartSVG(history []CustomerPoint) string {
	if len(history) == 0 {
		return ""
	}
	times := make([]time.Time, len(history))
	values := make([]float64, len(history))
	for i, cp := range history {
		times[i], values[i] = cp.Date, float64(cp.Count)
	}
	svg, err := gogal.NewLineChart(
		gogal.WithSize(660, 180),
		gogal.WithMargins(10, 40, 24, 60),
		gogal.WithLegend(false),
		gogal.WithPoints(false),
		gogal.WithTooltips(false),
		gogal.WithAccessibility(false),
		gogal.WithTimeFormat(chartDateFormat),
		gogal.WithYFormat("%.0f"),
		gogal.WithIntegerY(true),
		gogal.WithEndLabels(true),
	).AddTimeSeries("customers", times, values).RenderString()
	if err != nil {
		return fmt.Sprintf(`<p class="has-text-danger">Chart error: %v</p>`, err)
	}
	return svg
}
