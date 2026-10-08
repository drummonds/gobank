package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/cmd/demo/sim"
	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/lofigui"
)

var authStore = NewAuthStore(5 * time.Minute)

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

// --- Dashboard section renderers ---

// renderDashDataDiv wraps shared dashboard content in a div with optional HTMX polling.
func renderDashDataDiv(d DashData, polling bool) string {
	var s strings.Builder
	s.WriteString(`<div id="dash-data"`)
	if polling {
		s.WriteString(` hx-get="dashboard/update" hx-trigger="every 1s" hx-swap="outerHTML"`)
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

func renderPaymentsPage(bank core.StaffQueries, ds *DemoState, piiAuth bool, page int, role Role) string {
	running := ds.IsPaymentsRunning()
	var s strings.Builder
	s.WriteString(buildPaymentsHTML(bank, piiAuth, page, running))

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

// newHandler is the demo as one http.Handler over its state (ADR-0002
// stage 6, story 1.6.1): the staff pages, the customer BFF under /v1/ and
// the phone frame. The server listens on it; the WASM build serves it in
// the tab from a service worker. scope is the path the handler is mounted
// under ("/" on the server, the service worker's scope in the tab): every
// page carries it as its <base>, links and form actions are relative to
// it, and every redirect is prefixed with it, so the pages never escape
// the scope they were served from.
func newHandler(state *DemoState, version, scope string) http.Handler {
	mux := http.NewServeMux()

	// redirect sends the browser to a scope-relative path. A target taken
	// from a form is only honoured when it is such a path: anything with a
	// leading slash or a scheme lands on the dashboard.
	redirect := func(w http.ResponseWriter, r *http.Request, target string) {
		if !inScope(target) {
			target = ""
		}
		http.Redirect(w, r, scope+target, http.StatusSeeOther)
	}

	// The bank (ADR-0002 stage 5): the BFF and every staff page read and
	// write it through the core.
	appPassword := os.Getenv("GOBANK_APP_PASSWORD")
	bank := state.Bank

	// The customer BFF, on this port under /v1/.
	appBFF := newAppBFF(bank, newAppLogin(bank, appPassword), state.DB(), slog.Default())
	mux.Handle("/v1/", appBFF)
	go func() {
		for range time.Tick(time.Minute) {
			appBFF.Sessions().Sweep()
		}
	}()
	if appPassword == "" {
		log.Printf("app BFF mounted at /v1/ with GOBANK_APP_PASSWORD unset: app login is off")
	}

	ctrl, err := lofigui.NewController(lofigui.ControllerConfig{
		TemplateString: LayoutModelBank,
		Name:           "Model Bank",
	})
	if err != nil {
		panic(err)
	}

	// Bank app controller (phone frame layout)
	appCtrl, err := lofigui.NewController(lofigui.ControllerConfig{
		TemplateString: LayoutBankApp,
		Name:           "Bank App",
	})
	if err != nil {
		panic(err)
	}

	// Register bank app routes
	registerBankAppAPI(mux, bank)
	registerBankAppRoutes(mux, bank, appCtrl, scope, redirect)

	// renderPage renders the layout around content; polling "Running" makes
	// the layout re-fetch the whole page every second.
	renderPage := func(w http.ResponseWriter, r *http.Request, content, polling string) {
		sessID := getSessionID(w, r)
		role := authStore.GetRole(sessID)
		ctrl.RenderTemplate(w, lofigui.TemplateContext{
			"request":         r,
			"version":         "Model Bank " + version,
			"controller_name": ctrl.Name,
			"results":         template.HTML(content),
			"polling":         polling,
			"role":            string(role),
			"scope":           scope,
			"path":            strings.TrimPrefix(r.URL.Path, "/"),
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

	mux.HandleFunc("/role", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			redirect(w, r, "")
			return
		}
		r.ParseForm()
		roleStr := r.FormValue("role")
		if !ValidRole(roleStr) {
			redirect(w, r, "")
			return
		}
		sessID := getSessionID(w, r)
		authStore.SetRole(sessID, Role(roleStr))
		target := r.FormValue("redirect")
		redirect(w, r, target)
	})

	// --- Dashboard ---

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
			"version":         "Model Bank " + version,
			"controller_name": ctrl.Name,
			"results":         template.HTML(content),
			"polling":         "Stopped",
			"role":            string(role),
			"scope":           scope,
			"path":            "",
		})
	})

	mux.HandleFunc("/dashboard/update", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			redirect(w, r, "")
			return
		}
		if !requireRole(w, r, "sim_controls") {
			return
		}
		state.Start()
		redirect(w, r, "")
	})

	mux.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			redirect(w, r, "")
			return
		}
		if !requireRole(w, r, "sim_controls") {
			return
		}
		state.Stop()
		redirect(w, r, "")
	})

	mux.HandleFunc("/advance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			redirect(w, r, "")
			return
		}
		if !requireRole(w, r, "sim_controls") {
			return
		}
		state.AdvanceDay()
		redirect(w, r, "")
	})

	mux.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			redirect(w, r, "")
			return
		}
		if !requireRole(w, r, "sim_controls") {
			return
		}
		state.Reset()
		redirect(w, r, "")
	})

	mux.HandleFunc("/add-customers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			redirect(w, r, "")
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
		redirect(w, r, "")
	})

	// --- Export/Import ---

	mux.HandleFunc("/export.goluca", func(w http.ResponseWriter, r *http.Request) {
		if !requireRole(w, r, "export") {
			return
		}
		state.handleExport(w, r)
	})
	mux.HandleFunc("/import", func(w http.ResponseWriter, r *http.Request) {
		if !requireRole(w, r, "export") {
			return
		}
		state.handleImport(w, r, redirect)
	})

	// --- Accounting ---

	mux.HandleFunc("/accounting/pnl", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := buildPnLHTML(bank)
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	mux.HandleFunc("/accounting/balance-sheet", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := buildBalanceSheetHTML(bank)
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// --- Products ---

	mux.HandleFunc("/products/savings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := buildProductsHTML(bank, gbp.FamilySavings)
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	mux.HandleFunc("/products/lending", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := buildProductsHTML(bank, gbp.FamilyLending)
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// --- Customers ---

	mux.HandleFunc("/customers", func(w http.ResponseWriter, r *http.Request) {
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
		content := buildCustomersHTML(bank, page, piiAuth)
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	mux.HandleFunc("/customers/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, "/customers/")
		if rest == "" {
			redirect(w, r, "customers")
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
			content := buildCustomerAccountHTML(bank, parts[0], idx, piiAuth, txPage)
			if serveHTMX(w, r, content) {
				return
			}
			fullPage(w, r, content)
			return
		}

		id := parts[0]
		content := buildCustomerDetailHTML(bank, id, piiAuth, txPage)
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// --- Payments ---

	mux.HandleFunc("/payments", func(w http.ResponseWriter, r *http.Request) {
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
		content := renderPaymentsPage(bank, state, piiAuth, page, role)
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	mux.HandleFunc("/payments/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/payments/")
		// Handle POST routes
		switch path {
		case "send":
			if r.Method != "POST" {
				redirect(w, r, "payments")
				return
			}
			if !requireRole(w, r, "send_payment") {
				return
			}
			state.SendPayment()
			redirect(w, r, "payments")
			return
		case "run":
			if r.Method != "POST" {
				redirect(w, r, "payments")
				return
			}
			if !requireRole(w, r, "send_payment") {
				return
			}
			state.StartPayments()
			redirect(w, r, "payments")
			return
		case "stop":
			if r.Method != "POST" {
				redirect(w, r, "payments")
				return
			}
			if !requireRole(w, r, "send_payment") {
				return
			}
			state.StopPayments()
			redirect(w, r, "payments")
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
		content := buildPaymentDetailHTML(bank, id, piiAuth)
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// --- Settings ---

	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			if !requireRole(w, r, "settings") {
				return
			}
			r.ParseForm()
			maxCust, _ := strconv.Atoi(r.FormValue("max_customers"))
			state.UpdateSettings(maxCust)
			if dayLength, err := sim.ParseDayLength(r.FormValue("day_length")); err == nil {
				state.SetDayLength(dayLength)
			}
			redirect(w, r, "settings")
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		polling := simStatus(state) == "Running"
		content := buildSettingsHTML(bank, state.Settings(), polling, state.Restarts(10))
		if serveHTMX(w, r, content) {
			return
		}
		staticPage(w, r, content)
	})

	// The settings page's status line, polled on its own so the form is
	// never re-rendered under the operator.
	mux.HandleFunc("/settings/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, renderSettingsStatus(bank, simStatus(state) == "Running"))
	})

	// --- Auth ---

	mux.HandleFunc("/auth/authorize", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			redirect(w, r, "")
			return
		}
		sessID := getSessionID(w, r)
		authStore.Authorize(sessID)
		target := r.FormValue("redirect")
		redirect(w, r, target)
	})

	mux.HandleFunc("/auth/revoke", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			redirect(w, r, "")
			return
		}
		sessID := getSessionID(w, r)
		authStore.Revoke(sessID)
		target := r.FormValue("redirect")
		redirect(w, r, target)
	})

	// --- Reports ---

	mux.HandleFunc("/reports/charts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := buildChartsHTML(bank)
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	mux.HandleFunc("/reports/bbsi", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		sessID := getSessionID(w, r)
		piiAuth := authStore.EffectivePII(sessID)
		content := buildBBSIHTML(bank, piiAuth)
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	mux.HandleFunc("/reports/customer-view", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			redirect(w, r, "customers")
			return
		}
		sessID := getSessionID(w, r)
		piiAuth := authStore.EffectivePII(sessID)
		content := buildCustomerViewHTML(bank, id, piiAuth)
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// --- Treasury ---

	mux.HandleFunc("/treasury/cash", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("/treasury/capital", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("/treasury/gilts", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("/treasury/gilts/buy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			redirect(w, r, "treasury/gilts")
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
		redirect(w, r, "treasury/gilts")
	})

	// --- Internal ---

	mux.HandleFunc("/internal/explorer", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/explorer" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := state.BuildExplorerPage(withRole(r.Context(), authStore.GetRole(getSessionID(w, r))), scope, r.URL.RequestURI())
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	mux.HandleFunc("/internal/explorer/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/internal/explorer/")
		if name == "" {
			redirect(w, r, "internal/explorer")
			return
		}
		content := state.BuildExplorerPage(withRole(r.Context(), authStore.GetRole(getSessionID(w, r))), scope, r.URL.RequestURI())
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// --- About ---

	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/about" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := BuildProjectAboutHTML()
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// The demo's state for another program: gobank-deploy's upgrade drill
	// reads the position and the restart record here.
	mux.HandleFunc("/about.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(aboutStatus(bank, state))
	})

	mux.HandleFunc("/about/runtime", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := state.BuildRuntimeHTML()
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	mux.HandleFunc("/about/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := BuildModelsHTML()
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	mux.HandleFunc("/about/docs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := BuildDocsHTML()
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	mux.HandleFunc("/about/docs/adr/", func(w http.ResponseWriter, r *http.Request) {
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
		content := page
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	mux.HandleFunc("/favicon.ico", lofigui.ServeFavicon)
	mux.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write(faviconSVG)
	})

	return mux
}

// inScope says whether target is a path relative to the scope: not empty
// of meaning, not absolute, not another origin.
func inScope(target string) bool {
	return target == "" || (!strings.HasPrefix(target, "/") && !strings.Contains(target, ":") && !strings.HasPrefix(target, "\\"))
}
