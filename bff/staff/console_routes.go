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
	control("/start", "sim_controls", func(*http.Request) { console.Start() })
	control("/stop", "sim_controls", func(*http.Request) { console.Stop() })
	control("/advance", "sim_controls", func(*http.Request) { console.AdvanceDay() })
	control("/reset", "sim_controls", func(*http.Request) { console.Reset() })
	control("/add-customers", "sim_controls", func(r *http.Request) {
		r.ParseForm()
		n, _ := strconv.Atoi(r.FormValue("n"))
		if n > 0 {
			console.AddCustomersBatch(n)
		}
	})

	// --- Export/Import ---

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
	payments("/payments/send", console.SendPayment)
	payments("/payments/run", console.StartPayments)
	payments("/payments/stop", console.StopPayments)

	// --- Settings ---

	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			if !s.require(w, r, "settings") {
				return
			}
			r.ParseForm()
			maxCust, _ := strconv.Atoi(r.FormValue("max_customers"))
			console.UpdateSettings(maxCust)
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
	mux.HandleFunc("/internal/explorer", explorer)
	mux.HandleFunc("/internal/explorer/", explorer)

	// --- Runtime, and the demo's state for another program: gobank-deploy's
	// upgrade drill reads the position and the restart record here ---

	mux.HandleFunc("/about/runtime", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildRuntimeHTML(bank, console.Runtime(), s.cfg.Version) })
	})
	mux.HandleFunc("/about.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AboutStatusOf(bank, console, s.cfg.Version))
	})
}
