//go:build !(js && wasm)

package main

import (
	"fmt"
	"log"
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
	if r.Header.Get("HX-Request") == "true" {
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

func renderDashboard(ds *DemoState) {
	lofigui.HTML(ds.buildSVG())

	running := ds.IsRunning()

	statusTag := `<span class="tag is-light">Stopped</span>`
	if running {
		statusTag = `<span class="tag is-success">Running</span>`
	}

	lofigui.HTML(fmt.Sprintf(`<div class="field is-grouped is-grouped-multiline mb-4">
  <div class="control">%s</div>
</div>`, statusTag))

	var startStopBtn string
	if running {
		startStopBtn = `<form action="/stop" method="post" style="display:inline"><button class="button is-danger" type="submit">Stop</button></form>`
	} else {
		startStopBtn = `<form action="/start" method="post" style="display:inline"><button class="button is-success" type="submit">Run</button></form>`
	}

	lofigui.HTML(fmt.Sprintf(`<div class="buttons">%s
  <form action="/advance" method="post" style="display:inline"><button class="button is-info" type="submit">Advance Day</button></form>
  <form action="/add-customers" method="post" style="display:inline">
    <div class="field has-addons"><div class="control"><input class="input is-small" type="number" name="n" value="100" min="1" max="1000000" style="width:7em"></div>
    <div class="control"><button class="button is-small is-primary" type="submit">Add Customers</button></div></div>
  </form>
  <form action="/reset" method="post" style="display:inline"><button class="button is-light" type="submit">Reset</button></form>
</div>`, startStopBtn))
}

func renderPaymentsPage(ds *DemoState) {
	lofigui.HTML(ds.BuildPaymentsHTML())

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
	if ds.IsRunning() || ds.IsPaymentsRunning() {
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
		content := renderAndCapture(func() { renderDashboard(state) })
		if serveHTMX(w, r, content) {
			return
		}
		fullPage(w, r, content)
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
			state.AddCustomers(n)
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
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
		content := renderAndCapture(func() { lofigui.HTML(state.BuildCustomersHTML()) })
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
		sessID := getSessionID(w, r)
		piiAuth := authStore.IsAuthorized(sessID)
		content := renderAndCapture(func() { lofigui.HTML(state.BuildCustomerDetailHTML(id, piiAuth)) })
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
		content := renderAndCapture(func() { renderPaymentsPage(state) })
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
		content := renderAndCapture(func() { lofigui.HTML(state.BuildPaymentDetailHTML(id)) })
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
		content := renderAndCapture(func() { lofigui.HTML(state.BuildAboutHTML()) })
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

	addr := ":1347"
	log.Printf("Starting Model Bank Demo on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
