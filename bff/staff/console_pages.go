package staff

import (
	"context"
	"fmt"
	"html"
	"math"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
)

// --- Dashboard: the data sections with the console's controls ---

// renderDashDataDiv wraps the dashboard's data sections in a div with optional HTMX polling.
func renderDashDataDiv(d DashData, polling bool) string {
	var s strings.Builder
	s.WriteString(`<div id="dash-data"`)
	if polling {
		s.WriteString(` hx-get="dashboard/update" hx-trigger="every 1s" hx-swap="outerHTML"`)
	}
	s.WriteString(`>`)
	s.WriteString(BuildDashboardHTML(d))
	s.WriteString(`</div>`)
	return s.String()
}

func renderDashControls(d DashData, oob bool, role Role) string {
	var s strings.Builder
	s.WriteString(`<div id="dash-controls"`)
	if oob {
		s.WriteString(` hx-swap-oob="outerHTML"`)
	}
	s.WriteString(`>`)

	if role.Can("sim_controls") {
		var startStopBtn string
		if d.Sim.Running {
			startStopBtn = `<form action="stop" method="post" style="display:inline"><button class="button is-danger" type="submit">Stop</button></form>`
		} else {
			startStopBtn = `<form action="start" method="post" style="display:inline"><button class="button is-success" type="submit">Run</button></form>`
		}

		s.WriteString(fmt.Sprintf(`<div class="buttons">%s
  <form action="advance" method="post" style="display:inline"><button class="button is-info" type="submit">Advance Day</button></form>
  <form action="reset" method="post" style="display:inline"><button class="button is-light" type="submit">Reset</button></form>`, startStopBtn))
		if role.Can("export") {
			s.WriteString(`
  <a href="export.goluca" class="button is-link is-light" download>Export .goluca</a>
  <form action="import" method="post" enctype="multipart/form-data" style="display:inline">
    <label class="button is-info is-light">Import .goluca<input type="file" name="file" accept=".goluca" onchange="this.form.submit()" style="display:none"></label>
  </form>`)
		}
		s.WriteString(`
</div>`)
	}

	s.WriteString(`</div>`)
	return s.String()
}

// renderDashAddCustomers is static (never OOB-swapped) so the input field isn't reset during polling.
func renderDashAddCustomers(role Role) string {
	if !role.Can("sim_controls") {
		return `<div id="dash-add-customers"></div>`
	}
	return `<div id="dash-add-customers" class="mb-4">
  <form action="add-customers" method="post" style="display:inline">
    <div class="field has-addons"><div class="control"><input class="input is-small" type="number" name="n" value="100" min="1" max="1000000" style="width:7em"></div>
    <div class="control"><button class="button is-small is-primary" type="submit">Add Customers</button></div></div>
  </form>
</div>`
}

func renderDashboardFull(d DashData, polling bool, role Role) string {
	var s strings.Builder
	s.WriteString(renderDashDataDiv(d, polling))
	s.WriteString(renderDashControls(d, false, role))
	s.WriteString(renderDashAddCustomers(role))
	return s.String()
}

func renderDashboardUpdate(d DashData, role Role) string {
	var s strings.Builder
	s.WriteString(renderDashDataDiv(d, true))
	s.WriteString(renderDashControls(d, true, role))
	return s.String()
}

// --- Payments: the list with the generator's controls ---

func renderPaymentsPage(q core.StaffQueries, running, piiAuth bool, page int, role Role) string {
	var s strings.Builder
	s.WriteString(BuildPaymentsHTML(q, piiAuth, page, running))

	if role.Can("send_payment") {
		var startStopBtn string
		if running {
			startStopBtn = `<form action="payments/stop" method="post" style="display:inline"><button class="button is-danger" type="submit">Stop Auto</button></form>`
		} else {
			startStopBtn = `<form action="payments/run" method="post" style="display:inline"><button class="button is-success" type="submit">Auto Send</button></form>`
		}

		fmt.Fprintf(&s, `<div class="buttons mt-4">
  <form action="payments/send" method="post" style="display:inline"><button class="button is-primary" type="submit">Send Payment</button></form>
  %s
</div>`, startStopBtn)
	}
	return s.String()
}

