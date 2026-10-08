package main

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.bytestone.uk/hum3/gobank/bff/staff"
	"git.bytestone.uk/hum3/gobank/cmd/demo/sim"
	"git.bytestone.uk/hum3/gobank/core"
)

// --- Dashboard section renderers ---

// renderDashDataDiv wraps shared dashboard content in a div with optional HTMX polling.
func renderDashDataDiv(d staff.DashData, polling bool) string {
	var s strings.Builder
	s.WriteString(`<div id="dash-data"`)
	if polling {
		s.WriteString(` hx-get="dashboard/update" hx-trigger="every 1s" hx-swap="outerHTML"`)
	}
	s.WriteString(`>`)
	s.WriteString(staff.BuildDashboardHTML(d))
	s.WriteString(`</div>`)
	return s.String()
}

func renderDashControls(d staff.DashData, oob bool, role staff.Role) string {
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
func renderDashAddCustomers(role staff.Role) string {
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

func renderDashboardFull(d staff.DashData, polling bool, role staff.Role) string {
	var s strings.Builder
	s.WriteString(renderDashDataDiv(d, polling))
	s.WriteString(renderDashControls(d, false, role))
	s.WriteString(renderDashAddCustomers(role))
	return s.String()
}

func renderDashboardUpdate(d staff.DashData, role staff.Role) string {
	var s strings.Builder
	s.WriteString(renderDashDataDiv(d, true))
	s.WriteString(renderDashControls(d, true, role))
	return s.String()
}

func renderPaymentsPage(bank core.StaffQueries, ds *DemoState, piiAuth bool, page int, role staff.Role) string {
	running := ds.IsPaymentsRunning()
	var s strings.Builder
	s.WriteString(staff.BuildPaymentsHTML(bank, piiAuth, page, running))

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

func simStatus(ds *DemoState) string {
	if ds.IsRunning() || ds.IsPaymentsRunning() || ds.IsAddingCustomers() {
		return "Running"
	}
	return "Stopped"
}

// newHandler is the demo as one http.Handler over its state: the BFF
// (ADR-0002 stage 6), which serves the customer app and web under /v1/
// and the staff web (bff/staff) everywhere else, with the console's own
// pages mounted on the staff web until story 1.6.4 moves them in. The
// server listens on it; the WASM build serves it in the tab from a
// service worker. scope is the path the handler is mounted under ("/" on
// the server, the service worker's scope in the tab).
func newHandler(state *DemoState, version, scope string) http.Handler {
	// The bank (ADR-0002 stage 5): the BFF and every staff page read and
	// write it through the core.
	bank := state.Bank

	site := staff.New(staff.Config{
		Bank: bank, Commands: bank, Scope: scope, Version: version,
		Status:     func() string { return simStatus(state) },
		Components: components, Debt: contractDebt,
	})
	consoleRoutes(site, state, version, scope)

	// The customer BFF under /v1/: the app's screens and the customer
	// web, which is the BFF's own HTML (story 1.6.2). The password is the
	// deployment's (GOBANK_APP_PASSWORD on a server, a fixed one in the
	// tab) and so is the session's keeping (a cookie on a server, the
	// tab's jar in the tab).
	password := appPassword()
	server := newAppBFF(bank, newAppLogin(bank, password), state.DB, scope, loginNote(), site, slog.Default())
	go func() {
		for range time.Tick(time.Minute) {
			server.Sessions().Sweep()
		}
	}()
	if password == "" {
		log.Printf("app BFF mounted at /v1/ with GOBANK_APP_PASSWORD unset: app login is off")
	}
	return customerSessions(server)
}

// consoleRoutes mounts the simulation console on the staff web: the
// dashboard and its controls, the payments generator, the settings, the
// runtime, export and import, the explorer and the status another program
// reads. Story 1.6.4 moves them into bff/staff over a Console interface.
func consoleRoutes(site *staff.Site, state *DemoState, version, scope string) {
	bank := state.Bank

	// --- Dashboard ---

	site.Handle("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		role := site.Role(w, r)
		d := dashboardData(bank, state)
		content := renderDashboardFull(d, d.Sim.Running, role)
		if site.Fragment(w, r, content) {
			return
		}
		// The dashboard polls its own sections, so the whole-page poll stays off.
		site.StaticPage(w, r, content)
	})

	site.Handle("/dashboard/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		role := site.Role(w, r)
		d := dashboardData(bank, state)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, renderDashboardUpdate(d, role))
	})

	// control is a POST that drives the simulation and returns to the dashboard.
	control := func(pattern, action string, do func(r *http.Request)) {
		site.Handle(pattern, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				site.Redirect(w, r, "")
				return
			}
			if !site.Require(w, r, action) {
				return
			}
			do(r)
			site.Redirect(w, r, "")
		})
	}
	control("/start", "sim_controls", func(*http.Request) { state.Start() })
	control("/stop", "sim_controls", func(*http.Request) { state.Stop() })
	control("/advance", "sim_controls", func(*http.Request) { state.AdvanceDay() })
	control("/reset", "sim_controls", func(*http.Request) { state.Reset() })
	control("/add-customers", "sim_controls", func(r *http.Request) {
		r.ParseForm()
		n, _ := strconv.Atoi(r.FormValue("n"))
		if n > 0 {
			state.AddCustomersBatch(n)
		}
	})

	// --- Export/Import ---

	site.Handle("/export.goluca", func(w http.ResponseWriter, r *http.Request) {
		if !site.Require(w, r, "export") {
			return
		}
		state.handleExport(w, r)
	})
	site.Handle("/import", func(w http.ResponseWriter, r *http.Request) {
		if !site.Require(w, r, "export") {
			return
		}
		state.handleImport(w, r, site.Redirect)
	})

	// --- Payments: the list with the generator's controls, and the generator ---

	site.Handle("/payments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		content := renderPaymentsPage(bank, state, site.PII(w, r), page, site.Role(w, r))
		if site.Fragment(w, r, content) {
			return
		}
		site.Page(w, r, content)
	})
	payments := func(pattern string, do func()) {
		site.Handle(pattern, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				site.Redirect(w, r, "payments")
				return
			}
			if !site.Require(w, r, "send_payment") {
				return
			}
			do()
			site.Redirect(w, r, "payments")
		})
	}
	payments("/payments/send", state.SendPayment)
	payments("/payments/run", state.StartPayments)
	payments("/payments/stop", state.StopPayments)

	// --- Settings ---

	site.Handle("/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			if !site.Require(w, r, "settings") {
				return
			}
			r.ParseForm()
			maxCust, _ := strconv.Atoi(r.FormValue("max_customers"))
			state.UpdateSettings(maxCust)
			if dayLength, err := sim.ParseDayLength(r.FormValue("day_length")); err == nil {
				state.SetDayLength(dayLength)
			}
			site.Redirect(w, r, "settings")
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		polling := simStatus(state) == "Running"
		content := buildSettingsHTML(bank, state.Settings(), polling, state.Restarts(10))
		if site.Fragment(w, r, content) {
			return
		}
		site.StaticPage(w, r, content)
	})

	// The settings page's status line, polled on its own so the form is
	// never re-rendered under the operator.
	site.Handle("/settings/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, renderSettingsStatus(bank, simStatus(state) == "Running"))
	})

	// --- Internal ---

	explorer := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if strings.TrimPrefix(r.URL.Path, "/internal/explorer") == "/" {
			site.Redirect(w, r, "internal/explorer")
			return
		}
		content := state.BuildExplorerPage(staff.WithRole(r.Context(), site.Role(w, r)), scope, r.URL.RequestURI())
		if site.Fragment(w, r, content) {
			return
		}
		site.Page(w, r, content)
	}
	site.Handle("/internal/explorer", explorer)
	site.Handle("/internal/explorer/", explorer)

	// --- Runtime, and the demo's state for another program: gobank-deploy's
	// upgrade drill reads the position and the restart record here ---

	site.Handle("/about/runtime", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := state.BuildRuntimeHTML()
		if site.Fragment(w, r, content) {
			return
		}
		site.Page(w, r, content)
	})
	site.Handle("/about.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(aboutStatus(bank, state))
	})
}
