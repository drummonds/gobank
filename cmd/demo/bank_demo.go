package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Account represents a bank account in the demo.
type Account struct {
	Name    string
	Balance float64
	Rate    float64 // annual interest rate as decimal (0.04 = 4%)
	Color   string
}

// BankDemo holds the demo bank state.
type BankDemo struct {
	mu            sync.Mutex
	running       bool
	cancel        context.CancelFunc
	accounts      []Account
	currentDay    time.Time
	dayCount      int
	totalInterest float64
}

func NewBankDemo() *BankDemo {
	return &BankDemo{
		accounts: []Account{
			{Name: "Current", Balance: 1000.00, Rate: 0.01, Color: "#3e8ed0"},
			{Name: "Savings", Balance: 5000.00, Rate: 0.04, Color: "#48c78e"},
			{Name: "Fixed Deposit", Balance: 10000.00, Rate: 0.055, Color: "#00d1b2"},
		},
		currentDay: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func (b *BankDemo) advanceDay() {
	for i := range b.accounts {
		dailyRate := b.accounts[i].Rate / 365.0
		interest := b.accounts[i].Balance * dailyRate
		b.accounts[i].Balance += interest
		b.totalInterest += interest
	}
	b.currentDay = b.currentDay.AddDate(0, 0, 1)
	b.dayCount++
}

// AdvanceDay processes one day of interest accrual.
func (b *BankDemo) AdvanceDay() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.advanceDay()
}

// Start begins auto-advancing days.
func (b *BankDemo) Start() {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel
	b.running = true
	b.mu.Unlock()

	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				b.mu.Lock()
				b.advanceDay()
				b.mu.Unlock()
			}
		}
	}()
}

// Stop halts auto-advance.
func (b *BankDemo) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.running {
		return
	}
	b.running = false
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
}

// IsRunning returns whether auto-advance is active.
func (b *BankDemo) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

// Deposit adds £100 to the current account.
func (b *BankDemo) Deposit() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.accounts[0].Balance += 100
}

// Withdraw removes £100 from the current account.
func (b *BankDemo) Withdraw() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.accounts[0].Balance >= 100 {
		b.accounts[0].Balance -= 100
	}
}

// Reset restores the demo to its initial state.
func (b *BankDemo) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running {
		b.running = false
		if b.cancel != nil {
			b.cancel()
			b.cancel = nil
		}
	}
	b.accounts = []Account{
		{Name: "Current", Balance: 1000.00, Rate: 0.01, Color: "#3e8ed0"},
		{Name: "Savings", Balance: 5000.00, Rate: 0.04, Color: "#48c78e"},
		{Name: "Fixed Deposit", Balance: 10000.00, Rate: 0.055, Color: "#00d1b2"},
	}
	b.currentDay = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	b.dayCount = 0
	b.totalInterest = 0
}

// buildSVG generates a bar chart of account balances.
func (b *BankDemo) buildSVG() string {
	b.mu.Lock()
	accounts := make([]Account, len(b.accounts))
	copy(accounts, b.accounts)
	day := b.currentDay
	dayCount := b.dayCount
	totalInterest := b.totalInterest
	b.mu.Unlock()

	var s strings.Builder

	const (
		width   = 700
		height  = 320
		barMaxW = 350.0
		barH    = 30
		barX    = 170
	)

	// Find max balance for scaling
	maxBal := 0.0
	totalBal := 0.0
	for _, a := range accounts {
		if a.Balance > maxBal {
			maxBal = a.Balance
		}
		totalBal += a.Balance
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
	s.WriteString(fmt.Sprintf(`<text x="680" y="27" text-anchor="end" font-size="14" fill="#fff">Day %d — %s</text>`, dayCount, dateStr))
	s.WriteString(fmt.Sprintf(`<text x="680" y="43" text-anchor="end" font-size="11" fill="rgba(255,255,255,0.8)">Interest earned: £%.2f</text>`, totalInterest))

	// Account bars
	yStart := 75.0
	rowH := 65.0
	for i, a := range accounts {
		y := yStart + float64(i)*rowH

		// Name
		s.WriteString(fmt.Sprintf(`<text x="20" y="%.0f" font-size="15" font-weight="bold" fill="#363636">%s</text>`, y+12, a.Name))
		// Rate
		s.WriteString(fmt.Sprintf(`<text x="20" y="%.0f" font-size="11" fill="#7a7a7a">%.1f%% APR</text>`, y+28, a.Rate*100))

		// Bar background
		s.WriteString(fmt.Sprintf(`<rect x="%d" y="%.0f" width="%.0f" height="%d" rx="4" fill="#f0f0f0" stroke="#dbdbdb" stroke-width="1"/>`, barX, y, barMaxW, barH))

		// Bar fill
		barW := barMaxW * a.Balance / maxBal
		if barW < 2 {
			barW = 2
		}
		if barW > barMaxW {
			barW = barMaxW
		}
		s.WriteString(fmt.Sprintf(`<rect x="%d" y="%.0f" width="%.1f" height="%d" rx="4" fill="%s" opacity="0.85"/>`, barX, y, barW, barH, a.Color))

		// Balance
		s.WriteString(fmt.Sprintf(`<text x="%.0f" y="%.0f" font-size="14" fill="#363636" font-weight="bold">£%.2f</text>`, float64(barX)+barMaxW+10, y+20, a.Balance))
	}

	// Divider
	divY := yStart + float64(len(accounts))*rowH
	s.WriteString(fmt.Sprintf(`<line x1="20" y1="%.0f" x2="680" y2="%.0f" stroke="#dbdbdb" stroke-width="1"/>`, divY, divY))

	// Total
	s.WriteString(fmt.Sprintf(`<text x="20" y="%.0f" font-size="18" font-weight="bold" fill="#363636">Total: £%.2f</text>`, divY+30, totalBal))

	s.WriteString(`</svg>`)
	return s.String()
}
