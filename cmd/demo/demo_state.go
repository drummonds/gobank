package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"time"

	luca "codeberg.org/hum3/go-luca"
	gbp "codeberg.org/hum3/gobank-products"
	customers "codeberg.org/hum3/gobanks-customers"
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
	Savings float64
	Lending float64
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
	mu                    sync.Mutex
	running               bool
	cancel                context.CancelFunc
	products              []Product
	customers             []CustomerRecord
	payments              []Payment
	currentDay            time.Time
	dayCount              int
	nextPaymentID         int
	payRunning            bool
	payCancel             context.CancelFunc
	opCostPerDay          float64
	rng                   *rand.Rand
	settings              Settings
	nextCustSeq           int
	piiAuthorized         bool
	boeHistory            []RatePoint
	balanceHistory        []BalancePoint
	customerHistory       []CustomerPoint
	addingCustRunning     bool
	addingCustCancel      context.CancelFunc
	addingCustProgress    int
	addingCustTarget      int
	nimHistory            []NIMPoint
	boeInterestAccum      float64
	db                    *sql.DB
	custStore             *customers.SQLCustomerStore
	txLog                 []TxEntry
	nextTxID              int
	sim                   *gbp.Simulation
	simClock              *gbp.SimClock
	equityAccountID       string
	interestExpenseAcctID string
	interestIncomeAcctID  string
	pendingMovements      []pendingMovement // deferred ledger writes
	memoryExceeded        bool              // true when heap > 800MB (WASM safety)
}

// pendingMovement holds a ledger movement deferred from the hot advanceDay loop.
// Flushed to SQL in batches before export or on demand.
type pendingMovement struct {
	fromID, toID string
	amount       luca.Amount
	code         string
	valueTime    time.Time
	description  string
}

const (
	maxTxLogEntries  = 100_000           // B3: cap txLog size
	txLogTrimPercent = 10                // trim oldest 10% when exceeded
	maxHistoryPoints = 7_300             // B4: ~20 years of daily data
	memoryLimitBytes = 800 * 1024 * 1024 // B2: auto-stop threshold
	memCheckInterval = 10                // check every N sim-days
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

// checkMemory reads heap stats and returns true if memory is exceeded.
func checkMemory() bool {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc > memoryLimitBytes
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
		opCostPerDay:  50.0,
		rng:           rng,
		settings:      settings,
		nextCustSeq:   1,
		boeHistory:    []RatePoint{{Date: startDay, Rate: settings.BoEBaseRate}},
	}
	ds.initDBWithDSN(dsn)
	ds.initLedger()
	ds.recordHistory()
	return ds
}

// initLedger creates a go-luca ledger sharing ds.db and gbp simulation.
func (ds *DemoState) initLedger() {
	ledger, err := luca.NewSQLLedger(ds.db)
	if err != nil {
		log.Printf("initLedger: open ledger: %v", err)
		return
	}
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

	// Cache interest account IDs (created by EnsureInterestAccounts in NewSimulation).
	if expAcct, err := ledger.GetAccount("Expense:Interest"); err == nil && expAcct != nil {
		ds.interestExpenseAcctID = expAcct.ID
	}
	if incAcct, err := ledger.GetAccount("Income:Interest"); err == nil && incAcct != nil {
		ds.interestIncomeAcctID = incAcct.ID
	}
}

// persistCustomer writes a customer record and PII to the SQL customer store.
// Must be called with ds.mu held.
func (ds *DemoState) persistCustomer(cust *CustomerRecord, pii PIIInput) {
	if ds.custStore == nil {
		return
	}
	rec := customers.CustomerRecord{
		ID:            cust.ID,
		Ref:           cust.ID, // ref == id in demo (e.g. "cust-001")
		JoinDate:      cust.JoinDate,
		KYCVerified:   cust.KYCStatus.Verified,
		KYCLastCheck:  cust.KYCStatus.LastCheckDate,
		KYCRiskRating: cust.KYCStatus.RiskRating,
	}
	cpii := customers.PIIInput{
		Name:    pii.Name,
		NI:      pii.NI,
		DOB:     pii.DOB,
		Address: pii.Address,
		Email:   pii.Email,
		Phone:   pii.Phone,
	}
	if err := ds.custStore.Create(context.Background(), rec, cpii); err != nil {
		log.Printf("persistCustomer: %v", err)
	}
}

// addCustomerToLedger registers a customer's accounts in the go-luca ledger.
// Must be called with ds.mu held.
func (ds *DemoState) addCustomerToLedger(cust *CustomerRecord) {
	if ds.sim == nil {
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
		ma, err := ds.sim.OpenAccount(a.ProductID, fullPath, "GBP", -2, params)
		if err != nil {
			log.Printf("ledger: open account %s: %v", fullPath, err)
			continue
		}
		a.LedgerAccountID = ma.Account.ID
	}
}

