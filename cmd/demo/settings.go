package main

import (
	"fmt"
	"strings"
)

// Settings holds configurable parameters for the simulation.
type Settings struct {
	MaxCustomers int
	BoEBaseRate  float64 // annual rate as decimal, e.g. 0.0525 = 5.25%
}

func DefaultSettings() Settings {
	return Settings{
		MaxCustomers: 50,
		BoEBaseRate:  0.0525,
	}
}

// BuildSettingsHTML renders the settings form.
func (ds *DemoState) BuildSettingsHTML() string {
	ds.mu.Lock()
	settings := ds.settings
	customerCount := len(ds.customers)
	ds.mu.Unlock()

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Settings</h2>`)
	s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">Current customers: %d</p>`, customerCount))

	s.WriteString(`<form action="/settings" method="post">`)
	s.WriteString(`<div class="box">`)

	// Max Customers
	s.WriteString(`<div class="field">`)
	s.WriteString(`<label class="label">Max Customers</label>`)
	s.WriteString(`<div class="control">`)
	s.WriteString(fmt.Sprintf(`<input class="input" type="number" name="max_customers" value="%d" min="3" max="500">`, settings.MaxCustomers))
	s.WriteString(`</div>`)
	s.WriteString(`<p class="help">Maximum number of customers in the simulation (3-500)</p>`)
	s.WriteString(`</div>`)

	// BoE Base Rate
	s.WriteString(`<div class="field">`)
	s.WriteString(`<label class="label">BoE Base Rate (%%)</label>`)
	s.WriteString(`<div class="control">`)
	s.WriteString(fmt.Sprintf(`<input class="input" type="number" name="boe_rate" value="%.2f" min="0" max="25" step="0.25">`, settings.BoEBaseRate*100))
	s.WriteString(`</div>`)
	s.WriteString(`<p class="help">Bank of England base rate as a percentage</p>`)
	s.WriteString(`</div>`)

	s.WriteString(`<div class="field">`)
	s.WriteString(`<div class="control">`)
	s.WriteString(`<button class="button is-primary" type="submit">Save Settings</button>`)
	s.WriteString(`</div></div>`)
	s.WriteString(`</div></form>`)

	return s.String()
}

// UpdateSettings updates the simulation settings.
func (ds *DemoState) UpdateSettings(maxCust int, boeRate float64) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if maxCust >= 3 && maxCust <= 500 {
		ds.settings.MaxCustomers = maxCust
	}
	if boeRate >= 0 && boeRate <= 0.25 {
		ds.settings.BoEBaseRate = boeRate
	}
}
