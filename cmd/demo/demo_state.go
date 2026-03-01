package main

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// RatePoint records a BoE base rate at a point in simulated time.
type RatePoint struct {
	Date time.Time
	Rate float64
}

// DemoState holds all unified state for the model bank demo.
type DemoState struct {
	mu            sync.Mutex
	running       bool
	cancel        context.CancelFunc
	products      []Product
	customers     []CustomerRecord
	payments      []Payment
	currentDay    time.Time
	dayCount      int
	nextPaymentID int
	payRunning    bool
	payCancel     context.CancelFunc
	opCostPerDay  float64
	rng           *rand.Rand
	piiStore      *PIIStore
	settings      Settings
	nextCustSeq   int
	piiAuthorized bool
	boeHistory    []RatePoint
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
	return ds
}

// --- Bank simulation ---

func (ds *DemoState) advanceDay() {
	// Interest accrual
	for ci := range ds.customers {
		for ai := range ds.customers[ci].Accounts {
			a := &ds.customers[ci].Accounts[ai]
			dailyRate := a.Rate / 365.0
			interest := a.Balance * dailyRate
			if a.Family == FamilySavings {
				a.Balance += interest
				a.Interest += interest
			} else {
				a.Balance += interest
				a.Interest += interest
			}
		}
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
			ds.nextCustSeq++
			_ = ds.piiStore.Store(cust.ID, name, ni)
			ds.customers = append(ds.customers, cust)
		}
	}
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
	ds.customers = initCustomers(ds.rng, ds.products, ds.currentDay, ds.piiStore)
}

// --- Dashboard SVG ---

func (ds *DemoState) buildSVG() string {
	ds.mu.Lock()
	day := ds.currentDay
	dayCount := ds.dayCount

	savingsTotal := 0.0
	lendingTotal := 0.0
	totalInterest := 0.0
	type topAccount struct {
		CustomerID string
		Product    string
		Balance    float64
		Family     ProductFamily
	}
	var tops []topAccount
	for _, c := range ds.customers {
		for _, a := range c.Accounts {
			if a.Family == FamilySavings {
				savingsTotal += a.Balance
			} else {
				lendingTotal += a.Balance
			}
			totalInterest += a.Interest
			tops = append(tops, topAccount{c.ID, a.ProductName, a.Balance, a.Family})
		}
	}
	customerCount := len(ds.customers)
	piiStore := ds.piiStore
	ds.mu.Unlock()

	// Sort tops by balance descending, take top 5
	for i := 0; i < len(tops); i++ {
		for j := i + 1; j < len(tops); j++ {
			if tops[j].Balance > tops[i].Balance {
				tops[i], tops[j] = tops[j], tops[i]
			}
		}
	}
	if len(tops) > 5 {
		tops = tops[:5]
	}

	var s strings.Builder
	const (
		width   = 700
		height  = 380
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
	s.WriteString(fmt.Sprintf(`<text x="680" y="35" text-anchor="end" font-size="11" fill="rgba(255,255,255,0.8)">Customers: %d | Interest: £%.2f</text>`, customerCount, totalInterest))

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
		s.WriteString(fmt.Sprintf(`<text x="%.0f" y="%.0f" font-size="14" fill="#363636" font-weight="bold">£%.2f</text>`, float64(barX)+barMaxW+10, y+20, f.Balance))
	}

	// Divider
	divY := yStart + 2*rowH
	s.WriteString(fmt.Sprintf(`<line x1="20" y1="%.0f" x2="680" y2="%.0f" stroke="#dbdbdb" stroke-width="1"/>`, divY, divY))

	// Top accounts
	s.WriteString(fmt.Sprintf(`<text x="20" y="%.0f" font-size="13" font-weight="bold" fill="#7a7a7a">Top Accounts</text>`, divY+22))
	for i, t := range tops {
		y := divY + 40 + float64(i)*22
		color := "#48c78e"
		if t.Family == FamilyLending {
			color = "#3e8ed0"
		}
		custName := piiStore.RetrieveName(t.CustomerID)
		s.WriteString(fmt.Sprintf(`<circle cx="28" cy="%.0f" r="4" fill="%s"/>`, y-4, color))
		s.WriteString(fmt.Sprintf(`<text x="40" y="%.0f" font-size="12" fill="#363636">%s — %s: £%.2f</text>`, y, custName, t.Product, t.Balance))
	}

	s.WriteString(`</svg>`)
	return s.String()
}