// recordHistory appends current balance/customer/NIM totals to history slices.
// Must be called with ds.mu held.
func (ds *DemoState) recordHistory() {
	var savings, lending float64
	var totalLoanInt, totalDepInt float64
	for _, c := range ds.customers {
		for _, a := range c.Accounts {
			if a.Family == gbp.FamilySavings {
				savings += a.Balance
				totalDepInt += a.Balance * a.Rate / 365.0
			} else {
				lending += a.Balance
				totalLoanInt += a.Balance * a.Rate / 365.0
			}
		}
	}
	ds.balanceHistory = append(ds.balanceHistory, BalancePoint{Date: ds.currentDay, Savings: savings, Lending: lending})
	ds.customerHistory = append(ds.customerHistory, CustomerPoint{Date: ds.currentDay, Count: len(ds.customers)})

	// NIM in bps: (loan interest income + BoE interest - deposit interest expense) / total deposits * 365 * 10000
	cash := savings - lending
	requiredReserves := savings * ds.settings.CapitalReserveRatio
	excessCash := cash - requiredReserves
	dailyBoeInt := 0.0
	if excessCash > 0 {
		dailyBoeInt = excessCash * ds.settings.BoEBaseRate / 365.0
	}
	nimBps := 0.0
	if savings > 0 {
		nimBps = (totalLoanInt + dailyBoeInt - totalDepInt) / savings * 365.0 * 10000.0
	}
	ds.nimHistory = append(ds.nimHistory, NIMPoint{Date: ds.currentDay, NIM: nimBps})
}

// --- Bank simulation ---

// lendingHeadroom returns how much additional lending the bank can take on
// while maintaining the capital reserve ratio. Must be called with ds.mu held.
func (ds *DemoState) lendingHeadroom() float64 {
	var deposits, loans float64
	for _, c := range ds.customers {
		for _, a := range c.Accounts {
			if a.Family == gbp.FamilySavings {
				deposits += a.Balance
			} else {
				loans += a.Balance
			}
		}
	}
	// Required reserves = ratio * deposits. Max loans = deposits - required reserves.
	maxLoans := deposits * (1 - ds.settings.CapitalReserveRatio)
	return maxLoans - loans
}

