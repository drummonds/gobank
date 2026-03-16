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

func renderDashSummary(d DashData, polling bool) string {
	var s strings.Builder
	s.WriteString(`<div id="dash-summary"`)
	if polling {
		s.WriteString(` hx-get="/dashboard/update" hx-trigger="every 1s" hx-swap="outerHTML"`)
	}
	s.WriteString(`>`)

	dateStr := d.Day.Format("2 Jan 2006")
	nimStr := fmt.Sprintf("%.0f bps", d.NIMBps)

	s.WriteString(`<nav class="level mb-4">`)
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Day</p><p class="title is-5">%d &mdash; %s</p></div></div>`, d.DayCount, dateStr))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Customers</p><p class="title is-5">%d</p></div></div>`, d.CustomerCount))
	s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">NIM</p><p class="title is-5">%s</p></div></div>`, nimStr))
	if d.AddingCust {
		s.WriteString(fmt.Sprintf(`<div class="level-item has-text-centered"><div><p class="heading">Adding</p><p class="title is-6">%d / %d</p></div></div>`, d.AddingProgress, d.AddingTarget))
	}
	s.WriteString(`</nav>`)
	s.WriteString(`</div>`)
	return s.String()
}

func renderDashBalances(d DashData, oob bool) string {
	var s strings.Builder
	s.WriteString(`<div id="dash-balances"`)
	if oob {
		s.WriteString(` hx-swap-oob="outerHTML"`)
	}
	s.WriteString(`>`)
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="box dash-box has-background-success-light"><p class="heading">Savings (deposits)</p><p class="title is-5">%s</p></div></div>`, fmtMoney(d.Savings)))
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="box dash-box has-background-info-light"><p class="heading">Lending (loans)</p><p class="title is-5">%s</p></div></div>`, fmtMoney(d.Lending)))
	reserveClass := "has-background-warning-light"
	if d.Cash < d.RequiredReserves {
		reserveClass = "has-background-danger-light"
	}
	s.WriteString(fmt.Sprintf(`<div class="column"><div class="box dash-box %s"><p class="heading">BoE Cash Reserve</p><p class="title is-5">%s</p><p class="subtitle is-7 mb-0">Required: %s (%.0f%%) | BoE: %.2f%%</p></div></div>`,
		reserveClass, fmtMoney(d.Cash), fmtMoney(d.RequiredReserves), d.CapitalReserveRatio*100, d.BoeRate*100))
	s.WriteString(`</div></div>`)
	return s.String()
}

func renderDashControls(d DashData, oob bool) string {
	var s strings.Builder
	s.WriteString(`<div id="dash-controls"`)
	if oob {
		s.WriteString(` hx-swap-oob="outerHTML"`)
	}
	s.WriteString(`>`)

	var startStopBtn string
	if d.Running {
		startStopBtn = `<form action="/stop" method="post" style="display:inline"><button class="button is-danger" type="submit">Stop</button></form>`
	} else {
		startStopBtn = `<form action="/start" method="post" style="display:inline"><button class="button is-success" type="submit">Run</button></form>`
	}

	s.WriteString(fmt.Sprintf(`<div class="buttons">%s
  <form action="/advance" method="post" style="display:inline"><button class="button is-info" type="submit">Advance Day</button></form>
  <form action="/reset" method="post" style="display:inline"><button class="button is-light" type="submit">Reset</button></form>
  <a href="/export.goluca" class="button is-link is-light" download>Export .goluca</a>
  <form action="/import" method="post" enctype="multipart/form-data" style="display:inline">
    <label class="button is-info is-light">Import .goluca<input type="file" name="file" accept=".goluca" onchange="this.form.submit()" style="display:none"></label>
  </form>
</div>`, startStopBtn))
	s.WriteString(`</div>`)
	return s.String()
}

// renderDashAddCustomers is static (never OOB-swapped) so the input field isn't reset during polling.
func renderDashAddCustomers() string {
	return `<div id="dash-add-customers" class="mb-4">
  <form action="/add-customers" method="post" style="display:inline">
    <div class="field has-addons"><div class="control"><input class="input is-small" type="number" name="n" value="100" min="1" max="1000000" style="width:7em"></div>
    <div class="control"><button class="button is-small is-primary" type="submit">Add Customers</button></div></div>
  </form>
</div>`
}

func renderDashNIMChart(d DashData, oob bool) string {
	var s strings.Builder
	s.WriteString(`<div id="dash-nim-chart"`)
	if oob {
		s.WriteString(` hx-swap-oob="outerHTML"`)
	}
	s.WriteString(`>`)
	s.WriteString(`<h3 class="title is-6 has-text-grey mt-4 mb-2">NIM History (bps)</h3>`)
	s.WriteString(buildNIMChart(d.NIMHistory))
	s.WriteString(`</div>`)
	return s.String()
}

