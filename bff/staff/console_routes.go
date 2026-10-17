package staff

import (
	"fmt"
	"net/http"
	"strconv"

	"git.bytestone.uk/hum3/gobank/internal/daylength"
)

// simulationRoutes mounts Simulation: the dashboard and its controls, the
// payments generator and the settings, what gobank-deploy drives.
func (s *Site) simulationRoutes() {
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

	// --- The payments generator ---

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
}