// advanceDay advances one day, releasing ds.mu between customers so that
// concurrent reads are not blocked for the entire duration.
// Must be called WITHOUT ds.mu held.
func (ds *DemoState) advanceDay() {
	ds.mu.Lock()
	if ds.memoryExceeded {
		ds.mu.Unlock()
		return
	}
	nCust := len(ds.customers)
	ds.mu.Unlock()

	// Phase 1: Interest accrual — yield lock between customers.
	// Ledger movements are deferred to pendingMovements to avoid SQL in the hot loop.
	var totalDeposits, totalLoans float64
	for ci := range nCust {
		ds.mu.Lock()
		for ai := range ds.customers[ci].Accounts {
			a := &ds.customers[ci].Accounts[ai]
			dailyRate := a.Rate / 365.0
			interest := a.Balance * dailyRate
			a.Balance += interest
			a.Interest += interest
			if a.Family == gbp.FamilySavings {
				totalDeposits += a.Balance
				if interest != 0 {
					ds.emitTx(ds.currentDay, ds.customers[ci].ID, ai, a.ProductName, TxInterestCredit, interest, a.Balance, "INT")
					if ds.sim != nil && a.LedgerAccountID != "" && ds.interestExpenseAcctID != "" {
						ds.pendingMovements = append(ds.pendingMovements, pendingMovement{
							fromID: ds.interestExpenseAcctID, toID: a.LedgerAccountID,
							amount: poundsToPence(interest), code: luca.CodeInterestAccrual,
							valueTime: ds.currentDay, description: "INT",
						})
					}
				}
			} else {
				totalLoans += a.Balance
				if interest != 0 {
					ds.emitTx(ds.currentDay, ds.customers[ci].ID, ai, a.ProductName, TxInterestDebit, interest, a.Balance, "INT")
					if ds.sim != nil && a.LedgerAccountID != "" && ds.interestIncomeAcctID != "" {
						ds.pendingMovements = append(ds.pendingMovements, pendingMovement{
							fromID: a.LedgerAccountID, toID: ds.interestIncomeAcctID,
							amount: poundsToPence(interest), code: luca.CodeInterestAccrual,
							valueTime: ds.currentDay, description: "INT",
						})
					}
				}
			}
		}
		ds.mu.Unlock()
	}

	// Phase 2: Finalize — single lock hold for day bookkeeping
	ds.mu.Lock()
	requiredReserves := totalDeposits * ds.settings.CapitalReserveRatio
	cash := totalDeposits - totalLoans
	excessCash := cash - requiredReserves
	if excessCash > 0 {
		ds.boeInterestAccum += excessCash * ds.settings.BoEBaseRate / 365.0
	}

	ds.currentDay = ds.currentDay.AddDate(0, 0, 1)
	ds.dayCount++
	if ds.simClock != nil {
		ds.simClock.SetDate(ds.currentDay)
	}

	ds.settings.BoEBaseRate = lookupBoERate(ds.currentDay)
	ds.boeHistory = append(ds.boeHistory, RatePoint{Date: ds.currentDay, Rate: ds.settings.BoEBaseRate})

	if len(ds.customers) < ds.settings.MaxCustomers {
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
			cust, pii := generateCustomer(ds.rng, ds.nextCustSeq, ds.products, ds.currentDay)
			ds.nextCustSeq++
			ds.persistCustomer(&cust, pii)
			ds.customers = append(ds.customers, cust)
			ds.addCustomerToLedger(&ds.customers[len(ds.customers)-1])
			ds.fundCustomer(len(ds.customers) - 1)
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
	if ds.dayCount%memCheckInterval == 0 && checkMemory() {
		ds.memoryExceeded = true
		ds.running = false
		if ds.cancel != nil {
			ds.cancel()
			ds.cancel = nil
		}
	}

	ds.mu.Unlock()
}

func (ds *DemoState) AdvanceDay() {
	ds.advanceDay()
}

// flushPendingMovements writes deferred interest movements to the ledger.
// Must be called with ds.mu held.
func (ds *DemoState) flushPendingMovements() {
	if ds.sim == nil || len(ds.pendingMovements) == 0 {
		return
	}
	// Group by date so each RecordLinkedMovements call shares one value_time.
	type batch struct {
		valueTime time.Time
		inputs    []luca.MovementInput
	}
	var batches []batch
	batchIdx := make(map[time.Time]int)
	for _, pm := range ds.pendingMovements {
		idx, ok := batchIdx[pm.valueTime]
		if !ok {
			idx = len(batches)
			batchIdx[pm.valueTime] = idx
			batches = append(batches, batch{valueTime: pm.valueTime})
		}
		batches[idx].inputs = append(batches[idx].inputs, luca.MovementInput{
			FromAccountID: pm.fromID,
			ToAccountID:   pm.toID,
			Amount:        pm.amount,
			Code:          pm.code,
			Description:   pm.description,
		})
	}
	for _, b := range batches {
		if _, err := ds.sim.Ledger.RecordLinkedMovements(b.inputs, b.valueTime); err != nil {
			log.Printf("flushPendingMovements: %v", err)
		}
	}
	ds.pendingMovements = ds.pendingMovements[:0]
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
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
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

// AddCustomersBatch starts a background goroutine that generates n customers,
// yielding the lock per customer so other operations can proceed.
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
	ds.mu.Unlock()

	go func() {
		for i := range n {
			select {
			case <-ctx.Done():
				ds.mu.Lock()
				ds.addingCustRunning = false
				ds.addingCustCancel = nil
				ds.addingCustProgress = 0
				ds.addingCustTarget = 0
				ds.mu.Unlock()
				return
			default:
			}
			ds.mu.Lock()
			if len(ds.customers) >= ds.settings.MaxCustomers {
				ds.addingCustRunning = false
				ds.addingCustCancel = nil
				ds.addingCustProgress = 0
				ds.addingCustTarget = 0
				ds.mu.Unlock()
				cancel()
				return
			}
			cust, pii := generateCustomer(ds.rng, ds.nextCustSeq, ds.products, ds.currentDay)
			ds.nextCustSeq++
			ds.persistCustomer(&cust, pii)
			ds.customers = append(ds.customers, cust)
			ds.addCustomerToLedger(&ds.customers[len(ds.customers)-1])
			ds.fundCustomer(len(ds.customers) - 1)
			ds.addingCustProgress = i + 1
			ds.mu.Unlock()
			time.Sleep(time.Millisecond) // yield to JS event loop in WASM
		}
		ds.mu.Lock()
		ds.addingCustRunning = false
		ds.addingCustCancel = nil
		ds.addingCustProgress = 0
		ds.addingCustTarget = 0
		ds.mu.Unlock()
	}()
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
		ds.addingCustRunning = false
		if ds.addingCustCancel != nil {
			ds.addingCustCancel()
			ds.addingCustCancel = nil
		}
		ds.addingCustProgress = 0
		ds.addingCustTarget = 0
	}
	ds.currentDay = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	ds.dayCount = 0
	ds.payments = nil
	ds.nextPaymentID = 1
	ds.rng = rand.New(rand.NewSource(42))
	if ds.custStore != nil {
		ds.custStore.Reset(context.Background())
	}
	ds.settings = DefaultSettings()
	ds.settings.BoEBaseRate = lookupBoERate(ds.currentDay)
	ds.nextCustSeq = 1
	ds.customers = nil
	ds.piiAuthorized = false
	ds.memoryExceeded = false
	ds.boeHistory = []RatePoint{{Date: ds.currentDay, Rate: ds.settings.BoEBaseRate}}
	ds.balanceHistory = nil
	ds.customerHistory = nil
	ds.nimHistory = nil
	ds.boeInterestAccum = 0
	ds.txLog = nil
	ds.nextTxID = 0
	ds.pendingMovements = nil
	ds.initDB()
	ds.initLedger()
	ds.recordHistory()
}

