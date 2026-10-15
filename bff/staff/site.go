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
	"net/url"
	"strconv"
	"strings"
	"time"

	dbexplorer "git.bytestone.uk/hum3/go-dbexplorer"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/adr"
	"git.bytestone.uk/hum3/gobank/api"
	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/gobank/internal/routestats"
	"git.bytestone.uk/hum3/gobank/internal/servemux"
	"git.bytestone.uk/hum3/gobank/internal/session"
	"git.bytestone.uk/hum3/lofigui"
)

// Config configures a Site.
type Config struct {
	Bank     core.StaffQueries // what the pages read; nil serves only the pages that read nothing
	Commands core.Commands     // what the pages act on (buying a gilt); nil refuses
	Users    core.Users        // who may sign in; nil lets nobody in

	// Stats counts and times every route served, for the API pages and
	// about.json to show; nil keeps no figures. The demo shares one with
	// the customer BFF so the pages cover both.
	Stats *routestats.Stats

	// Sessions is where the signed-in users are kept; nil keeps them in
	// memory for the process. SecureCookies marks the session cookie
	// Secure: on unless serving plain HTTP. LoginNote is a note under the
	// login form; the tab uses it to state its password.
	Sessions      session.Store
	SecureCookies bool
	LoginNote     string

	// APIToken lets another program drive the console (story 9.4): a
	// request carrying it as a bearer token is served as the operator.
	// Empty admits no operator.
	APIToken string

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
	mux      *servemux.Recording
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
		cfg: cfg, mux: servemux.New(), sessions: sessions,
		limiter: session.NewLimiter(5, 15*time.Minute, time.Now),
		pii:     newPIIStore(cfg.PIITTL), ctrl: ctrl, catalog: explorerCatalog(cfg.Components),
	}
	s.loginRoutes()
	s.routes()
	s.viewRoutes()
	s.movementRoutes()
	if cfg.Console != nil {
		s.consoleRoutes()
	}
	if cfg.Stats != nil {
		s.mux.Observe = cfg.Stats.Observe
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

// polling says whether a page's own fragments should re-poll: while the
// console runs.
func (s *Site) polling() bool { return s.status() == "Running" }

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
	s.render(w, r, content, s.polling())
}

// staticPage renders content in the layout with the whole-page poll off,
// so forms on it keep what is typed; the page polls its own fragments if
// anything. The status tag still says whether the console runs.
func (s *Site) staticPage(w http.ResponseWriter, r *http.Request, content string) {
	s.render(w, r, content, false)
}

