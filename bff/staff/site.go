// Package staff is the staff web of the Model Bank: the login (story
// 1.7.1), the pages a member of staff reads the bank through (accounting,
// products, customers, payments, treasury, reports, about and the
// documentation), the role the signed-in user holds and the PII
// authorisation that gate what they see, the layout every page is
// rendered in, and the simulation console when there is one (ADR-0002
// stage 6, stories 1.6.3 and 1.6.4).
//
// It reads the bank through core.StaffQueries, acts on it through
// core.Commands, checks logins through core.Users and drives the
// simulation through Console, and knows nothing else of it. The BFF mounts
// it beside the customer routes (bff.Config.Staff) and its sessions are
// kept in the BFF's store.
package staff

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	dbexplorer "git.bytestone.uk/hum3/go-dbexplorer"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/gobank/internal/session"
	"git.bytestone.uk/hum3/lofigui"
)

// Config configures a Site.
type Config struct {
	Bank     core.StaffQueries // what the pages read; nil serves only the pages that read nothing
	Commands core.Commands     // what the pages act on (buying a gilt); nil refuses
	Users    core.Users        // who may sign in; nil lets nobody in

	// Sessions is where the signed-in users are kept; nil keeps them in
	// memory for the process. SecureCookies marks the session cookie
	// Secure: on unless serving plain HTTP. LoginNote is a note under the
	// login form; the tab uses it to state its password.
	Sessions      session.Store
	SecureCookies bool
	LoginNote     string

	// Scope is the path the site is mounted under: "/" on a server, the
	// service worker's scope in the tab. Every page carries it as its
	// <base>, links and form actions are relative to it, and every
	// redirect is prefixed with it, so the pages never escape the scope
	// they were served from.
	Scope   string
	Version string // shown in the footer

	// Console is the simulation console; its pages are served when one is
	// given. The layout's status tag and whole-page poll follow it.
	Console Console

	// Components and Debt are the component registry the documentation
	// page renders.
	Components []Component
	Debt       []CrossRead

	PIITTL time.Duration // how long an admin's PII authorisation lasts; default 5m
}

// Site is the staff web as one http.Handler.
type Site struct {
	cfg      Config
	mux      *http.ServeMux
	handler  http.Handler // the mux behind the login
	sessions session.Store
	limiter  *session.Limiter
	pii      *piiStore
	ctrl     *lofigui.Controller
	catalog  dbexplorer.StaticCatalog
}

// New builds the site and its routes.
func New(cfg Config) *Site {
	if cfg.Scope == "" {
		cfg.Scope = "/"
	}
	if cfg.PIITTL == 0 {
		cfg.PIITTL = 5 * time.Minute
	}
	ctrl, err := lofigui.NewController(lofigui.ControllerConfig{TemplateString: layout, Name: "Model Bank"})
	if err != nil {
		panic(err)
	}
	sessions := cfg.Sessions
	if sessions == nil {
		sessions = session.NewMemory(8*time.Hour, 15*time.Minute, nil)
	}
	s := &Site{
		cfg: cfg, mux: http.NewServeMux(), sessions: sessions,
		limiter: session.NewLimiter(5, 15*time.Minute, time.Now),
		pii:     newPIIStore(cfg.PIITTL), ctrl: ctrl, catalog: explorerCatalog(cfg.Components),
	}
	s.loginRoutes()
	s.routes()
	if cfg.Console != nil {
		s.consoleRoutes()
	}
	s.handler = s.requireLogin(s.mux)
	return s
}

func (s *Site) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

// status is the console's status for the layout's tag and its whole-page
// poll: "Running" while anything is going, else "Stopped".
func (s *Site) status() string {
	if s.cfg.Console != nil && s.cfg.Console.SimStatus().Busy() {
		return "Running"
	}
	return "Stopped"
}

// --- what a page uses ---