func renderDashBalanceChart(d DashData, oob bool) string {
	var s strings.Builder
	s.WriteString(`<div id="dash-balance-chart"`)
	if oob {
		s.WriteString(` hx-swap-oob="outerHTML"`)
	}
	s.WriteString(`>`)
	s.WriteString(`<h3 class="title is-6 has-text-grey mt-4 mb-2">Balance History</h3>`)
	s.WriteString(buildBalanceChartSVG(d.BalanceHistory))
	s.WriteString(`</div>`)
	return s.String()
}

func renderDashCustomerChart(d DashData, oob bool) string {
	var s strings.Builder
	s.WriteString(`<div id="dash-customer-chart"`)
	if oob {
		s.WriteString(` hx-swap-oob="outerHTML"`)
	}
	s.WriteString(`>`)
	s.WriteString(`<h3 class="title is-6 has-text-grey mt-4 mb-2">Customer Count</h3>`)
	s.WriteString(buildCustomerChartSVG(d.CustomerHistory))
	s.WriteString(`</div>`)
	return s.String()
}

func renderDashboardFull(d DashData, polling bool) string {
	var s strings.Builder
	s.WriteString(renderDashSummary(d, polling))
	s.WriteString(renderDashBalances(d, false))
	s.WriteString(renderDashControls(d, false))
	s.WriteString(renderDashAddCustomers())
	s.WriteString(renderDashNIMChart(d, false))
	s.WriteString(renderDashBalanceChart(d, false))
	s.WriteString(renderDashCustomerChart(d, false))
	return s.String()
}

func renderDashboardUpdate(d DashData) string {
	var s strings.Builder
	// Primary target: summary (with polling attribute)
	s.WriteString(renderDashSummary(d, true))
	// OOB swaps for all other sections
	s.WriteString(renderDashBalances(d, true))
	s.WriteString(renderDashControls(d, true))
	s.WriteString(renderDashNIMChart(d, true))
	s.WriteString(renderDashBalanceChart(d, true))
	s.WriteString(renderDashCustomerChart(d, true))
	return s.String()
}

func renderPaymentsPage(ds *DemoState, piiAuth bool, page int) {
	lofigui.HTML(ds.BuildPaymentsHTML(piiAuth, page))

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
		ctrl.RenderTemplate(w, pongo2.Context{
			"request":         r,
			"version":         app.Version,
			"controller_name": ctrl.Name,
			"results":         content,
			"polling":         simStatus(state),
		})
	}

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
		d := state.DashboardData()
		content := renderDashboardFull(d, d.Running)
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
		})
	})

	http.HandleFunc("/dashboard/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		d := state.DashboardData()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, renderDashboardUpdate(d))
	})

	http.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
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
		state.Stop()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/advance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
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
		state.Reset()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/add-customers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
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

	http.HandleFunc("/export.goluca", state.handleExport)
	http.HandleFunc("/import", state.handleImport)

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
		content := renderAndCapture(func() { lofigui.HTML(state.BuildProductsHTML(FamilySavings)) })
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
		content := renderAndCapture(func() { lofigui.HTML(state.BuildProductsHTML(FamilyLending)) })
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
		id := strings.TrimPrefix(r.URL.Path, "/customers/")
		if id == "" {
			http.Redirect(w, r, "/customers", http.StatusSeeOther)
			return
		}
		txPage, _ := strconv.Atoi(r.URL.Query().Get("txpage"))
		sessID := getSessionID(w, r)
		piiAuth := authStore.IsAuthorized(sessID)
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
		piiAuth := authStore.IsAuthorized(sessID)
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		content := renderAndCapture(func() { renderPaymentsPage(state, piiAuth, page) })
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
			state.SendPayment()
			http.Redirect(w, r, "/payments", http.StatusSeeOther)
			return
		case "run":
			if r.Method != "POST" {
				http.Redirect(w, r, "/payments", http.StatusSeeOther)
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
		piiAuth := authStore.IsAuthorized(sessID)
		content := renderAndCapture(func() { lofigui.HTML(state.BuildPaymentDetailHTML(id, piiAuth)) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	// --- Settings ---

	http.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
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
		piiAuth := authStore.IsAuthorized(sessID)
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
		piiAuth := authStore.IsAuthorized(sessID)
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

	http.HandleFunc("/internal/tables", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/tables" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := state.BuildTablesHTML()
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
	})

	http.HandleFunc("/internal/tables/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/internal/tables/")
		if name == "" {
			http.Redirect(w, r, "/internal/tables", http.StatusSeeOther)
			return
		}
		content := state.BuildTableDetailHTML(name)
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
