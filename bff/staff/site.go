// Package staff is the staff web of the Model Bank: the pages a member of
// staff reads the bank through (accounting, products, customers, payments,
// treasury, reports, about and the documentation), the role switch and the
// PII authorisation that gate what they see, and the layout every page is
// rendered in (ADR-0002 stage 6, story 1.6.3).
//
// It reads the bank through core.StaffQueries and acts on it through
// core.Commands, and knows nothing else of it. The BFF mounts it beside the
// customer routes (bff.Config.Staff). Until story 1.6.4 brings the console
// here, the console mounts its own pages on the site (Handle) and renders
// them in the layout (Page).
package staff

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/lofigui"
)

// Config configures a Site.
type Config struct {
	Bank     core.StaffQueries // what the pages read; nil serves only the pages that read nothing
	Commands core.Commands     // what the pages act on (buying a gilt); nil refuses

	// Scope is the path the site is mounted under: "/" on a server, the
	// service worker's scope in the tab. Every page carries it as its
	// <base>, links and form actions are relative to it, and every
	// redirect is prefixed with it, so the pages never escape the scope
	// they were served from.
	Scope   string
	Version string // shown in the footer

	// Status is the console's status, "Running" or "Stopped", for the
	// layout's tag and its whole-page poll; nil is "Stopped".
	Status func() string

	// Components and Debt are the component registry the documentation
	// page renders.
	Components []Component
	Debt       []CrossRead

	PIITTL time.Duration // how long an admin's PII authorisation lasts; default 5m
}

// Site is the staff web as one http.Handler.
type Site struct {
	cfg  Config
	mux  *http.ServeMux
	auth *AuthStore
	ctrl *lofigui.Controller
}

// New builds the site and its routes.
func New(cfg Config) *Site {
	if cfg.Scope == "" {
		cfg.Scope = "/"
	}
	if cfg.Status == nil {
		cfg.Status = func() string { return "Stopped" }
	}
	if cfg.PIITTL == 0 {
		cfg.PIITTL = 5 * time.Minute
	}
	ctrl, err := lofigui.NewController(lofigui.ControllerConfig{TemplateString: layout, Name: "Model Bank"})
	if err != nil {
		panic(err)
	}
	s := &Site{cfg: cfg, mux: http.NewServeMux(), auth: NewAuthStore(cfg.PIITTL), ctrl: ctrl}
	s.routes()
	return s
}

func (s *Site) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// Handle mounts a page of the console's on the site, until story 1.6.4
// brings the console here. pattern is a net/http pattern.
func (s *Site) Handle(pattern string, h http.HandlerFunc) { s.mux.HandleFunc(pattern, h) }

// --- services a page uses ---

// Session is the staff session the request belongs to.
func (s *Site) Session(w http.ResponseWriter, r *http.Request) string { return sessionID(w, r) }

// Role is the role the request's session has chosen.
func (s *Site) Role(w http.ResponseWriter, r *http.Request) Role {
	return s.auth.GetRole(sessionID(w, r))
}

// PII says whether the request's session may see personal data.
func (s *Site) PII(w http.ResponseWriter, r *http.Request) bool {
	return s.auth.EffectivePII(sessionID(w, r))
}

// Require answers 403 and returns false unless the session's role may
// take the action.
func (s *Site) Require(w http.ResponseWriter, r *http.Request, action string) bool {
	if !s.Role(w, r).Can(action) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}
	return true
}

// Redirect sends the browser to a scope-relative path. A target taken
// from a form is only honoured when it is such a path: anything with a
// leading slash or a scheme lands on the dashboard.
func (s *Site) Redirect(w http.ResponseWriter, r *http.Request, target string) {
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

// Fragment serves content alone to an HTMX request and returns true; a
// page request gets false and renders the whole page.
func (s *Site) Fragment(w http.ResponseWriter, r *http.Request, content string) bool {
	if r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Boosted") != "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, content)
		return true
	}
	return false
}

// Page renders content in the layout; while the console runs the layout
// re-fetches the whole page every second.
func (s *Site) Page(w http.ResponseWriter, r *http.Request, content string) {
	s.render(w, r, content, s.cfg.Status())
}

// StaticPage renders content in the layout with the whole-page poll off,
// so forms on it keep what is typed; the page polls its own fragments if
// anything.
func (s *Site) StaticPage(w http.ResponseWriter, r *http.Request, content string) {
	s.render(w, r, content, "Stopped")
}

func (s *Site) render(w http.ResponseWriter, r *http.Request, content, polling string) {
	s.ctrl.RenderTemplate(w, lofigui.TemplateContext{
		"request":         r,
		"version":         "Model Bank " + s.cfg.Version,
		"controller_name": s.ctrl.Name,
		"results":         template.HTML(content),
		"polling":         polling,
		"role":            string(s.Role(w, r)),
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
	if s.Fragment(w, r, content) {
		return
	}
	s.Page(w, r, content)
}

// --- routes ---

func (s *Site) routes() {
	bank := s.cfg.Bank
	mux := s.mux

	// Role and PII
	mux.HandleFunc("/role", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			s.Redirect(w, r, "")
			return
		}
		r.ParseForm()
		role := r.FormValue("role")
		if !ValidRole(role) {
			s.Redirect(w, r, "")
			return
		}
		s.auth.SetRole(sessionID(w, r), Role(role))
		s.Redirect(w, r, r.FormValue("redirect"))
	})
	mux.HandleFunc("/auth/authorize", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			s.Redirect(w, r, "")
			return
		}
		s.auth.Authorize(sessionID(w, r))
		s.Redirect(w, r, r.FormValue("redirect"))
	})
	mux.HandleFunc("/auth/revoke", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			s.Redirect(w, r, "")
			return
		}
		s.auth.Revoke(sessionID(w, r))
		s.Redirect(w, r, r.FormValue("redirect"))
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
		s.page(w, r, func() string { return BuildCustomersHTML(bank, pageParam(r, "page"), s.PII(w, r)) })
	})
	mux.HandleFunc("/customers/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, "/customers/")
		if rest == "" {
			s.Redirect(w, r, "customers")
			return
		}
		pii := s.PII(w, r)
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
		s.page(w, r, func() string { return BuildPaymentDetailHTML(bank, id, s.PII(w, r)) })
	})

	// Reports
	mux.HandleFunc("/reports/charts", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildChartsHTML(bank) })
	})
	mux.HandleFunc("/reports/bbsi", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildBBSIHTML(bank, s.PII(w, r)) })
	})
	mux.HandleFunc("/reports/customer-view", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			s.Redirect(w, r, "customers")
			return
		}
		s.page(w, r, func() string { return BuildCustomerViewHTML(bank, id, s.PII(w, r)) })
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
			s.Redirect(w, r, "treasury/gilts")
			return
		}
		if !s.Require(w, r, "buy_gilt") {
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
		s.Redirect(w, r, "treasury/gilts")
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
