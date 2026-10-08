package main

import (
	"git.bytestone.uk/hum3/gobank/bff/staff"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// The demo is one http.Handler built over its state (ADR-0002 stage 6,
// story 1.6.1). The server listens on it and the WASM build serves it in
// the tab under the service worker's scope, so every page it renders must
// stay inside whatever scope it is given: links, form actions and HTMX
// polls are scope-relative against a <base>, and every redirect carries
// the scope.

// newSite is the staff web over a demo, for the pages tested on their own.
func newSite(ds *DemoState) *staff.Site { return staff.New(siteConfig(ds, "test", "/")) }

// staffPages is every GET page the staff UI serves.
var staffPages = []string{
	"/",
	"/accounting/pnl",
	"/accounting/balance-sheet",
	"/products/savings",
	"/products/lending",
	"/customers",
	"/customers/cust-001",
	"/customers/cust-001/account/0",
	"/payments",
	"/settings",
	"/settings/status",
	"/dashboard/update",
	"/reports/charts",
	"/reports/bbsi",
	"/reports/customer-view?id=cust-001",
	"/treasury/cash",
	"/treasury/capital",
	"/treasury/gilts",
	"/internal/explorer",
	"/internal/explorer/customer_accounts?filter=customer_id&value=cust-001",
	"/about",
	"/about/runtime",
	"/about/models",
	"/about/docs",
	"/about/docs/adr/contract-views",
	"/v1/screen/login",
}

// staffActions is every POST the staff UI makes, with its form body.
var staffActions = map[string]string{
	"/start":              "",
	"/stop":               "",
	"/advance":            "",
	"/add-customers":      "n=1",
	"/payments/send":      "",
	"/payments/run":       "",
	"/payments/stop":      "",
	"/settings":           "max_customers=10&day_length=",
	"/role":               "role=admin&redirect=customers",
	"/auth/authorize":     "redirect=customers/cust-001",
	"/auth/revoke":        "",
	"/treasury/gilts/buy": "tenor=5Y&face_value=1000",
	"/v1/login":           "customer_id=cust-001&password=" + testAppPassword,
}

// testAppPassword is the app password the handler tests run with.
const testAppPassword = "pw"

// sameOriginURL matches an attribute carrying a same-origin absolute URL.
var sameOriginURL = regexp.MustCompile(`(?:href|action|hx-get|hx-post|src|value)="(/[^"]*)"`)

func TestHandlerStaysInScope(t *testing.T) {
	const scope = "/demo/"
	t.Setenv("GOBANK_APP_PASSWORD", testAppPassword)
	ds := NewDemoState()
	addFundedCustomer(ds)
	h := newHandler(ds, "test", scope)

	for _, page := range staffPages {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", page, nil)
		req.Header.Set("Accept", "text/html") // a browser's navigation
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("GET %s: status %d", page, rr.Code)
			continue
		}
		body := rr.Body.String()
		if !strings.Contains(body, `<base href="`+scope+`">`) && !isFragment(page) {
			t.Errorf("GET %s: no <base href=%q>", page, scope)
		}
		for _, m := range sameOriginURL.FindAllStringSubmatch(body, -1) {
			if !strings.HasPrefix(m[1], scope) {
				t.Errorf("GET %s: absolute URL %q escapes the scope %q", page, m[1], scope)
			}
		}
	}

	for action, form := range staffActions {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", action, strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusSeeOther {
			t.Errorf("POST %s: status %d, want 303", action, rr.Code)
			continue
		}
		loc := rr.Header().Get("Location")
		if !strings.HasPrefix(loc, scope) {
			t.Errorf("POST %s: Location %q escapes the scope %q", action, loc, scope)
		}
	}
}

// isFragment says whether a page is an HTMX fragment rather than a document.
func isFragment(page string) bool {
	p, _ := url.Parse(page)
	return p.Path == "/settings/status" || p.Path == "/dashboard/update"
}

// On the server the scope is the origin root and the pages are what they
// always were: a link to the customers page is /customers.
func TestHandlerAtRoot(t *testing.T) {
	ds := NewDemoState()
	h := newHandler(ds, "test", "/")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
	body := rr.Body.String()
	if !strings.Contains(body, `<base href="/">`) {
		t.Error("dashboard: no <base href=\"/\">")
	}
	if !strings.Contains(body, `href="customers"`) {
		t.Error("dashboard: customers link is not scope-relative")
	}
}