// role is the role the signed-in user holds; read-only for a request
// that carries no session, which only the open paths see.
func (s *Site) role(r *http.Request) Role {
	if sess, ok := signedIn(r); ok {
		return sess.role
	}
	return RoleReadOnly
}

// piiAuthorised says whether the signed-in user may see personal data.
func (s *Site) piiAuthorised(r *http.Request) bool {
	sess, ok := signedIn(r)
	return ok && s.pii.effective(sess.key(), sess.role)
}

// require answers 403 and returns false unless the session's role may
// take the action.
func (s *Site) require(w http.ResponseWriter, r *http.Request, action string) bool {
	if !s.role(r).Can(action) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}
	return true
}

// redirect sends the browser to a scope-relative path. A target taken
// from a form is only honoured when it is such a path: anything with a
// leading slash or a scheme lands on the dashboard.
func (s *Site) redirect(w http.ResponseWriter, r *http.Request, target string) {
	if !inScope(target) {
		target = ""
	}
	http.Redirect(w, r, s.cfg.Scope+target, http.StatusSeeOther)
}

// inScope says whether target is a path relative to the scope: not empty
// of meaning, not absolute, not another origin.
func inScope(target string) bool {
	return target == "" || (!strings.HasPrefix(target, "/") && !strings.Contains(target, ":") && !strings.HasPrefix(target, "\\"))
}

// fragment serves content alone to an HTMX request and returns true; a
// page request gets false and renders the whole page.
func (s *Site) fragment(w http.ResponseWriter, r *http.Request, content string) bool {
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, content)
		return true
	}
	return false
}

// fullPage renders content in the layout; while the console runs the
// layout re-fetches the whole page every second.
func (s *Site) fullPage(w http.ResponseWriter, r *http.Request, content string) {
	s.render(w, r, content, s.status())
}

// staticPage renders content in the layout with the whole-page poll off,
// so forms on it keep what is typed; the page polls its own fragments if
// anything.
func (s *Site) staticPage(w http.ResponseWriter, r *http.Request, content string) {
	s.render(w, r, content, "Stopped")
}

func (s *Site) render(w http.ResponseWriter, r *http.Request, content, polling string) {
	sess, signedIn := signedIn(r)
	s.ctrl.RenderTemplate(w, lofigui.TemplateContext{
		"request":         r,
		"version":         "Model Bank " + s.cfg.Version,
		"controller_name": s.ctrl.Name,
		"results":         template.HTML(content),
		"polling":         polling,
		"signed_in":       signedIn,
		"login":           sess.user.Login,
		"role":            string(sess.role),
		"role_label":      sess.role.Label(),
		"scope":           s.cfg.Scope,
		"path":            strings.TrimPrefix(r.URL.Path, "/"),
	})
}

// page is a GET page that reads the bank: content is built and rendered,
// or served alone to HTMX.
func (s *Site) page(w http.ResponseWriter, r *http.Request, build func() string) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	content := build()
	if s.fragment(w, r, content) {
		return
	}
	s.fullPage(w, r, content)
}

// --- routes ---

