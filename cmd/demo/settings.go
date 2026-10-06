package main

import (
	"context"
	"fmt"
	"strings"

	"git.bytestone.uk/hum3/gobank/cmd/demo/sim"
	"git.bytestone.uk/hum3/gobank/core"
)

// buildSettingsHTML renders the settings page: a status line that polls
// while the simulation runs, then the form, which is static so a value
// being typed is never wiped before Save (as the dashboard's controls
// are). The bank's parameters come from the core, the console's settings
// from the simulation; the restart record follows the form.
func buildSettingsHTML(q core.BookQueries, settings sim.Settings, polling bool, restarts []Restart) string {
	pos, _ := q.Position(context.Background())

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Settings</h2>`)
	s.WriteString(renderSettingsStatus(q, polling))

	s.WriteString(`<form action="settings" method="post">`)
	s.WriteString(`<div class="box">`)

	// Max Customers
	s.WriteString(`<div class="field">`)
	s.WriteString(`<label class="label">Max Customers</label>`)
	s.WriteString(`<div class="control">`)
	s.WriteString(fmt.Sprintf(`<input class="input" type="number" name="max_customers" value="%d" min="3" max="1000000">`, settings.MaxCustomers))
	s.WriteString(`</div>`)
	s.WriteString(`<p class="help">Maximum number of customers in the simulation (3-1,000,000)</p>`)
	s.WriteString(`</div>`)

	// Day length
	s.WriteString(`<div class="field">`)
	s.WriteString(`<label class="label">Day Length</label>`)
	s.WriteString(`<div class="control">`)
	s.WriteString(fmt.Sprintf(`<input class="input" type="text" name="day_length" value="%s">`, settings.DayLength))
	s.WriteString(`</div>`)
	s.WriteString(`<p class="help">Wall-clock length of a simulated day, e.g. 2h or 90m; 0 runs flat out. Takes effect from the next day</p>`)
	s.WriteString(`</div>`)

	// BoE Base Rate (read-only)
	s.WriteString(`<div class="field">`)
	s.WriteString(`<label class="label">BoE Base Rate</label>`)
	s.WriteString(`<div class="control">`)
	s.WriteString(fmt.Sprintf(`<span class="tag is-medium is-info">%.2f%%</span>`, pos.BoERate*100))
	s.WriteString(`</div>`)
	s.WriteString(`<p class="help">Driven by historical Bank of England data</p>`)
	s.WriteString(`</div>`)

	// Capital Reserve Ratio (read-only)
	s.WriteString(`<div class="field">`)
	s.WriteString(`<label class="label">Capital Reserve Ratio</label>`)
	s.WriteString(`<div class="control">`)
	s.WriteString(fmt.Sprintf(`<span class="tag is-medium is-warning">%.0f%%</span>`, pos.ReserveRatio*100))
	s.WriteString(`</div>`)
	s.WriteString(`<p class="help">Minimum fraction of deposits held as BoE reserves</p>`)
	s.WriteString(`</div>`)

	s.WriteString(`<div class="field">`)
	s.WriteString(`<div class="control">`)
	s.WriteString(`<button class="button is-primary" type="submit">Save Settings</button>`)
	s.WriteString(`</div></div>`)
	s.WriteString(`</div></form>`)
	s.WriteString(renderRestarts(restarts))

	return s.String()
}

// renderSettingsStatus is the settings page's status line: customers on
// the books and the simulated date. It is the only part of the page that
// polls, and only while the simulation runs.
func renderSettingsStatus(q core.BookQueries, polling bool) string {
	pos, _ := q.Position(context.Background())
	var s strings.Builder
	s.WriteString(`<div id="settings-status"`)
	if polling {
		s.WriteString(` hx-get="settings/status" hx-trigger="every 1s" hx-swap="outerHTML"`)
	}
	s.WriteString(fmt.Sprintf(`><p class="subtitle is-6 has-text-grey">Current customers: %d | Sim date: %s</p></div>`,
		pos.Customers, pos.Day.Format("2 Jan 2006")))
	return s.String()
}

// UpdateSettings sets the customer ceiling; out-of-range values are refused.