// --- Settings ---

// BuildSettingsHTML renders the settings page: a status line that polls
// while the simulation runs, then the form, which is static so a value
// being typed is never wiped before Save (as the dashboard's controls
// are). The bank's parameters come from the core, the console's settings
// from the simulation; the restart record follows the form.
func BuildSettingsHTML(q core.BookQueries, settings Settings, polling bool, restarts []Restart) string {
	pos, _ := q.Position(context.Background())

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Settings</h2>`)
	s.WriteString(RenderSettingsStatus(q, polling))

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

// RenderSettingsStatus is the settings page's status line: customers on
// the books and the simulated date. It is the only part of the page that
// polls, and only while the simulation runs.
func RenderSettingsStatus(q core.BookQueries, polling bool) string {
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

// renderRestarts is the settings page's restart record: each start
// against the stop before it, so an upgrade's downtime and what it
// carried across are read off the console.
func renderRestarts(restarts []Restart) string {
	var s strings.Builder
	s.WriteString(`<h3 class="title is-5 mt-5">Restarts</h3>`)
	s.WriteString(`<p class="help mb-2">Each process start against the stop before it. Downtime is from the previous stop (or its last write of the run, for a release that kept no record) to this start; day and customers are at that stop and at this start.</p>`)
	s.WriteString(`<table class="table is-narrow is-fullwidth"><thead><tr><th>Started</th><th>Version</th><th>Previous</th><th>Downtime</th><th>Day</th><th>Customers</th><th>Stopped</th></tr></thead><tbody>`)
	for i, r := range restarts {
		var previous *Restart
		if i+1 < len(restarts) {
			previous = &restarts[i+1]
		}
		s.WriteString(`<tr>`)
		s.WriteString(fmt.Sprintf(`<td>%s</td><td>%s</td>`, r.StartedAt.Format("2 Jan 15:04:05"), r.Version))
		switch {
		case r.PreviousVersion != "":
			s.WriteString(fmt.Sprintf(`<td>%s</td>`, r.PreviousVersion))
		case !r.PreviousStoppedAt.IsZero():
			s.WriteString(`<td class="has-text-grey">unrecorded</td>`)
		default:
			s.WriteString(`<td class="has-text-grey">—</td>`)
		}
		if d, ok := r.Downtime(); ok {
			s.WriteString(fmt.Sprintf(`<td>%s</td>`, d.Round(time.Second)))
		} else if r.PreviousVersion != "" {
			s.WriteString(`<td class="has-text-danger">unknown (unclean stop)</td>`)
		} else {
			s.WriteString(`<td class="has-text-grey">—</td>`)
		}
		s.WriteString(renderHandover(r, previous))
		if r.StoppedAt.IsZero() {
			s.WriteString(`<td class="has-text-grey">—</td>`)
		} else {
			s.WriteString(fmt.Sprintf(`<td>%s</td>`, r.StoppedAt.Format("2 Jan 15:04:05")))
		}
		s.WriteString(`</tr>`)
	}
	s.WriteString(`</tbody></table>`)
	return s.String()
}

// renderHandover is the day and customers cells: at the previous stop and
// at this start, red when they differ.
func renderHandover(r Restart, previous *Restart) string {
	if previous == nil || previous.StoppedAt.IsZero() || r.PreviousVersion != previous.Version {
		return fmt.Sprintf(`<td>%d</td><td>%s</td>`, r.DayCount, GroupThousands(strconv.Itoa(r.Customers)))
	}
	class := ""
	if !r.ResumedIntact(*previous) {
		class = ` class="has-text-danger"`
	}
	return fmt.Sprintf(`<td%s>%d → %d</td><td%s>%s → %s</td>`, class, previous.StopDayCount, r.DayCount,
		class, GroupThousands(strconv.Itoa(previous.StopCustomers)), GroupThousands(strconv.Itoa(r.Customers)))
}

// --- Runtime ---

// BuildRuntimeHTML renders runtime stats about the banking model: the
// process (rt) and the bank's position through the core.
func BuildRuntimeHTML(q core.StaffQueries, rt Runtime, version string) string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	ctx := context.Background()
	pos, _ := q.Position(ctx)
	currentDay := pos.Day.Format("2 Jan 2006")
	dayCount := pos.DayCount
	customerCount := pos.Customers
	products, _ := q.Products(ctx)
	productCount := len(products)
	boeRate := pos.BoERate * 100
	paymentCount := 0
	if pp, err := q.PaymentPage(ctx, 1); err == nil {
		paymentCount = pp.Total
	}

	var s strings.Builder

	s.WriteString(`<h2 class="title is-4">Runtime</h2>`)
	s.WriteString(`<p class="subtitle is-6 has-text-grey">Runtime stats for the Model Bank simulation</p>`)

	// Environment
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Environment</h3>`)
	s.WriteString(`<table class="table is-fullwidth">`)
	s.WriteString(fmt.Sprintf(`<tr><th>Version</th><td>%s</td></tr>`, version))
	s.WriteString(fmt.Sprintf(`<tr><th>Runtime</th><td>%s</td></tr>`, rt.Env))
	s.WriteString(fmt.Sprintf(`<tr><th>Go version</th><td>%s</td></tr>`, runtime.Version()))
	s.WriteString(fmt.Sprintf(`<tr><th>GOARCH</th><td>%s</td></tr>`, runtime.GOARCH))
	s.WriteString(fmt.Sprintf(`<tr><th>GOOS</th><td>%s</td></tr>`, runtime.GOOS))
	s.WriteString(`</table></div>`)

	// Memory
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Memory</h3>`)
	s.WriteString(`<p class="has-text-grey mb-3">Go runtime memory statistics from <code>runtime.MemStats</code>.</p>`)
	s.WriteString(`<table class="table is-fullwidth">`)
	s.WriteString(fmt.Sprintf(`<tr><th>Alloc</th><td>%s</td><td class="has-text-grey">Heap memory in use by live objects</td></tr>`, FormatBytes(m.Alloc)))
	s.WriteString(fmt.Sprintf(`<tr><th>TotalAlloc</th><td>%s</td><td class="has-text-grey">Cumulative bytes allocated (never decreases)</td></tr>`, FormatBytes(m.TotalAlloc)))
	s.WriteString(fmt.Sprintf(`<tr><th>Sys</th><td>%s</td><td class="has-text-grey">Total memory obtained from the OS</td></tr>`, FormatBytes(m.Sys)))
	s.WriteString(fmt.Sprintf(`<tr><th>HeapObjects</th><td>%d</td><td class="has-text-grey">Number of allocated heap objects</td></tr>`, m.HeapObjects))
	s.WriteString(fmt.Sprintf(`<tr><th>NumGC</th><td>%d</td><td class="has-text-grey">Completed garbage collection cycles</td></tr>`, m.NumGC))
	s.WriteString(fmt.Sprintf(`<tr><th>Goroutines</th><td>%d</td><td class="has-text-grey">Active goroutines</td></tr>`, runtime.NumGoroutine()))
	s.WriteString(fmt.Sprintf(`<tr><th>CPU cores</th><td>%d</td><td class="has-text-grey">Host cores (shared with the database); GOMAXPROCS %d</td></tr>`, runtime.NumCPU(), runtime.GOMAXPROCS(0)))

	// Memory limits
	gcLimit := debug.SetMemoryLimit(-1) // read without changing
	if gcLimit > 0 && gcLimit < math.MaxInt64 {
		s.WriteString(fmt.Sprintf(`<tr><th>GC memory limit</th><td>%s</td><td class="has-text-grey">Advisory limit from <code>debug.SetMemoryLimit</code></td></tr>`, FormatBytes(uint64(gcLimit))))
	}
	s.WriteString(fmt.Sprintf(`<tr><th>Auto-stop threshold</th><td>%s</td><td class="has-text-grey">Simulation pauses when heap exceeds this</td></tr>`, FormatBytes(rt.MemoryLimit)))
	if rt.MemoryExceeded {
		s.WriteString(`<tr><th>Status</th><td><span class="tag is-danger">Memory exceeded — simulation paused</span></td><td></td></tr>`)
	}

	s.WriteString(`</table></div>`)

	// Data store
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Data Store</h3>`)
	s.WriteString(`<table class="table is-fullwidth">`)
	s.WriteString(fmt.Sprintf(`<tr><th>Type</th><td>%s</td></tr>`, rt.DBBackend))
	for _, v := range rt.Schema {
		s.WriteString(fmt.Sprintf(`<tr><th>Schema: %s</th><td>%d</td></tr>`, html.EscapeString(v.Component), v.Version))
	}
	for _, r := range rt.DBConfig {
		s.WriteString(fmt.Sprintf(`<tr><th>%s</th><td>%s</td></tr>`, html.EscapeString(r[0]), html.EscapeString(r[1])))
	}
	dbStats := rt.DBStats
	s.WriteString(fmt.Sprintf(`<tr><th>PII records (encrypted)</th><td>%d</td></tr>`, customerCount))
	s.WriteString(fmt.Sprintf(`<tr><th>Max open connections</th><td>%d</td></tr>`, dbStats.MaxOpenConnections))
	s.WriteString(fmt.Sprintf(`<tr><th>Open connections</th><td>%d</td><td class="has-text-grey">In use: %d, Idle: %d</td></tr>`, dbStats.OpenConnections, dbStats.InUse, dbStats.Idle))
	s.WriteString(fmt.Sprintf(`<tr><th>Wait count</th><td>%d</td><td class="has-text-grey">Connections waited for due to pool limit</td></tr>`, dbStats.WaitCount))
	if dbStats.MaxIdleClosed > 0 || dbStats.MaxLifetimeClosed > 0 {
		s.WriteString(fmt.Sprintf(`<tr><th>Connections closed</th><td>idle: %d, lifetime: %d</td><td class="has-text-grey">Closed by pool maintenance</td></tr>`, dbStats.MaxIdleClosed, dbStats.MaxLifetimeClosed))
	}
	s.WriteString(`</table></div>`)

	// Simulation
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Simulation</h3>`)
	s.WriteString(`<table class="table is-fullwidth">`)
	s.WriteString(fmt.Sprintf(`<tr><th>Current day</th><td>%s</td></tr>`, currentDay))
	s.WriteString(fmt.Sprintf(`<tr><th>Days elapsed</th><td>%d</td></tr>`, dayCount))
	s.WriteString(progressRow(rt.Progress))
	s.WriteString(fmt.Sprintf(`<tr><th>Customers</th><td>%d</td></tr>`, customerCount))
	s.WriteString(fmt.Sprintf(`<tr><th>Products</th><td>%d</td></tr>`, productCount))
	s.WriteString(fmt.Sprintf(`<tr><th>Payments</th><td>%d</td></tr>`, paymentCount))
	s.WriteString(fmt.Sprintf(`<tr><th>BoE base rate</th><td>%.2f%%</td></tr>`, boeRate))
	s.WriteString(`</table></div>`)

	return s.String()
}

// FormatBytes renders a byte count as B, KiB, MiB or GiB.
func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMG"[exp])
}

// progressRow renders the day in progress as a row of the runtime page's
// Simulation table, or "" before the first day has run.
func progressRow(s DayProgress) string {
	switch {
	case s.Active:
		work := s.Phase
		if s.Total > 0 {
			work += fmt.Sprintf(": %s / %s (%d%%) at %s/s", groupInt(s.Done), groupInt(s.Total), s.Done*100/s.Total, groupInt(int(math.Round(s.Rate))))
		}
		return fmt.Sprintf(`<tr><th>Day in progress</th><td>%s — %s</td><td class="has-text-grey">%s elapsed</td></tr>`,
			s.Day.Format("2 Jan 2006"), work, s.Elapsed.Round(time.Second))
	case !s.LastDay.IsZero():
		return fmt.Sprintf(`<tr><th>Last day</th><td>%s: %s accounts in %s</td></tr>`,
			s.LastDay.Format("2 Jan 2006"), groupInt(s.LastAccounts), s.LastDuration.Round(time.Second))
	}
	return ""
}

func groupInt(n int) string { return GroupThousands(strconv.Itoa(n)) }
