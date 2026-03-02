package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"time"
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
	mu                 sync.Mutex
	running            bool
	cancel             context.CancelFunc
	products           []Product
	customers          []CustomerRecord
	payments           []Payment
	currentDay         time.Time
	dayCount           int
	nextPaymentID      int
	payRunning         bool
	payCancel          context.CancelFunc
	opCostPerDay       float64
	rng                *rand.Rand
	piiStore           *PIIStore
	settings           Settings
	nextCustSeq        int
	piiAuthorized      bool
	boeHistory         []RatePoint
	balanceHistory     []BalancePoint
	customerHistory    []CustomerPoint
	addingCustRunning  bool
	addingCustCancel   context.CancelFunc
	addingCustProgress int
	addingCustTarget   int
	nimHistory         []NIMPoint
	boeInterestAccum   float64
}

func NewDemoState() *DemoState {
	piiStore := NewPIIStore()
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
		piiStore:      piiStore,
		settings:      settings,
		nextCustSeq:   len(seedCustomers) + 1,
		boeHistory:    []RatePoint{{Date: startDay, Rate: settings.BoEBaseRate}},
	}
	ds.customers = initCustomers(ds.rng, ds.products, ds.currentDay, piiStore)
	ds.recordHistory()
	return ds
}

// recordHistory appends current balance/customer/NIM totals to history slices.
// Must be called with ds.mu held.
func (ds *DemoState) recordHistory() {
	var savings, lending float64
	var totalLoanInt, totalDepInt float64
	for _, c := range ds.customers {
		for _, a := range c.Accounts {
			if a.Family == FamilySavings {
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
			if a.Family == FamilySavings {
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

// constrainLending strips or reduces lending accounts on a new customer
// to stay within the capital reserve requirement. Must be called with ds.mu held.
func (ds *DemoState) constrainLending(cust *CustomerRecord) {
	headroom := ds.lendingHeadroom()
	if headroom <= 0 {
		// No room for any lending — remove all lending accounts
		kept := cust.Accounts[:0]
		for _, a := range cust.Accounts {
			if a.Family == FamilySavings {
				kept = append(kept, a)
			}
		}
		cust.Accounts = kept
		return
	}
	for i := range cust.Accounts {
		if cust.Accounts[i].Family == FamilyLending {
			if cust.Accounts[i].Balance > headroom {
				cust.Accounts[i].Balance = headroom
			}
			headroom -= cust.Accounts[i].Balance
			if headroom <= 0 {
				headroom = 0
			}
		}
	}
}

func (ds *DemoState) advanceDay() {
	// Interest accrual
	var totalDeposits, totalLoans float64
	for ci := range ds.customers {
		for ai := range ds.customers[ci].Accounts {
			a := &ds.customers[ci].Accounts[ai]
			dailyRate := a.Rate / 365.0
			interest := a.Balance * dailyRate
			a.Balance += interest
			a.Interest += interest
			if a.Family == FamilySavings {
				totalDeposits += a.Balance
			} else {
				totalLoans += a.Balance
			}
		}
	}

	// BoE interest on excess cash (only on reserves above the required minimum)
	requiredReserves := totalDeposits * ds.settings.CapitalReserveRatio
	cash := totalDeposits - totalLoans
	excessCash := cash - requiredReserves
	if excessCash > 0 {
		ds.boeInterestAccum += excessCash * ds.settings.BoEBaseRate / 365.0
	}

	ds.currentDay = ds.currentDay.AddDate(0, 0, 1)
	ds.dayCount++

	// Look up BoE rate for current day and record history
	ds.settings.BoEBaseRate = lookupBoERate(ds.currentDay)
	ds.boeHistory = append(ds.boeHistory, RatePoint{Date: ds.currentDay, Rate: ds.settings.BoEBaseRate})

	// Customer generation
	if len(ds.customers) < ds.settings.MaxCustomers {
		boeRate := ds.settings.BoEBaseRate
		avgSavings := averageRate(ds.products, FamilySavings)
		avgLending := averageRate(ds.products, FamilyLending)

		savingsAttract := 0.0
		lendingAttract := 0.0
		if boeRate > 0 {
			savingsAttract = (avgSavings - boeRate) / boeRate
			lendingAttract = (boeRate - avgLending) / boeRate
		}
		attractiveness := clamp((savingsAttract+lendingAttract)/2, 0, 1)
		dailyProb := 0.10 + attractiveness*0.20

		if ds.rng.Float64() < dailyProb {
			cust, name, ni := generateCustomer(ds.rng, ds.nextCustSeq, ds.products, ds.currentDay)
			ds.constrainLending(&cust)
			ds.nextCustSeq++
			_ = ds.piiStore.Store(cust.ID, name, ni)
			if len(cust.Accounts) > 0 {
				ds.customers = append(ds.customers, cust)
			}
		}
	}

	ds.recordHistory()
}

func (ds *DemoState) AdvanceDay() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.advanceDay()
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
				ds.mu.Lock()
				ds.advanceDay()
				ds.mu.Unlock()
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
		for i := 0; i < n; i++ {
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
			cust, name, ni := generateCustomer(ds.rng, ds.nextCustSeq, ds.products, ds.currentDay)
			ds.constrainLending(&cust)
			ds.nextCustSeq++
			_ = ds.piiStore.Store(cust.ID, name, ni)
			if len(cust.Accounts) > 0 {
				ds.customers = append(ds.customers, cust)
			}
			ds.addingCustProgress = i + 1
			ds.mu.Unlock()
			runtime.Gosched()
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
	ds.piiStore.Reset()
	ds.settings = DefaultSettings()
	ds.settings.BoEBaseRate = lookupBoERate(ds.currentDay)
	ds.nextCustSeq = len(seedCustomers) + 1
	ds.piiAuthorized = false
	ds.boeHistory = []RatePoint{{Date: ds.currentDay, Rate: ds.settings.BoEBaseRate}}
	ds.balanceHistory = nil
	ds.customerHistory = nil
	ds.nimHistory = nil
	ds.boeInterestAccum = 0
	ds.customers = initCustomers(ds.rng, ds.products, ds.currentDay, ds.piiStore)
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
			if a.Family == FamilySavings {
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
		BalanceHistory:      balHist,
		CustomerHistory:     custHist,
		NIMHistory:          nimHist,
	}
}

// --- Dashboard SVG ---

func (ds *DemoState) buildSVG() string {
	ds.mu.Lock()
	day := ds.currentDay
	dayCount := ds.dayCount

	savingsTotal := 0.0
	lendingTotal := 0.0
	totalInterest := 0.0
	for _, c := range ds.customers {
		for _, a := range c.Accounts {
			if a.Family == FamilySavings {
				savingsTotal += a.Balance
			} else {
				lendingTotal += a.Balance
			}
			totalInterest += a.Interest
		}
	}
	customerCount := len(ds.customers)
	addingProgress := ds.addingCustProgress
	addingTarget := ds.addingCustTarget
	addingRunning := ds.addingCustRunning
	boeRate := ds.settings.BoEBaseRate
	boeInterest := ds.boeInterestAccum
	cash := savingsTotal - lendingTotal
	balHist := make([]BalancePoint, len(ds.balanceHistory))
	copy(balHist, ds.balanceHistory)
	custHist := make([]CustomerPoint, len(ds.customerHistory))
	copy(custHist, ds.customerHistory)
	nimHist := make([]NIMPoint, len(ds.nimHistory))
	copy(nimHist, ds.nimHistory)
	ds.mu.Unlock()

	// Current NIM bps
	nimBps := 0.0
	if len(nimHist) > 0 {
		nimBps = nimHist[len(nimHist)-1].NIM
	}

	var s strings.Builder
	const (
		width   = 700
		height  = 700
		barMaxW = 350.0
		barH    = 30
		barX    = 170
	)

	maxBal := savingsTotal
	if lendingTotal > maxBal {
		maxBal = lendingTotal
	}
	if maxBal < 100 {
		maxBal = 100
	}

	s.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg" style="max-width:%dpx;width:100%%;height:auto">`, width, height, width))
	s.WriteString(`<style>text{font-family:Arial,Helvetica,sans-serif}</style>`)

	// Header
	s.WriteString(`<rect x="0" y="0" width="700" height="50" rx="6" fill="#00d1b2"/>`)
	s.WriteString(`<text x="20" y="33" font-size="22" font-weight="bold" fill="#fff">Model Bank</text>`)
	dateStr := day.Format("2 Jan 2006")
	s.WriteString(fmt.Sprintf(`<text x="680" y="20" text-anchor="end" font-size="14" fill="#fff">Day %d — %s</text>`, dayCount, dateStr))
	if addingRunning {
		s.WriteString(fmt.Sprintf(`<text x="680" y="35" text-anchor="end" font-size="11" fill="rgba(255,255,255,0.8)">Adding customers: %d / %d</text>`, addingProgress, addingTarget))
	} else {
		s.WriteString(fmt.Sprintf(`<text x="680" y="35" text-anchor="end" font-size="11" fill="rgba(255,255,255,0.8)">Customers: %d | Cash: %s (BoE: %.2f%%) | NIM: %.0f bps</text>`,
			customerCount, fmtMoney(cash), boeRate*100, nimBps))
	}
	_ = boeInterest

	// Family bars
	families := []struct {
		Name    string
		Balance float64
		Color   string
	}{
		{"Savings (deposits)", savingsTotal, "#48c78e"},
		{"Lending (loans)", lendingTotal, "#3e8ed0"},
	}

	yStart := 75.0
	rowH := 65.0
	for i, f := range families {
		y := yStart + float64(i)*rowH
		s.WriteString(fmt.Sprintf(`<text x="20" y="%.0f" font-size="15" font-weight="bold" fill="#363636">%s</text>`, y+12, f.Name))
		s.WriteString(fmt.Sprintf(`<rect x="%d" y="%.0f" width="%.0f" height="%d" rx="4" fill="#f0f0f0" stroke="#dbdbdb" stroke-width="1"/>`, barX, y, barMaxW, barH))
		barW := barMaxW * f.Balance / maxBal
		if barW < 2 {
			barW = 2
		}
		if barW > barMaxW {
			barW = barMaxW
		}
		s.WriteString(fmt.Sprintf(`<rect x="%d" y="%.0f" width="%.1f" height="%d" rx="4" fill="%s" opacity="0.85"/>`, barX, y, barW, barH, f.Color))
		s.WriteString(fmt.Sprintf(`<text x="%.0f" y="%.0f" font-size="14" fill="#363636" font-weight="bold">%s</text>`, float64(barX)+barMaxW+10, y+20, fmtMoney(f.Balance)))
	}

	// Divider
	divY := yStart + 2*rowH
	s.WriteString(fmt.Sprintf(`<line x1="20" y1="%.0f" x2="680" y2="%.0f" stroke="#dbdbdb" stroke-width="1"/>`, divY, divY))

	// --- Balance history chart ---
	chartTop := divY + 15
	s.WriteString(fmt.Sprintf(`<text x="20" y="%.0f" font-size="13" font-weight="bold" fill="#7a7a7a">Balance History</text>`, chartTop+12))
	s.WriteString(buildBalanceChart(balHist, chartTop+20))

	// --- Customer count chart ---
	custChartTop := chartTop + 20 + 200 + 15
	s.WriteString(fmt.Sprintf(`<text x="20" y="%.0f" font-size="13" font-weight="bold" fill="#7a7a7a">Customer Count</text>`, custChartTop+12))
	s.WriteString(buildCustomerChart(custHist, custChartTop+20))

	s.WriteString(`</svg>`)
	return s.String()
}

// buildBalanceChart renders a dual-line chart (savings=green, lending=blue) as an SVG <g>.
func buildBalanceChart(history []BalancePoint, yOffset float64) string {
	if len(history) == 0 {
		return ""
	}
	const (
		padL   = 80
		padR   = 20
		chartW = 700 - padL - padR
		chartH = 160
		padT   = 10
	)

	// Find Y range
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

	// Background
	s.WriteString(fmt.Sprintf(`<rect x="%d" y="%.0f" width="%d" height="%d" fill="#fafafa" stroke="#dbdbdb" stroke-width="1"/>`, padL, yOffset+float64(padT), chartW, chartH))

	// Y-axis labels
	for i := 0; i <= 4; i++ {
		val := minVal + valRange*float64(i)/4.0
		y := yOffset + float64(padT+chartH) - float64(chartH)*float64(i)/4.0
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
			y := yOffset + float64(padT+chartH) - float64(chartH)*(v-minVal)/valRange
			s.WriteString(fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="3" fill="%s"/>`, x, y, line.color))
			continue
		}
		var pts strings.Builder
		for i, bp := range history {
			v := line.vals(bp)
			x := float64(padL) + float64(chartW)*float64(i)/float64(len(history)-1)
			y := yOffset + float64(padT+chartH) - float64(chartH)*(v-minVal)/valRange
			y = math.Max(yOffset+float64(padT), math.Min(yOffset+float64(padT+chartH), y))
			if i == 0 {
				pts.WriteString(fmt.Sprintf("%.1f,%.1f", x, y))
			} else {
				pts.WriteString(fmt.Sprintf(" %.1f,%.1f", x, y))
			}
		}
		s.WriteString(fmt.Sprintf(`<polyline points="%s" fill="none" stroke="%s" stroke-width="2"/>`, pts.String(), line.color))
	}

	// Legend
	legendY := yOffset + float64(padT+chartH) + 18
	s.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%.0f" r="4" fill="#48c78e"/>`, padL, legendY-3))
	s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" font-size="10" fill="#363636">Savings</text>`, padL+8, legendY))
	s.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%.0f" r="4" fill="#3e8ed0"/>`, padL+70, legendY-3))
	s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" font-size="10" fill="#363636">Lending</text>`, padL+78, legendY))

	return s.String()
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

// buildCustomerChartSVG renders a standalone SVG customer count chart (for HTML dashboard sections).
func buildCustomerChartSVG(history []CustomerPoint) string {
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

	minVal := float64(history[0].Count)
	maxVal := float64(history[0].Count)
	for _, cp := range history {
		v := float64(cp.Count)
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	valRange := maxVal - minVal
	if valRange < 1 {
		valRange = 2
		minVal -= 1
		maxVal += 1
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
	totalH := padT + chartH + 10
	s.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg" style="width:100%%;height:auto">`, width, totalH))
	s.WriteString(`<style>text{font-family:Arial,Helvetica,sans-serif}</style>`)

	// Background
	s.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="#fafafa" stroke="#dbdbdb" stroke-width="1"/>`, padL, padT, chartW, chartH))

	// Y-axis labels
	for i := 0; i <= 4; i++ {
		val := minVal + valRange*float64(i)/4.0
		y := float64(padT+chartH) - float64(chartH)*float64(i)/4.0
		s.WriteString(fmt.Sprintf(`<line x1="%d" y1="%.0f" x2="%d" y2="%.0f" stroke="#ededed" stroke-width="1"/>`, padL, y, padL+chartW, y))
		s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" text-anchor="end" font-size="9" fill="#7a7a7a">%d</text>`, padL-5, y+3, int(val)))
	}

	// Line
	if len(history) == 1 {
		x := float64(padL) + float64(chartW)/2
		y := float64(padT+chartH) / 2
		s.WriteString(fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="3" fill="#00947e"/>`, x, y))
	} else {
		var pts strings.Builder
		for i, cp := range history {
			v := float64(cp.Count)
			x := float64(padL) + float64(chartW)*float64(i)/float64(len(history)-1)
			y := float64(padT+chartH) - float64(chartH)*(v-minVal)/valRange
			y = math.Max(float64(padT), math.Min(float64(padT+chartH), y))
			if i == 0 {
				pts.WriteString(fmt.Sprintf("%.1f,%.1f", x, y))
			} else {
				pts.WriteString(fmt.Sprintf(" %.1f,%.1f", x, y))
			}
		}
		s.WriteString(fmt.Sprintf(`<polyline points="%s" fill="none" stroke="#00947e" stroke-width="2"/>`, pts.String()))
	}

	s.WriteString(`</svg>`)
	return s.String()
}

// buildCustomerChart renders a single-line chart of customer count as an SVG <g>.
func buildCustomerChart(history []CustomerPoint, yOffset float64) string {
	if len(history) == 0 {
		return ""
	}
	const (
		padL   = 80
		padR   = 20
		chartW = 700 - padL - padR
		chartH = 140
		padT   = 10
	)

	minVal := float64(history[0].Count)
	maxVal := float64(history[0].Count)
	for _, cp := range history {
		v := float64(cp.Count)
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	valRange := maxVal - minVal
	if valRange < 1 {
		valRange = 2
		minVal -= 1
		maxVal += 1
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

	// Background
	s.WriteString(fmt.Sprintf(`<rect x="%d" y="%.0f" width="%d" height="%d" fill="#fafafa" stroke="#dbdbdb" stroke-width="1"/>`, padL, yOffset+float64(padT), chartW, chartH))

	// Y-axis labels
	for i := 0; i <= 4; i++ {
		val := minVal + valRange*float64(i)/4.0
		y := yOffset + float64(padT+chartH) - float64(chartH)*float64(i)/4.0
		s.WriteString(fmt.Sprintf(`<line x1="%d" y1="%.0f" x2="%d" y2="%.0f" stroke="#ededed" stroke-width="1"/>`, padL, y, padL+chartW, y))
		s.WriteString(fmt.Sprintf(`<text x="%d" y="%.0f" text-anchor="end" font-size="9" fill="#7a7a7a">%d</text>`, padL-5, y+3, int(val)))
	}

	// Line
	if len(history) == 1 {
		x := float64(padL) + float64(chartW)/2
		y := yOffset + float64(padT+chartH)/2
		s.WriteString(fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="3" fill="#00947e"/>`, x, y))
	} else {
		var pts strings.Builder
		for i, cp := range history {
			v := float64(cp.Count)
			x := float64(padL) + float64(chartW)*float64(i)/float64(len(history)-1)
			y := yOffset + float64(padT+chartH) - float64(chartH)*(v-minVal)/valRange
			y = math.Max(yOffset+float64(padT), math.Min(yOffset+float64(padT+chartH), y))
			if i == 0 {
				pts.WriteString(fmt.Sprintf("%.1f,%.1f", x, y))
			} else {
				pts.WriteString(fmt.Sprintf(" %.1f,%.1f", x, y))
			}
		}
		s.WriteString(fmt.Sprintf(`<polyline points="%s" fill="none" stroke="#00947e" stroke-width="2"/>`, pts.String()))
	}

	return s.String()
}
