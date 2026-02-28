//go:build !(js && wasm)

package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/drummonds/lofigui"
)

func (ds *DemoState) renderDashboard() {
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
  <form action="/deposit" method="post" style="display:inline"><button class="button is-primary" type="submit">Deposit £100</button></form>
  <form action="/withdraw" method="post" style="display:inline"><button class="button is-warning" type="submit">Withdraw £100</button></form>
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

func main() {
	state := NewDemoState()

	app := lofigui.NewApp()
	app.Version = "Model Bank Demo v0.2"
	app.SetRefreshTime(1)
	app.SetDisplayURL("/")

	ctrl, err := lofigui.NewController(lofigui.ControllerConfig{
		TemplateString: LayoutModelBank,
		Name:           "Model Bank",
	})
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}
	app.SetController(ctrl)

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
		lofigui.Reset()
		state.renderDashboard()
		app.HandleDisplay(w, r)
	})

	http.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		state.Start()
		app.StartAction()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		state.Stop()
		app.EndAction()
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

	http.HandleFunc("/deposit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		state.Deposit()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/withdraw", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		state.Withdraw()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		state.Reset()
		app.EndAction()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	// --- Accounting ---

	http.HandleFunc("/accounting/pnl", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		lofigui.Reset()
		lofigui.HTML(state.BuildPnLHTML())
		app.HandleDisplay(w, r)
	})

	http.HandleFunc("/accounting/balance-sheet", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		lofigui.Reset()
		lofigui.HTML(state.BuildBalanceSheetHTML())
		app.HandleDisplay(w, r)
	})

	// --- Products ---

	http.HandleFunc("/products/savings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		lofigui.Reset()
		lofigui.HTML(state.BuildProductsHTML(FamilySavings))
		app.HandleDisplay(w, r)
	})

	http.HandleFunc("/products/lending", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		lofigui.Reset()
		lofigui.HTML(state.BuildProductsHTML(FamilyLending))
		app.HandleDisplay(w, r)
	})

	// --- Customers ---

	http.HandleFunc("/customers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		lofigui.Reset()
		lofigui.HTML(state.BuildCustomersHTML())
		app.HandleDisplay(w, r)
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
		lofigui.Reset()
		lofigui.HTML(state.BuildCustomerDetailHTML(id))
		app.HandleDisplay(w, r)
	})

	// --- Payments ---

	http.HandleFunc("/payments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		lofigui.Reset()
		renderPaymentsPage(state)
		app.HandleDisplay(w, r)
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
			app.StartAction()
			app.SetDisplayURL("/payments")
			http.Redirect(w, r, "/payments", http.StatusSeeOther)
			return
		case "stop":
			if r.Method != "POST" {
				http.Redirect(w, r, "/payments", http.StatusSeeOther)
				return
			}
			state.StopPayments()
			app.EndAction()
			app.SetDisplayURL("/")
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
		lofigui.Reset()
		lofigui.HTML(state.BuildPaymentDetailHTML(id))
		app.HandleDisplay(w, r)
	})

	// --- Reports ---

	http.HandleFunc("/reports/bbsi", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		lofigui.Reset()
		lofigui.HTML(state.BuildBBSIHTML())
		app.HandleDisplay(w, r)
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
		lofigui.Reset()
		lofigui.HTML(state.BuildCustomerViewHTML(id))
		app.HandleDisplay(w, r)
	})

	// --- About ---

	http.HandleFunc("/about/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		lofigui.Reset()
		lofigui.HTML(BuildModelsHTML())
		app.HandleDisplay(w, r)
	})

	http.HandleFunc("/favicon.ico", lofigui.ServeFavicon)

	addr := ":1347"
	log.Printf("Starting Model Bank Demo on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
