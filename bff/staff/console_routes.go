package staff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"git.bytestone.uk/hum3/gobank/internal/daylength"
)

// consoleRoutes mounts the simulation console: the dashboard and its
// controls, the payments generator, the settings, the runtime, export and
// import, the explorer and the status another program reads.
func (s *Site) consoleRoutes() {
	bank, console := s.cfg.Bank, s.cfg.Console
	mux := s.mux

	// --- Dashboard ---

	// The dashboard: the bank's position, the run's state and its controls;
	// it polls its own sections.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		role := s.role(r)
		d := DashboardData(bank, console)
		content := renderDashboardFull(d, d.Sim.Running, role)
		if s.fragment(w, r, content) {
			return
		}
		// The dashboard polls its own sections, so the whole-page poll stays off.
		s.staticPage(w, r, content)
	})

	// The dashboard's polled sections, as an HTML fragment.
	mux.HandleFunc("/dashboard/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		role := s.role(r)
		d := DashboardData(bank, console)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, renderDashboardUpdate(d, role))
	})

	// control is a POST that drives the simulation and returns to the dashboard.
	control := func(pattern, action string, do func(r *http.Request)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				s.redirect(w, r, "")
				return
			}
			if !s.require(w, r, action) {
				return
			}
			do(r)
			s.redirect(w, r, "")
		})
	}
	// Starts the run (POST): the simulation moves day after day at the
	// settings' day length until stopped.
	control("/start", "sim_controls", func(*http.Request) { console.Start() })
	// Stops the run after the day in progress (POST).
	control("/stop", "sim_controls", func(*http.Request) { console.Stop() })
	// Runs one business day while the run is stopped (POST).
	control("/advance", "sim_controls", func(*http.Request) { console.AdvanceDay() })
	// Empties the bank and starts again on its opening day (POST); the
	// settings are kept.
	control("/reset", "sim_controls", func(*http.Request) { console.Reset() })
	// Opens n customers in the background (POST n=), with the product mix
	// the simulation uses; /about.json shows the batch's rate.
	control("/add-customers", "sim_controls", func(r *http.Request) {
		r.ParseForm()
		n, _ := strconv.Atoi(r.FormValue("n"))
		if n > 0 {
			console.AddCustomersBatch(n)
		}
	})

	// --- Export/Import ---

	// The whole book as a .goluca file (GET): admin only.
	mux.HandleFunc("/export.goluca", func(w http.ResponseWriter, r *http.Request) {
		if !s.require(w, r, "export") {
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var buf bytes.Buffer
		if err := console.Export(&buf); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="gobank.goluca"`)
		w.Write(buf.Bytes())
	})
	// Replaces the book with an uploaded .goluca file (POST, multipart
	// file=): admin only.
	mux.HandleFunc("/import", func(w http.ResponseWriter, r *http.Request) {
		if !s.require(w, r, "export") {
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "No file uploaded", http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "Read error", http.StatusBadRequest)
			return
		}
		if err := console.Import(bytes.NewReader(data)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.redirect(w, r, "")
	})

	// --- Payments: the list with the generator's controls, and the generator ---

	// Payments: the list, newest first, one page (?page=) at a time, with
	// the generator's controls.
	mux.HandleFunc("/payments", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string {
			return renderPaymentsPage(bank, console.SimStatus().PaymentsRunning, s.piiAuthorised(r), pageParam(r, "page"), s.role(r))
		})
	})
	payments := func(pattern string, do func()) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				s.redirect(w, r, "payments")
				return
			}
			if !s.require(w, r, "send_payment") {
				return
			}
			do()
			s.redirect(w, r, "payments")
		})
	}
	// Sends one generated payment now (POST).
	payments("/payments/send", console.SendPayment)
	// Starts the payment generator (POST): the settings' payments a day.
	payments("/payments/run", console.StartPayments)
	// Stops the payment generator (POST).
	payments("/payments/stop", console.StopPayments)

	// --- Settings ---

	// The console settings: the page (GET), and the change (POST
	// max_customers, payments_per_day, day_length; 0s runs flat out, a day
	// length that does not parse leaves the current one). The day length set
	// here is the run's and outlives the process.
	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			if !s.require(w, r, "settings") {
				return
			}
			r.ParseForm()
			maxCust, _ := strconv.Atoi(r.FormValue("max_customers"))
			paymentsPerDay, _ := strconv.Atoi(r.FormValue("payments_per_day"))
			console.UpdateSettings(maxCust, paymentsPerDay)
			if d, err := daylength.Parse(r.FormValue("day_length")); err == nil {
				console.SetDayLength(d)
			}
			s.redirect(w, r, "settings")
			return
		}
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content := BuildSettingsHTML(bank, console.Settings(), console.SimStatus().Busy(), console.Restarts(10))
		if s.fragment(w, r, content) {
			return
		}
		s.staticPage(w, r, content)
	})

	// The settings page's status line, polled on its own so the form is
	// never re-rendered under the operator.
	mux.HandleFunc("/settings/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, RenderSettingsStatus(bank, console.SimStatus().Busy()))
	})

	// --- Internal ---

	explorer := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if strings.TrimPrefix(r.URL.Path, "/internal/explorer") == "/" {
			s.redirect(w, r, "internal/explorer")
			return
		}
		content := s.BuildExplorerPage(WithRole(r.Context(), s.role(r)), r.URL.RequestURI())
		if s.fragment(w, r, content) {
			return
		}
		s.page(w, r, func() string { return content })
	}
	// The DB explorer: every component's tables, read-only; a role sees only
	// the components it may (customers needs view_pii).
	mux.HandleFunc("/internal/explorer", explorer)
	// The DB explorer's tables and rows, under its prefix.
	mux.HandleFunc("/internal/explorer/", explorer)

	// --- Runtime, and the demo's state for another program: gobank-deploy's
	// upgrade drill reads the position and the restart record here ---

	// Runtime: the process, its memory against the limit, the schema
	// versions and the restart record.
	mux.HandleFunc("/about/runtime", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildRuntimeHTML(bank, console.Runtime(), s.cfg.Version) })
	})
	// The demo's state for another program (GET, no session): version,
	// schema, settings, the run and its rates, the position and the restart
	// record. gobank-deploy's upgrade drill reads it before and after each
	// redeploy; a performance run reads the rates.
	mux.HandleFunc("/about.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AboutStatusOf(bank, console, s.cfg.Version))
	})
}
