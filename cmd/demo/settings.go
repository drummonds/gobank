package main

import (
	"fmt"
	"strings"
)

// Settings holds configurable parameters for the simulation.
type Settings struct {
	MaxCustomers        int
	BoEBaseRate         float64 // annual rate as decimal, e.g. 0.0525 = 5.25%
	CapitalReserveRatio float64 // fraction of deposits that must be held as reserves, e.g. 0.15 = 15%
}

func DefaultSettings() Settings {
	return Settings{
		MaxCustomers:        1_000_000,
		BoEBaseRate:         0.0525,
		CapitalReserveRatio: 0.15,
	}
}

// BuildSettingsHTML renders the settings form.
func (ds *DemoState) BuildSettingsHTML() string {
	ds.mu.Lock()
	settings := ds.settings
	customerCount := len(ds.customers)
	currentDay := ds.currentDay
	ds.mu.Unlock()

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Settings</h2>`)
	s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">Current customers: %d | Sim date: %s</p>`, customerCount, currentDay.Format("2 Jan 2006")))

	s.WriteString(`<form action="/settings" method="post">`)
	s.WriteString(`<div class="box">`)

	// Max Customers
	s.WriteString(`<div class="field">`)
	s.WriteString(`<label class="label">Max Customers</label>`)
	s.WriteString(`<div class="control">`)
	s.WriteString(fmt.Sprintf(`<input class="input" type="number" name="max_customers" value="%d" min="3" max="1000000">`, settings.MaxCustomers))
	s.WriteString(`</div>`)
	s.WriteString(`<p class="help">Maximum number of customers in the simulation (3-1,000,000)</p>`)
	s.WriteString(`</div>`)

	// BoE Base Rate (read-only)
	s.WriteString(`<div class="field">`)
	s.WriteString(`<label class="label">BoE Base Rate</label>`)
	s.WriteString(`<div class="control">`)
	s.WriteString(fmt.Sprintf(`<span class="tag is-medium is-info">%.2f%%</span>`, settings.BoEBaseRate*100))
	s.WriteString(`</div>`)
	s.WriteString(`<p class="help">Driven by historical Bank of England data</p>`)
	s.WriteString(`</div>`)

	// Capital Reserve Ratio (read-only)
	s.WriteString(`<div class="field">`)
	s.WriteString(`<label class="label">Capital Reserve Ratio</label>`)
	s.WriteString(`<div class="control">`)
	s.WriteString(fmt.Sprintf(`<span class="tag is-medium is-warning">%.0f%%</span>`, settings.CapitalReserveRatio*100))
	s.WriteString(`</div>`)
	s.WriteString(`<p class="help">Minimum fraction of deposits held as BoE reserves</p>`)
	s.WriteString(`</div>`)

	s.WriteString(`<div class="field">`)
	s.WriteString(`<div class="control">`)
	s.WriteString(`<button class="button is-primary" type="submit">Save Settings</button>`)
	s.WriteString(`</div></div>`)
	s.WriteString(`</div></form>`)

	return s.String()
}

// UpdateSettings updates the simulation settings.
func (ds *DemoState) UpdateSettings(maxCust int) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if maxCust >= 3 && maxCust <= 1_000_000 {
		ds.settings.MaxCustomers = maxCust
	}
}