// A redirect target supplied by a form is a path inside the scope, never
// another origin.
func TestRedirectTargetStaysOnSite(t *testing.T) {
	ds := NewDemoState()
	h := newHandler(ds, "test", "/demo/")
	for _, target := range []string{"//evil.example/x", "https://evil.example/x", "/customers"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/role", strings.NewReader("role=admin&redirect="+url.QueryEscape(target)))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		h.ServeHTTP(rr, req)
		if loc := rr.Header().Get("Location"); loc != "/demo/" {
			t.Errorf("redirect=%q: Location %q, want /demo/", target, loc)
		}
	}
}

// The customer web is the BFF's HTML (ADR-0002 stage 6, story 1.6.2): no
// customer data is served without a session, the open JSON API is gone,
// and what the browser shows is the screen tree the app renders.
func TestCustomerWebNeedsASession(t *testing.T) {
	const scope = "/demo/"
	t.Setenv("GOBANK_APP_PASSWORD", testAppPassword)
	ds := NewDemoState()
	addFundedCustomer(ds)
	h := newHandler(ds, "test", scope)
	get := func(path string, cookie *http.Cookie) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Accept", "text/html")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		h.ServeHTTP(rr, req)
		return rr
	}

	for _, gone := range []string{"/app/", "/app/customer/cust-001", "/api/customers", "/api/customer/cust-001/accounts"} {
		if rr := get(gone, nil); rr.Code != http.StatusNotFound {
			t.Errorf("GET %s: status %d, want 404 (retired)", gone, rr.Code)
		}
	}
	for _, page := range []string{"/v1/screen/accounts", "/v1/screen/activity", "/v1/screen/product/0"} {
		rr := get(page, nil)
		if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != scope+"v1/screen/login" {
			t.Errorf("GET %s without a session: %d %s, want 303 to the login screen in scope", page, rr.Code, rr.Header().Get("Location"))
		}
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/login", strings.NewReader("customer_id=cust-001&password="+testAppPassword))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != scope+"v1/screen/accounts" {
		t.Fatalf("login: %d %s", rr.Code, rr.Header().Get("Location"))
	}
	var cookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == "mb_session" {
			cookie = c
		}
	}
	if cookie == nil || cookie.Path != scope+"v1" {
		t.Fatalf("session cookie: %+v", cookie)
	}

	const dollar, pound = "&#128178;", "&#128183;"
	for _, page := range []string{"/v1/screen/accounts", "/v1/screen/activity", "/v1/screen/product/0"} {
		rr := get(page, cookie)
		if rr.Code != http.StatusOK {
			t.Errorf("GET %s with a session: %d", page, rr.Code)
			continue
		}
		body := rr.Body.String()
		if !strings.Contains(body, `<base href="`+scope+`">`) {
			t.Errorf("GET %s: no <base href=%q>", page, scope)
		}
		for _, m := range sameOriginURL.FindAllStringSubmatch(body, -1) {
			if !strings.HasPrefix(m[1], scope) {
				t.Errorf("GET %s: absolute URL %q escapes the scope %q", page, m[1], scope)
			}
		}
		if strings.Contains(body, dollar) {
			t.Errorf("GET %s: shows the dollar sign for a GBP account", page)
		}
	}
	if body := get("/v1/screen/accounts", cookie).Body.String(); !strings.Contains(body, pound) {
		t.Error("accounts: no pound note for a GBP savings account")
	}

	// A reset replaces the demo's database: the old session is gone with
	// the run it belonged to, and a customer of the new run can log in.
	ds.Reset()
	addFundedCustomer(ds)
	if rr := get("/v1/screen/accounts", cookie); rr.Code != http.StatusSeeOther {
		t.Errorf("GET accounts with the old run's session after a reset: %d, want 303", rr.Code)
	}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/v1/login", strings.NewReader("customer_id=cust-001&password="+testAppPassword))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != scope+"v1/screen/accounts" {
		t.Fatalf("login after a reset: %d %s", rr.Code, rr.Header().Get("Location"))
	}
	for _, c := range rr.Result().Cookies() {
		if c.Name == "mb_session" {
			cookie = c
		}
	}
	if rr := get("/v1/screen/accounts", cookie); rr.Code != http.StatusOK {
		t.Errorf("GET accounts after a reset and a new login: %d", rr.Code)
	}
}