// render lays content out; polling says whether the layout re-fetches
// the page every second while the console runs.
func (s *Site) render(w http.ResponseWriter, r *http.Request, content string, polling bool) {
	sess, signedIn := signedIn(r)
	v := s.view(r)
	s.ctrl.RenderTemplate(w, lofigui.TemplateContext{
		"request":          r,
		"view":             string(v),
		"view_label":       v.Label(),
		"other_view":       string(v.Other()),
		"other_view_label": v.Other().Label(),
		"version":          "Model Bank " + s.cfg.Version,
		"controller_name":  s.ctrl.Name,
		"results":          template.HTML(content),
		"status":           s.status(),
		"polling":          polling,
		"signed_in":        signedIn,
		"login":            sess.user.Login,
		"role":             string(sess.role),
		"role_label":       sess.role.Label(),
		"scope":            s.cfg.Scope,
		"path":             strings.TrimPrefix(r.URL.Path, "/"),
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

// masterPage is a GET master-detail page: content is served alone to
// HTMX, or rendered in the layout with the whole-page poll off, since
// that poll would wipe the detail a row has loaded; the master polls
// itself while the console runs.
func (s *Site) masterPage(w http.ResponseWriter, r *http.Request, build func() string) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	content := build()
	if s.fragment(w, r, content) {
		return
	}
	s.staticPage(w, r, content)
}

// routes mounts the staff pages: accounting, products, customers,
// payments, treasury, reports and about, and the PII authorisation.
func (s *Site) routes() {
	bank := s.cfg.Bank
	mux := s.mux

	// PII (POST): an admin authorises themselves to see personal data for a
	// while, then returns to redirect=.
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
	// PII (POST): the admin gives the authorisation up and returns to
	// redirect=.
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

	// The profit and loss statement since the bank opened, on an accrual
	// basis.
	mux.HandleFunc("/accounting/pnl", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildPnLHTML(bank) })
	})
	// The balance sheet on the current business day, with the CET1 position.
	mux.HandleFunc("/accounting/balance-sheet", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildBalanceSheetHTML(bank) })
	})

	// Products: the masters (a family's products, the bank's own) and
	// the detail, a parameter's own page, which is also where a staff
	// setting is made (ADR-0010): decided by the signed-in user.

	// The savings products: the family's catalogue, each product with its
	// book.
	mux.HandleFunc("/products/savings", func(w http.ResponseWriter, r *http.Request) {
		s.masterPage(w, r, func() string { return BuildProductsHTML(bank, gbp.FamilySavings, s.view(r), s.polling()) })
	})
	// The lending products: the family's catalogue, each product with its
	// book.
	mux.HandleFunc("/products/lending", func(w http.ResponseWriter, r *http.Request) {
		s.masterPage(w, r, func() string { return BuildProductsHTML(bank, gbp.FamilyLending, s.view(r), s.polling()) })
	})
	// The bank's own parameters (Bank Rate, the reserve ratio and the rest),
	// as the Bank tab shows them.
	mux.HandleFunc("/products/bank", func(w http.ResponseWriter, r *http.Request) {
		s.masterPage(w, r, func() string { return BuildBankHTML(bank, s.polling()) })
	})
	// One parameter of a product or of the bank: its history and, for a
	// signed-in user who may, the form that sets it (POST value, effective);
	// ?refused= carries a refusal back to the page.
	mux.HandleFunc("/products/{owner}/parameters/{key}", func(w http.ResponseWriter, r *http.Request) {
		owner, key := r.PathValue("owner"), r.PathValue("key")
		d, err := bank.Parameter(r.Context(), owner, key)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		detail := func(refused string) string {
			d, _ = bank.Parameter(r.Context(), owner, key) // after a setting, the state as it now is
			pos, _ := bank.Position(r.Context())
			return BuildParameterHTML(d, pos.Day, s.role(r).Can("settings"), s.polling(), refused)
		}
		switch r.Method {
		case "GET":
			content := detail(r.URL.Query().Get("refused"))
			if s.fragment(w, r, content) {
				return
			}
			s.staticPage(w, r, `<p><a href="`+ownerPage(d)+`">&larr; Products</a></p>`+content)
		case "POST":
			if !s.require(w, r, "settings") {
				return
			}
			if s.cfg.Commands == nil {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			sess, _ := signedIn(r)
			setting := core.ParameterSetting{Owner: owner, Key: key, Value: r.FormValue("value"), By: sess.user.Login}
			if setting.Effective, _ = time.Parse("2006-01-02T15:04", r.FormValue("effective")); setting.Effective.IsZero() {
				if pos, err := bank.Position(r.Context()); err == nil {
					setting.Effective = pos.Day // an empty instant is the bank's day: in force from now
				}
			}
			refused := ""
			if err := s.cfg.Commands.SetParameter(r.Context(), setting); err != nil {
				log.Printf("set parameter %+v: %v", setting, err)
				refused = err.Error()
			}
			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				fmt.Fprint(w, detail(refused))
				return
			}
			page := parameterPath(owner, key)
			if refused != "" {
				page += "?refused=" + url.QueryEscape(refused)
			}
			s.redirect(w, r, page)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// The customers, one page (?page=) at a time; personal data only for an
	// authorised admin.
	mux.HandleFunc("/customers", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildCustomersHTML(bank, pageParam(r, "page"), s.piiAuthorised(r)) })
	})
	// Customer search by name (POST name=): the customers whose name
	// contains it, oldest first and at most core.CustomerSearchLimit, as the
	// register's table — the matches alone for htmx, the Customers page
	// with them for a browser. The name is personal data: it travels in the
	// body and never in a URL, and only an authorised admin may search.
	mux.HandleFunc("/customers/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !s.piiAuthorised(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		results := BuildCustomerSearchHTML(bank, r.FormValue("name"))
		if s.fragment(w, r, results) {
			return
		}
		s.staticPage(w, r, customersPage(bank, 1, true, strings.Replace(searchForm, `<div id="customer-search-results"></div>`, results, 1)))
	})
	// Under /customers/: a customer's detail at /customers/{id}, or one of
	// their accounts at /customers/{id}/account/{index}, each with its
	// transactions one page (?txpage=) at a time.
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
			s.page(w, r, func() string { return BuildCustomerAccountHTML(bank, parts[0], idx, pii, txPage, s.view(r)) })
			return
		}
		s.page(w, r, func() string { return BuildCustomerDetailHTML(bank, parts[0], pii, txPage, s.view(r)) })
	})

	// One payment: its detail and the movements it made.
	mux.HandleFunc("/payments/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		s.page(w, r, func() string { return BuildPaymentDetailHTML(bank, id, s.piiAuthorised(r)) })
	})

	// Events: the record of what happened and what it caused (ADR-0011)
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildEventsHTML(bank) })
	})

	// The charts: the book, the customer count and the rates over the run.
	mux.HandleFunc("/reports/charts", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildChartsHTML(bank) })
	})
	// The BBSI return (Bank and Building Society Interest): each customer's
	// interest for the year; personal data only for an authorised admin.
	mux.HandleFunc("/reports/bbsi", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildBBSIHTML(bank, s.piiAuthorised(r)) })
	})
	// A customer as the app shows them (?id=); without an id, the customer
	// list.
	mux.HandleFunc("/reports/customer-view", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			s.redirect(w, r, "customers")
			return
		}
		s.page(w, r, func() string { return BuildCustomerViewHTML(bank, id, s.piiAuthorised(r)) })
	})

	// The cash position: deposits, loans, cash at the BoE against the
	// required reserves, and the Bank Rate.
	mux.HandleFunc("/treasury/cash", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildCashPositionHTML(bank) })
	})
	// The reserve ratio against its requirement, and the lending headroom.
	mux.HandleFunc("/treasury/capital", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildCapitalHTML(bank) })
	})
	// The gilt holdings and the purchase form.
	mux.HandleFunc("/treasury/gilts", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildGiltsHTML(bank) })
	})
	// Buys a gilt (POST tenor, face_value in pounds): admin only; returns to
	// the gilts page.
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

	// The project: what the Model Bank is, its aims and its non-goals.
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/about" {
			http.NotFound(w, r)
			return
		}
		s.page(w, r, BuildProjectAboutHTML)
	})
	// The architecture models: the C4 system context and container diagrams.
	mux.HandleFunc("/about/models", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, BuildModelsHTML)
	})
	// The system documentation: the component registry, who may read what,
	// the pre-rule debt and the ADR list.
	mux.HandleFunc("/about/docs", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, func() string { return BuildDocsHTML(s.cfg.Components, s.cfg.Debt) })
	})
	// The API: every route the BFF and the staff web serve, read off the
	// code at build time (package api), in sections by where each is
	// registered, each linking to its own page; the types the bodies
	// are made of follow. The OpenAPI document is at v1/openapi.json.
	// A static page: the surface does not change while the run goes.
	mux.HandleFunc("/about/api", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.staticPage(w, r, BuildAPIIndexHTML(s.cfg.Version, s.cfg.Stats))
	})
	// One route of the API: what its handler needs, reads and answers,
	// with a link to the registration in the source at this build's tag.
	// The method is the pattern's or "any", the path the pattern's.
	mux.HandleFunc("/about/api/{method}/{path...}", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		content, ok := BuildAPIDetailHTML(r.PathValue("method"), "/"+r.PathValue("path"), s.cfg.Version, s.cfg.Stats)
		if !ok {
			http.NotFound(w, r)
			return
		}
		s.staticPage(w, r, content)
	})
	// The architecture decision records compiled into this build.
	mux.HandleFunc("/about/adr", func(w http.ResponseWriter, r *http.Request) {
		s.page(w, r, BuildADRIndexHTML)
	})
	// An ADR whole: its page, or one of the diagrams and sources its
	// markdown names beside it.
	mux.HandleFunc("/about/docs/adr/", func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimPrefix(r.URL.Path, "/about/docs/adr/")
		if body, kind, ok := adr.Asset(slug); ok {
			w.Header().Set("Content-Type", kind)
			w.Write(body)
			return
		}
		page, ok := BuildADRHTML(slug)
		if !ok {
			http.NotFound(w, r)
			return
		}
		s.page(w, r, func() string { return page })
	})

	// The tab icon as .ico, from lofigui.
	mux.HandleFunc("/favicon.ico", lofigui.ServeFavicon)
	// The tab icon as SVG.
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

