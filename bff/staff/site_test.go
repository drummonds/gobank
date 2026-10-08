package staff

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// sameOriginURL matches an attribute carrying a same-origin absolute URL.
var sameOriginURL = regexp.MustCompile(`(?:href|action|hx-get|hx-post|src|value)="(/[^"]*)"`)

// newSite is a site over no bank: enough for the pages that read none
// (about, models, docs) and the role and PII machinery.
func newSite(t *testing.T, scope string) *Site {
	t.Helper()
	return New(Config{Scope: scope, Version: "test", Status: func() string { return "Stopped" }})
}

func get(h http.Handler, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	h.ServeHTTP(rr, req)
	return rr
}

func post(h http.Handler, path, form string, cookie *http.Cookie) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", path, strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	h.ServeHTTP(rr, req)
	return rr
}

func sessionCookie(rr *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rr.Result().Cookies() {
		if c.Name == "session_id" {
			return c
		}
	}
	return nil
}

// Every page is served inside the scope the site is mounted under: it
// carries the scope as its <base>, its links are relative, and every
// redirect carries the scope.
func TestPagesStayInScope(t *testing.T) {
	const scope = "/demo/"
	site := newSite(t, scope)
	for _, page := range []string{"/about", "/about/models", "/about/docs", "/about/docs/adr/contract-views", "/favicon.svg"} {
		rr := get(site, page, nil)
		if rr.Code != http.StatusOK {
			t.Errorf("GET %s: status %d", page, rr.Code)
			continue
		}
		body := rr.Body.String()
		if page != "/favicon.svg" && !strings.Contains(body, `<base href="`+scope+`">`) {
			t.Errorf("GET %s: no <base href=%q>", page, scope)
		}
		for _, m := range sameOriginURL.FindAllStringSubmatch(body, -1) {
			if !strings.HasPrefix(m[1], scope) {
				t.Errorf("GET %s: absolute URL %q escapes the scope %q", page, m[1], scope)
			}
		}
	}
	for _, target := range []string{"//evil.example/x", "https://evil.example/x", "/customers"} {
		rr := post(site, "/role", "role=admin&redirect="+target, nil)
		if loc := rr.Header().Get("Location"); rr.Code != http.StatusSeeOther || loc != scope {
			t.Errorf("redirect=%q: %d %q, want 303 %s", target, rr.Code, loc, scope)
		}
	}
	if rr := get(site, "/no/such/page", nil); rr.Code != http.StatusNotFound {
		t.Errorf("unknown page: %d", rr.Code)
	}
}

// The role switch and the PII authorisation hang off the staff session:
// what one session chooses another does not see, and the layout shows the
// session's role.
func TestRoleAndPIIFollowTheSession(t *testing.T) {
	site := newSite(t, "/")
	rr := post(site, "/role", "role=readonly&redirect=about", nil)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/about" {
		t.Fatalf("role switch: %d %s", rr.Code, rr.Header().Get("Location"))
	}
	cookie := sessionCookie(rr)
	if cookie == nil {
		t.Fatal("the role switch started no session")
	}
	body := get(site, "/about", cookie).Body.String()
	if !strings.Contains(body, `<option value="readonly" selected>`) {
		t.Error("the layout does not show the session's role")
	}
	if strings.Contains(body, `href="settings"`) {
		t.Error("a read-only session sees the admin's Internal menu")
	}
	if body := get(site, "/about", nil).Body.String(); !strings.Contains(body, `<option value="admin" selected>`) {
		t.Error("a new session is not admin")
	}

	// PII: an admin is authorised for a while, a read-only role never.
	if site.PII(httptest.NewRecorder(), withCookie(cookie)) {
		t.Error("read-only session is authorised for PII")
	}
	rr = post(site, "/role", "role=admin&redirect=", cookie)
	if site.PII(httptest.NewRecorder(), withCookie(cookie)) {
		t.Error("admin is authorised before authorising")
	}
	rr = post(site, "/auth/authorize", "redirect=customers", cookie)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/customers" {
		t.Fatalf("authorize: %d %s", rr.Code, rr.Header().Get("Location"))
	}
	if !site.PII(httptest.NewRecorder(), withCookie(cookie)) {
		t.Error("admin is not authorised after authorising")
	}
	post(site, "/auth/revoke", "", cookie)
	if site.PII(httptest.NewRecorder(), withCookie(cookie)) {
		t.Error("admin is still authorised after revoking")
	}
	if rr := post(site, "/role", "role=emperor&redirect=about", cookie); rr.Header().Get("Location") != "/" {
		t.Errorf("an unknown role should land on the dashboard, got %s", rr.Header().Get("Location"))
	}
}

func withCookie(c *http.Cookie) *http.Request {
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(c)
	return req
}

// The console's pages (until ADR-0002 story 1.6.4 moves them here) are
// mounted on the site and rendered in its layout with the session's role
// and the console's status; a page may require a role.
func TestConsolePagesRenderInTheLayout(t *testing.T) {
	status := "Running"
	site := New(Config{Scope: "/demo/", Version: "v9", Status: func() string { return status }})
	site.Handle("/console", func(w http.ResponseWriter, r *http.Request) {
		if !site.Require(w, r, "settings") {
			return
		}
		if site.Fragment(w, r, "<p>console content</p>") {
			return
		}
		site.Page(w, r, "<p>console content</p>")
	})
	rr := get(site, "/console", nil)
	body := rr.Body.String()
	for _, want := range []string{"console content", `<base href="/demo/">`, "Model Bank v9", `hx-get="console" hx-trigger="every 1s"`, `>Running<`} {
		if !strings.Contains(body, want) {
			t.Errorf("console page missing %q", want)
		}
	}
	status = "Stopped"
	if body := get(site, "/console", nil).Body.String(); strings.Contains(body, `hx-trigger="every 1s"`) {
		t.Error("a stopped console still polls the whole page")
	}
	cookie := sessionCookie(post(site, "/role", "role=readonly&redirect=", nil))
	if rr := get(site, "/console", cookie); rr.Code != http.StatusForbidden {
		t.Errorf("read-only reaching an admin page: %d, want 403", rr.Code)
	}
	// An HTMX fragment request gets the content alone.
	rr = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/console", nil)
	req.Header.Set("HX-Request", "true")
	site.ServeHTTP(rr, req)
	if body := rr.Body.String(); body != "<p>console content</p>" {
		t.Errorf("fragment request got %q", body)
	}
}
