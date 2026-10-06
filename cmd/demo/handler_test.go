package main

import (
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
	"/app/",
	"/app/customer/cust-001",
	"/app/customer/cust-001/transactions",
	"/app/customer/cust-001/product/0",
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
	"/app/login":          "customer_id=cust-001",
}

// sameOriginURL matches an attribute carrying a same-origin absolute URL.
var sameOriginURL = regexp.MustCompile(`(?:href|action|hx-get|hx-post|src|value)="(/[^"]*)"`)

func TestHandlerStaysInScope(t *testing.T) {
	const scope = "/demo/"
	ds := NewDemoState()
	addFundedCustomer(ds)
	h := newHandler(ds, "test", scope)

	for _, page := range staffPages {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest("GET", page, nil))
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