// apiOptions link the API pages: a route's detail under about/api, and
// its source at this build's tag in the forge, or on main for a build
// that is not a tag.
func apiOptions(version string, stats *routestats.Stats) api.Options {
	ref := "branch/main"
	if strings.HasPrefix(version, "v") && !strings.Contains(version, "-") {
		ref = "tag/" + version
	}
	o := api.Options{
		DetailURL: func(r api.Route) string { return "about/api/" + api.Key(r) },
		IndexURL:  "about/api",
		SourceRef: ref,
		Measured:  api.LoadMeasured(), // the last scan committed, if any
	}
	if stats != nil {
		// One snapshot per page: a route is matched by the pattern it was
		// registered with, which is what the stats are kept by.
		byPattern := map[string]routestats.RouteStat{}
		for _, st := range stats.Snapshot() {
			byPattern[st.Pattern] = st
		}
		o.Perf = func(r api.Route) (api.Perf, bool) {
			pattern := r.Path
			if r.Method != "" {
				pattern = r.Method + " " + r.Path
			}
			st, ok := byPattern[pattern]
			if !ok {
				return api.Perf{}, false
			}
			return api.Perf{Count: st.Count, PeakInFlight: st.PeakInFlight, P50: st.P50, P95: st.P95, Max: st.Max, PerSecond: st.PerSecond}, true
		}
	}
	return o
}

// BuildAPIIndexHTML renders the API index and the types from the
// generated surface, with what the process has measured of each route
// when stats are given.
func BuildAPIIndexHTML(version string, stats *routestats.Stats) string {
	s, err := api.Load()
	if err != nil {
		return "<p>The API surface did not load: " + template.HTMLEscapeString(err.Error()) + "</p>"
	}
	return string(api.Index(s, apiOptions(version, stats))) + string(api.Types(s))
}

// BuildAPIDetailHTML renders one route's page; false when no route has
// the method and path.
func BuildAPIDetailHTML(method, path, version string, stats *routestats.Stats) (string, bool) {
	s, err := api.Load()
	if err != nil {
		return "", false
	}
	r, ok := api.Find(s, method, path)
	if !ok {
		return "", false
	}
	return string(api.Detail(s, r, apiOptions(version, stats))) + string(api.Types(s)), true
}
