//go:build !(js && wasm)

package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	gbp "codeberg.org/hum3/gobank-products"
	"github.com/drummonds/lofigui"
	"github.com/flosch/pongo2/v6"
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
		if d.Running {
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

func renderPaymentsPage(ds *DemoState, piiAuth bool, page int, role Role) {
	lofigui.HTML(ds.BuildPaymentsHTML(piiAuth, page))

	if role.Can("send_payment") {
		running := ds.IsPaymentsRunning()
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
	state := NewDemoState()

	app := lofigui.NewApp()
	app.Version = "Model Bank " + version
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

	// fullPage renders template with app state context (no Refresh header).
	fullPage := func(w http.ResponseWriter, r *http.Request, content string) {
		sessID := getSessionID(w, r)
		role := authStore.GetRole(sessID)
		ctrl.RenderTemplate(w, pongo2.Context{
			"request":         r,
			"version":         app.Version,
			"controller_name": ctrl.Name,
			"results":         content,
			"polling":         simStatus(state),
			"role":            string(role),
		})
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
		d := state.DashboardData()
		content := renderDashboardFull(d, d.Running, role)
		if serveHTMX(w, r, content) {
			return
		}
		// Dashboard self-manages polling via sections, so set "Stopped" to prevent #results polling
		ctrl.RenderTemplate(w, pongo2.Context{
			"request":         r,
			"version":         app.Version,
			"controller_name": ctrl.Name,
			"results":         content,
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
		d := state.DashboardData()
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
		content := renderAndCapture(func() { lofigui.HTML(state.BuildPnLHTML()) })
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
		content := renderAndCapture(func() { lofigui.HTML(state.BuildBalanceSheetHTML()) })
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
		content := renderAndCapture(func() { lofigui.HTML(state.BuildProductsHTML(gbp.FamilySavings)) })
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
		content := renderAndCapture(func() { lofigui.HTML(state.BuildProductsHTML(gbp.FamilyLending)) })
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
		content := renderAndCapture(func() { lofigui.HTML(state.BuildCustomersHTML(page)) })
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
			content := renderAndCapture(func() { lofigui.HTML(state.BuildCustomerAccountHTML(parts[0], idx, piiAuth, txPage)) })
			if serveHTMX(w, r, content) {
				return
			}
			fullPage(w, r, content)
			return
		}

		id := parts[0]
		content := renderAndCapture(func() { lofigui.HTML(state.BuildCustomerDetailHTML(id, piiAuth, txPage)) })
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
		content := renderAndCapture(func() { renderPaymentsPage(state, piiAuth, page, role) })
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
		content := renderAndCapture(func() { lofigui.HTML(state.BuildPaymentDetailHTML(id, piiAuth)) })
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
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := renderAndCapture(func() { lofigui.HTML(state.BuildSettingsHTML()) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
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

	http.HandleFunc("/reports/bbsi", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		sessID := getSessionID(w, r)
		piiAuth := authStore.EffectivePII(sessID)
		content := renderAndCapture(func() { lofigui.HTML(state.BuildBBSIHTML(piiAuth)) })
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
		content := renderAndCapture(func() { lofigui.HTML(state.BuildCustomerViewHTML(id, piiAuth)) })
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
		content := state.BuildCashPositionHTML()
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
		content := state.BuildCapitalHTML()
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
		content := state.BuildGiltsHTML()
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
		faceValue := 0.0
		fmt.Sscanf(r.FormValue("face_value"), "%f", &faceValue)
		if tenor != "" && faceValue >= 1000 {
			state.BuyGilt(tenor, faceValue)
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
		content := state.BuildExplorerHTML()
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
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		sort := r.URL.Query().Get("sort")
		dir := r.URL.Query().Get("dir")
		content := state.BuildExplorerTableHTML(name, page, sort, dir)
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

	http.HandleFunc("/favicon.ico", lofigui.ServeFavicon)

	// Try ports starting from 1347, auto-increment if in use
	for port := 1347; port < 1357; port++ {
		addr := fmt.Sprintf(":%d", port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			log.Printf("Port %d in use, trying next...", port)
			continue
		}
		log.Printf("Starting Model Bank Demo on http://localhost%s", addr)
		log.Fatal(http.Serve(ln, nil))
	}
	log.Fatal("Could not find an available port in range 1347-1356")
}
