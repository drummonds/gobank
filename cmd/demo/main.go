//go:build !(js && wasm)

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/lofigui"
)

var authStore = NewAuthStore(5 * time.Minute)

var renderMu sync.Mutex

// renderAndCapture runs fn (which calls lofigui output functions) under a lock,
// captures the buffer content, and returns it.
func renderAndCapture(fn func()) string {
	renderMu.Lock()
	defer renderMu.Unlock()
	lofigui.Reset()
	fn()
	return lofigui.Buffer()
}

// serveHTMX checks for HTMX request and serves HTML fragment if so.
// Returns true if served as fragment (caller should return).
func serveHTMX(w http.ResponseWriter, r *http.Request, content string) bool {
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, content)
		return true
	}
	return false
}

// getSessionID returns a session ID from a cookie, creating one if needed.
func getSessionID(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie("session_id")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}
	id := fmt.Sprintf("sess-%d", time.Now().UnixNano())
	http.SetCookie(w, &http.Cookie{
		Name:  "session_id",
		Value: id,
		Path:  "/",
	})
	return id
}

// --- Dashboard section renderers ---

// renderDashDataDiv wraps shared dashboard content in a div with optional HTMX polling.
func renderDashDataDiv(d DashData, polling bool) string {
	var s strings.Builder
	s.WriteString(`<div id="dash-data"`)
	if polling {
		s.WriteString(` hx-get="/dashboard/update" hx-trigger="every 1s" hx-swap="outerHTML"`)
	}
	s.WriteString(`>`)
	s.WriteString(renderDashContent(d))
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
			startStopBtn = `<form action="/stop" method="post" style="display:inline"><button class="button is-danger" type="submit">Stop</button></form>`
		} else {
			startStopBtn = `<form action="/start" method="post" style="display:inline"><button class="button is-success" type="submit">Run</button></form>`
		}

		s.WriteString(fmt.Sprintf(`<div class="buttons">%s
  <form action="/advance" method="post" style="display:inline"><button class="button is-info" type="submit">Advance Day</button></form>
  <form action="/reset" method="post" style="display:inline"><button class="button is-light" type="submit">Reset</button></form>`, startStopBtn))
		if role.Can("export") {
			s.WriteString(`
  <a href="/export.goluca" class="button is-link is-light" download>Export .goluca</a>
  <form action="/import" method="post" enctype="multipart/form-data" style="display:inline">
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
  <form action="/add-customers" method="post" style="display:inline">
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

func renderPaymentsPage(bank core.StaffQueries, ds *DemoState, piiAuth bool, page int, role Role) {
	running := ds.IsPaymentsRunning()
	lofigui.HTML(buildPaymentsHTML(bank, piiAuth, page, running))

	if role.Can("send_payment") {
		var startStopBtn string
		if running {
			startStopBtn = `<form action="/payments/stop" method="post" style="display:inline"><button class="button is-danger" type="submit">Stop Auto</button></form>`
		} else {
			startStopBtn = `<form action="/payments/run" method="post" style="display:inline"><button class="button is-success" type="submit">Auto Send</button></form>`
		}

		lofigui.HTML(fmt.Sprintf(`<div class="buttons mt-4">
  <form action="/payments/send" method="post" style="display:inline"><button class="button is-primary" type="submit">Send Payment</button></form>
  %s
</div>`, startStopBtn))
	}
}

func simStatus(ds *DemoState) string {
	if ds.IsRunning() || ds.IsPaymentsRunning() || ds.IsAddingCustomers() {
		return "Running"
	}
	return "Stopped"
}

func main() {
	// GOBANK_PG_DSN selects a real PostgreSQL backend (postgres://...);
	// unset means in-memory pglike, as before.
	state := NewDemoStateWithDSN(os.Getenv("GOBANK_PG_DSN"))
	// GOBANK_MEMORY_LIMIT sizes the auto-stop threshold to the box (the
	// deployer sets it); unset keeps the browser-sized default.
	limit, err := memoryLimitFromEnv()
	if err != nil {
		log.Fatalf("GOBANK_MEMORY_LIMIT: %v", err)
	}
	state.SetMemoryLimit(limit)
	// GOBANK_DAY_LENGTH slows the simulation to a wall-clock day length
	// (e.g. 2h); unset runs flat out. Also settable on the settings page.
	dayLength, err := dayLengthFromEnv()
	if err != nil {
		log.Fatalf("GOBANK_DAY_LENGTH: %v", err)
	}
	state.SetDayLength(dayLength)
	// A run that was going when the previous process stopped carries on.
	if state.ResumedRunning() {
		log.Printf("resume: the run was going; starting the loop")
		state.Start()
	}

	app := lofigui.NewApp()
	app.Version = "Model Bank " + version

	// The core as the demo implements it (ADR-0002 stage 1): the BFF and
	// every staff page read and write the bank through it.
	appPassword := os.Getenv("GOBANK_APP_PASSWORD")
	bank := newCoreAdapter(state, appPassword)

	// The customer BFF, on this port under /v1/.
	appBFF := newAppBFF(bank, state.DB(), slog.Default())
	http.Handle("/v1/", appBFF)
	go func() {
		for range time.Tick(time.Minute) {
			appBFF.Sessions().Sweep()
		}
	}()
	if appPassword == "" {
		log.Printf("app BFF mounted at /v1/ with GOBANK_APP_PASSWORD unset: app login is off")
	}
	app.SetDisplayURL("/")

	ctrl, err := lofigui.NewController(lofigui.ControllerConfig{
		TemplateString: LayoutModelBank,
		Name:           "Model Bank",
	})
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}
	app.SetController(ctrl)

	// Bank app controller (phone frame layout)
	appCtrl, err := lofigui.NewController(lofigui.ControllerConfig{
		TemplateString: LayoutBankApp,
		Name:           "Bank App",
	})
	if err != nil {
		log.Fatalf("Failed to create bank app controller: %v", err)
	}

	// Register bank app routes
	registerBankAppAPI(state)
	registerBankAppRoutes(state, appCtrl)

	// renderPage renders the layout around content; polling "Running" makes
	// the layout re-fetch the whole page every second.
	renderPage := func(w http.ResponseWriter, r *http.Request, content, polling string) {
		sessID := getSessionID(w, r)
		role := authStore.GetRole(sessID)
		ctrl.RenderTemplate(w, lofigui.TemplateContext{
			"request":         r,
			"version":         app.Version,
			"controller_name": ctrl.Name,
			"results":         template.HTML(content),
			"polling":         polling,
			"role":            string(role),
		})
	}
	// fullPage renders template with app state context (no Refresh header).
	fullPage := func(w http.ResponseWriter, r *http.Request, content string) {
		renderPage(w, r, content, simStatus(state))
	}
	// staticPage is a page that polls its own fragments, if anything: the
	// layout's whole-page poll stays off so forms on it keep what is typed.
	staticPage := func(w http.ResponseWriter, r *http.Request, content string) {
		renderPage(w, r, content, "Stopped")
	}

	// requireRole returns 403 if the session's role lacks the given permission.
	requireRole := func(w http.ResponseWriter, r *http.Request, action string) bool {
		sessID := getSessionID(w, r)
		role := authStore.GetRole(sessID)
		if !role.Can(action) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return false
		}
		return true
	}

	// --- Role ---

	http.HandleFunc("/role", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		r.ParseForm()
		roleStr := r.FormValue("role")
		if !ValidRole(roleStr) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		sessID := getSessionID(w, r)
		authStore.SetRole(sessID, Role(roleStr))
		redirect := r.FormValue("redirect")
		if redirect == "" {
			redirect = "/"
		}
		http.Redirect(w, r, redirect, http.StatusSeeOther)
	})

	// --- Dashboard ---

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		sessID := getSessionID(w, r)
		role := authStore.GetRole(sessID)
		d := dashboardData(bank, state)
		content := renderDashboardFull(d, d.Sim.Running, role)
		if serveHTMX(w, r, content) {
			return
		}
		// Dashboard self-manages polling via sections, so set "Stopped" to prevent #results polling
		ctrl.RenderTemplate(w, lofigui.TemplateContext{
			"request":         r,
			"version":         app.Version,
			"controller_name": ctrl.Name,
			"results":         template.HTML(content),
			"polling":         "Stopped",
			"role":            string(role),
		})
	})

	http.HandleFunc("/dashboard/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		sessID := getSessionID(w, r)
		role := authStore.GetRole(sessID)
		d := dashboardData(bank, state)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, renderDashboardUpdate(d, role))
	})

	http.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if !requireRole(w, r, "sim_controls") {
			return
		}
		state.Start()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if !requireRole(w, r, "sim_controls") {
			return
		}
		state.Stop()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/advance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if !requireRole(w, r, "sim_controls") {
			return
		}
		state.AdvanceDay()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if !requireRole(w, r, "sim_controls") {
			return
		}
		state.Reset()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/add-customers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if !requireRole(w, r, "sim_controls") {
			return
		}
		r.ParseForm()
		n, _ := strconv.Atoi(r.FormValue("n"))
		if n > 0 {
			state.AddCustomersBatch(n)
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	// --- Export/Import ---

	http.HandleFunc("/export.goluca", func(w http.ResponseWriter, r *http.Request) {
		if !requireRole(w, r, "export") {
			return
		}
		state.handleExport(w, r)
	})
	http.HandleFunc("/import", func(w http.ResponseWriter, r *http.Request) {
		if !requireRole(w, r, "export") {
			return
		}
		state.handleImport(w, r)
	})

	// --- Accounting ---

	http.HandleFunc("/accounting/pnl", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := renderAndCapture(func() { lofigui.HTML(buildPnLHTML(bank)) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/accounting/balance-sheet", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := renderAndCapture(func() { lofigui.HTML(buildBalanceSheetHTML(bank)) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// --- Products ---

	http.HandleFunc("/products/savings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := renderAndCapture(func() { lofigui.HTML(buildProductsHTML(bank, gbp.FamilySavings)) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/products/lending", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := renderAndCapture(func() { lofigui.HTML(buildProductsHTML(bank, gbp.FamilyLending)) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// --- Customers ---

	http.HandleFunc("/customers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		sessID := getSessionID(w, r)
		piiAuth := authStore.EffectivePII(sessID)
		content := renderAndCapture(func() { lofigui.HTML(buildCustomersHTML(bank, page, piiAuth)) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/customers/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, "/customers/")
		if rest == "" {
			http.Redirect(w, r, "/customers", http.StatusSeeOther)
			return
		}
		sessID := getSessionID(w, r)
		piiAuth := authStore.EffectivePII(sessID)
		txPage, _ := strconv.Atoi(r.URL.Query().Get("txpage"))

		// Parse /customers/{id}/account/{idx}
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) >= 3 && parts[1] == "account" {
			idx, err := strconv.Atoi(parts[2])
			if err != nil {
				http.NotFound(w, r)
				return
			}
			content := renderAndCapture(func() { lofigui.HTML(buildCustomerAccountHTML(bank, parts[0], idx, piiAuth, txPage)) })
			if serveHTMX(w, r, content) {
				return
			}
			fullPage(w, r, content)
			return
		}

		id := parts[0]
		content := renderAndCapture(func() { lofigui.HTML(buildCustomerDetailHTML(bank, id, piiAuth, txPage)) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// --- Payments ---

	http.HandleFunc("/payments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		sessID := getSessionID(w, r)
		piiAuth := authStore.EffectivePII(sessID)
		role := authStore.GetRole(sessID)
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		content := renderAndCapture(func() { renderPaymentsPage(bank, state, piiAuth, page, role) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/payments/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/payments/")
		// Handle POST routes
		switch path {
		case "send":
			if r.Method != "POST" {
				http.Redirect(w, r, "/payments", http.StatusSeeOther)
				return
			}
			if !requireRole(w, r, "send_payment") {
				return
			}
			state.SendPayment()
			http.Redirect(w, r, "/payments", http.StatusSeeOther)
			return
		case "run":
			if r.Method != "POST" {
				http.Redirect(w, r, "/payments", http.StatusSeeOther)
				return
			}
			if !requireRole(w, r, "send_payment") {
				return
			}
			state.StartPayments()
			http.Redirect(w, r, "/payments", http.StatusSeeOther)
			return
		case "stop":
			if r.Method != "POST" {
				http.Redirect(w, r, "/payments", http.StatusSeeOther)
				return
			}
			if !requireRole(w, r, "send_payment") {
				return
			}
			state.StopPayments()
			http.Redirect(w, r, "/payments", http.StatusSeeOther)
			return
		}
		// Payment detail
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id, err := strconv.Atoi(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		sessID := getSessionID(w, r)
		piiAuth := authStore.EffectivePII(sessID)
		content := renderAndCapture(func() { lofigui.HTML(buildPaymentDetailHTML(bank, id, piiAuth)) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// --- Settings ---

	http.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			if !requireRole(w, r, "settings") {
				return
			}
			r.ParseForm()
			maxCust, _ := strconv.Atoi(r.FormValue("max_customers"))
			state.UpdateSettings(maxCust)
			if dayLength, err := parseDayLength(r.FormValue("day_length")); err == nil {
				state.SetDayLength(dayLength)
			}
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		polling := simStatus(state) == "Running"
		content := renderAndCapture(func() { lofigui.HTML(buildSettingsHTML(bank, state.Settings(), polling, state.Restarts(10))) })
		if serveHTMX(w, r, content) {
			return
		}
		staticPage(w, r, content)
	})

	// The settings page's status line, polled on its own so the form is
	// never re-rendered under the operator.
	http.HandleFunc("/settings/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, renderSettingsStatus(bank, simStatus(state) == "Running"))
	})

	// --- Auth ---

	http.HandleFunc("/auth/authorize", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		sessID := getSessionID(w, r)
		authStore.Authorize(sessID)
		redirect := r.FormValue("redirect")
		if redirect == "" {
			redirect = "/"
		}
		http.Redirect(w, r, redirect, http.StatusSeeOther)
	})

	http.HandleFunc("/auth/revoke", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		sessID := getSessionID(w, r)
		authStore.Revoke(sessID)
		redirect := r.FormValue("redirect")
		if redirect == "" {
			redirect = "/"
		}
		http.Redirect(w, r, redirect, http.StatusSeeOther)
	})

	// --- Reports ---

	http.HandleFunc("/reports/charts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := renderAndCapture(func() { lofigui.HTML(buildChartsHTML(bank)) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/reports/bbsi", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		sessID := getSessionID(w, r)
		piiAuth := authStore.EffectivePII(sessID)
		content := renderAndCapture(func() { lofigui.HTML(buildBBSIHTML(bank, piiAuth)) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/reports/customer-view", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Redirect(w, r, "/customers", http.StatusSeeOther)
			return
		}
		sessID := getSessionID(w, r)
		piiAuth := authStore.EffectivePII(sessID)
		content := renderAndCapture(func() { lofigui.HTML(buildCustomerViewHTML(bank, id, piiAuth)) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// --- Treasury ---

	http.HandleFunc("/treasury/cash", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := buildCashPositionHTML(bank)
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/treasury/capital", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := buildCapitalHTML(bank)
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/treasury/gilts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := buildGiltsHTML(bank)
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/treasury/gilts/buy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/treasury/gilts", http.StatusSeeOther)
			return
		}
		if !requireRole(w, r, "buy_gilt") {
			return
		}
		r.ParseForm()
		tenor := r.FormValue("tenor")
		pounds := 0.0
		fmt.Sscanf(r.FormValue("face_value"), "%f", &pounds)
		faceValue := poundsToPence(pounds) // form input is pounds; storage is minor units
		if err := bank.BuyGilt(r.Context(), tenor, faceValue); err != nil {
			log.Printf("buy gilt %s %d: %v", tenor, faceValue, err)
		}
		http.Redirect(w, r, "/treasury/gilts", http.StatusSeeOther)
	})

	// --- Internal ---

	http.HandleFunc("/internal/explorer", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/explorer" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := state.BuildExplorerPage(withRole(r.Context(), authStore.GetRole(getSessionID(w, r))), r.URL.RequestURI())
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/internal/explorer/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/internal/explorer/")
		if name == "" {
			http.Redirect(w, r, "/internal/explorer", http.StatusSeeOther)
			return
		}
		content := state.BuildExplorerPage(withRole(r.Context(), authStore.GetRole(getSessionID(w, r))), r.URL.RequestURI())
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// --- About ---

	http.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/about" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := renderAndCapture(func() { lofigui.HTML(BuildProjectAboutHTML()) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// The demo's state for another program: gobank-deploy's upgrade drill
	// reads the position and the restart record here.
	http.HandleFunc("/about.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(aboutStatus(bank, state))
	})

	http.HandleFunc("/about/runtime", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := renderAndCapture(func() { lofigui.HTML(state.BuildRuntimeHTML()) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/about/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := renderAndCapture(func() { lofigui.HTML(BuildModelsHTML()) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/about/docs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := renderAndCapture(func() { lofigui.HTML(BuildDocsHTML()) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/about/docs/adr/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		slug := strings.TrimPrefix(r.URL.Path, "/about/docs/adr/")
		page, ok := BuildADRHTML(slug)
		if !ok {
			http.NotFound(w, r)
			return
		}
		content := renderAndCapture(func() { lofigui.HTML(page) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/favicon.ico", lofigui.ServeFavicon)
	http.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write(faviconSVG)
	})

	// GOBANK_ADDR pins the listen address (e.g. ":1347" under systemd on a
	// cloud host) instead of scanning for a free port.
	if addr := os.Getenv("GOBANK_ADDR"); addr != "" {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatalf("listen %s: %v", addr, err)
		}
		log.Printf("Starting Model Bank Demo on %s", addr)
		serve(ln, state)
		return
	}

	// Try ports starting from 1347, auto-increment if in use
	for port := 1347; port < 1357; port++ {
		addr := fmt.Sprintf(":%d", port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			log.Printf("Port %d in use, trying next...", port)
			continue
		}
		log.Printf("Starting Model Bank Demo on http://localhost%s", addr)
		serve(ln, state)
		return
	}
	log.Fatal("Could not find an available port in range 1347-1356")
}

// serve runs the HTTP server on ln until SIGTERM or SIGINT, then stops the
// run loop, waits for the day in progress to finish writing and drains the
// server. This is the stop step of an in-place upgrade (ADR-0003); the
// service manager's stop timeout bounds it.
func serve(ln net.Listener, state *DemoState) {
	srv := &http.Server{}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	stop()
	log.Printf("shutdown: signal received; finishing the day in progress")
	if err := state.Shutdown(context.Background()); err != nil {
		log.Printf("shutdown: run loop: %v", err)
	}
	drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(drain); err != nil {
		log.Printf("shutdown: server: %v", err)
	}
	state.RecordStop()
	log.Printf("shutdown: done")
}