// DashData holds a snapshot of all dashboard state, grabbed under one lock.
type DashData struct {
	Day                 time.Time
	DayCount            int
	Savings             float64
	Lending             float64
	Cash                float64
	RequiredReserves    float64
	CapitalReserveRatio float64
	BoeRate             float64
	BoeInterest         float64
	NIMBps              float64
	CustomerCount       int
	Running             bool
	AddingCust          bool
	AddingProgress      int
	AddingTarget        int
	MemoryExceeded      bool
	BalanceHistory      []BalancePoint
	CustomerHistory     []CustomerPoint
	NIMHistory          []NIMPoint
}

// DashboardData returns a snapshot of all dashboard-relevant state.
func (ds *DemoState) DashboardData() DashData {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	var savings, lending float64
	for _, c := range ds.customers {
		for _, a := range c.Accounts {
			if a.Family == gbp.FamilySavings {
				savings += a.Balance
			} else {
				lending += a.Balance
			}
		}
	}

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

	return DashData{
		Day:                 ds.currentDay,
		DayCount:            ds.dayCount,
		Savings:             savings,
		Lending:             lending,
		Cash:                savings - lending,
		RequiredReserves:    savings * ds.settings.CapitalReserveRatio,
		CapitalReserveRatio: ds.settings.CapitalReserveRatio,
		BoeRate:             ds.settings.BoEBaseRate,
		BoeInterest:         ds.boeInterestAccum,
		NIMBps:              nimBps,
		CustomerCount:       len(ds.customers),
		Running:             ds.running,
		AddingCust:          ds.addingCustRunning,
		AddingProgress:      ds.addingCustProgress,
		AddingTarget:        ds.addingCustTarget,
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
	if d.AddingCust {
		s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Adding</p><p class="title is-6">%d / %d</p></div></div>`, d.AddingProgress, d.AddingTarget))
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

	minVal := history[0].Savings
	maxVal := history[0].Savings
	for _, bp := range history {
		for _, v := range []float64{bp.Savings, bp.Lending} {
			if v < minVal {
				minVal = v
			}
			if v > maxVal {
				maxVal = v
			}
		}
	}
	valRange := maxVal - minVal
	if valRange < 100 {
		valRange = 200
		minVal -= 100
		maxVal += 100
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
		s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" text-anchor="end" font-size="9" fill="#7a7a7a">%s</text>`, padL-5, y+3, fmtMoney(val)))
	}

	// Lines
	type lineSpec struct {
		color string
		vals  func(BalancePoint) float64
	}
	lines := []lineSpec{
		{"#48c78e", func(bp BalancePoint) float64 { return bp.Savings }},
		{"#3e8ed0", func(bp BalancePoint) float64 { return bp.Lending }},
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

// buildCustomerChartSVG renders a standalone SVG customer count chart using go-analyze/charts.
func buildCustomerChartSVG(history []CustomerPoint) string {
	if len(history) == 0 {
		return ""
	}

	// Build data and labels
	values := make([]float64, len(history))
	labels := make([]string, len(history))
	for i, cp := range history {
		values[i] = float64(cp.Count)
		labels[i] = cp.Date.Format("Jan 06")
	}

	// Thin labels to max ~6
	if len(labels) > 6 {
		step := len(labels) / 6
		for i := range labels {
			if i%step != 0 && i != len(labels)-1 {
				labels[i] = ""
			}
		}
	}

	p, err := charts.LineRender(
		[][]float64{values},
		charts.SVGOutputOptionFunc(),
		charts.DimensionsOptionFunc(660, 180),
		charts.XAxisLabelsOptionFunc(labels),
		charts.LegendOptionFunc(charts.LegendOption{Show: new(false)}),
		charts.PaddingOptionFunc(charts.Box{Left: 60, Right: 10, Top: 10, Bottom: 10, IsSet: true}),
		func(opt *charts.ChartOption) {
			opt.Symbol = charts.SymbolNone
			opt.LineStrokeWidth = 2
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