func (s *Site) routes() {
	bank := s.cfg.Bank
	mux := s.mux

	// PII: an admin authorises themselves to see personal data for a while
	mux.HandleFunc("/auth/authorize", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			s.redirect(w, r, "")
			return
		}
		if sess, ok := signedIn(r); ok {
			s.pii.authorise(sess.key())
		}
		s.redirect(w, r, r.FormValue("redirect"))
	})
	mux.HandleFunc("/auth/revoke", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			s.redirect(w, r, "")
			return
		}
		if sess, ok := signedIn(r); ok {
			s.pii.revoke(sess.key())
		}
		s.redirect(w, r, r.FormValue("redirect"))
	})

	// Accounting
	mux.HandleFunc("/accounting/pnl", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildPnLHTML(bank) })
	})
	mux.HandleFunc("/accounting/balance-sheet", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildBalanceSheetHTML(bank) })
	})

	// Products
	mux.HandleFunc("/products/savings", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildProductsHTML(bank, gbp.FamilySavings) })
	})
	mux.HandleFunc("/products/lending", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildProductsHTML(bank, gbp.FamilyLending) })
	})

	// Customers
	mux.HandleFunc("/customers", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildCustomersHTML(bank, pageParam(r, "page"), s.piiAuthorised(r)) })
	})
	mux.HandleFunc("/customers/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, "/customers/")
		if rest == "" {
			s.redirect(w, r, "customers")
			return
		}
		pii := s.piiAuthorised(r)
		txPage := pageParam(r, "txpage")
		// /customers/{id}/account/{idx}
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) >= 3 && parts[1] == "account" {
			idx, err := strconv.Atoi(parts[2])
			if err != nil {
				http.NotFound(w, r)
				return
			}
			s.page(w, r, func() string { return BuildCustomerAccountHTML(bank, parts[0], idx, pii, txPage) })
			return
		}
		s.page(w, r, func() string { return BuildCustomerDetailHTML(bank, parts[0], pii, txPage) })
	})

	// Payments: the detail; the list carries the console's controls until 1.6.4
	mux.HandleFunc("/payments/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		s.page(w, r, func() string { return BuildPaymentDetailHTML(bank, id, s.piiAuthorised(r)) })
	})

	// Reports
	mux.HandleFunc("/reports/charts", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildChartsHTML(bank) })
	})
	mux.HandleFunc("/reports/bbsi", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildBBSIHTML(bank, s.piiAuthorised(r)) })
	})
	mux.HandleFunc("/reports/customer-view", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			s.redirect(w, r, "customers")
			return
		}
		s.page(w, r, func() string { return BuildCustomerViewHTML(bank, id, s.piiAuthorised(r)) })
	})

	// Treasury
	mux.HandleFunc("/treasury/cash", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildCashPositionHTML(bank) })
	})
	mux.HandleFunc("/treasury/capital", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildCapitalHTML(bank) })
	})
	mux.HandleFunc("/treasury/gilts", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildGiltsHTML(bank) })
	})
	mux.HandleFunc("/treasury/gilts/buy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			s.redirect(w, r, "treasury/gilts")
			return
		}
		if !s.require(w, r, "buy_gilt") {
			return
		}
		r.ParseForm()
		tenor := r.FormValue("tenor")
		pounds := 0.0
		fmt.Sscanf(r.FormValue("face_value"), "%f", &pounds)
		faceValue := poundsToPence(pounds) // form input is pounds; storage is minor units
		if s.cfg.Commands == nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if err := s.cfg.Commands.BuyGilt(r.Context(), tenor, faceValue); err != nil {
			log.Printf("buy gilt %s %d: %v", tenor, faceValue, err)
		}
		s.redirect(w, r, "treasury/gilts")
	})

	// About
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/about" {
			http.NotFound(w, r)
			return
		}
		s.page(w, r, BuildProjectAboutHTML)
	})
	mux.HandleFunc("/about/models", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, BuildModelsHTML)
	})
	mux.HandleFunc("/about/docs", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildDocsHTML(s.cfg.Components, s.cfg.Debt) })
	})
	mux.HandleFunc("/about/docs/adr/", func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimPrefix(r.URL.Path, "/about/docs/adr/")
		page, ok := BuildADRHTML(slug)
		if !ok {
			http.NotFound(w, r)
			return
		}
		s.page(w, r, func() string { return page })
	})

	mux.HandleFunc("/favicon.ico", lofigui.ServeFavicon)
	mux.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write(faviconSVG)
	})
}

// pageParam is a 1-based page number from the query, 1 when absent.
func pageParam(r *http.Request, name string) int {
	page, _ := strconv.Atoi(r.URL.Query().Get(name))
	if page < 1 {
		page = 1
	}
	return page
}
