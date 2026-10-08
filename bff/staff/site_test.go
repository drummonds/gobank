package staff

import (
	"bytes"
	"database/sql"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// sameOriginURL matches an attribute carrying a same-origin absolute URL.
var sameOriginURL = regexp.MustCompile(`(?:href|action|hx-get|hx-post|src|value)="(/[^"]*)"`)

// newSite is a site over no bank: enough for the pages that read none
// (about, models, docs) and the role and PII machinery.
func newSite(t *testing.T, scope string) *Site {
	t.Helper()
	return New(Config{Scope: scope, Version: "test"})
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
	if site.pii(httptest.NewRecorder(), withCookie(cookie)) {
		t.Error("read-only session is authorised for PII")
	}
	rr = post(site, "/role", "role=admin&redirect=", cookie)
	if site.pii(httptest.NewRecorder(), withCookie(cookie)) {
		t.Error("admin is authorised before authorising")
	}
	rr = post(site, "/auth/authorize", "redirect=customers", cookie)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/customers" {
		t.Fatalf("authorize: %d %s", rr.Code, rr.Header().Get("Location"))
	}
	if !site.pii(httptest.NewRecorder(), withCookie(cookie)) {
		t.Error("admin is not authorised after authorising")
	}
	post(site, "/auth/revoke", "", cookie)
	if site.pii(httptest.NewRecorder(), withCookie(cookie)) {
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

// fakeConsole is a console that records what it was asked to do.
type fakeConsole struct {
	status   SimStatus
	calls    []string
	settings Settings
	exported string
	imported string
}

func (c *fakeConsole) SimStatus() SimStatus         { return c.status }
func (c *fakeConsole) Start()                       { c.calls = append(c.calls, "start") }
func (c *fakeConsole) Stop()                        { c.calls = append(c.calls, "stop") }
func (c *fakeConsole) AdvanceDay()                  { c.calls = append(c.calls, "advance") }
func (c *fakeConsole) Reset()                       { c.calls = append(c.calls, "reset") }
func (c *fakeConsole) AddCustomersBatch(n int)      { c.calls = append(c.calls, "add "+strconv.Itoa(n)) }
func (c *fakeConsole) SendPayment()                 { c.calls = append(c.calls, "send") }
func (c *fakeConsole) StartPayments()               { c.calls = append(c.calls, "payments run") }
func (c *fakeConsole) StopPayments()                { c.calls = append(c.calls, "payments stop") }
func (c *fakeConsole) Settings() Settings           { return c.settings }
func (c *fakeConsole) UpdateSettings(n int)         { c.settings.MaxCustomers = n }
func (c *fakeConsole) SetDayLength(d time.Duration) { c.settings.DayLength = d }
func (c *fakeConsole) Restarts(int) []Restart       { return nil }
func (c *fakeConsole) Runtime() Runtime             { return Runtime{Env: "test"} }
func (c *fakeConsole) Export(w io.Writer) error     { _, err := io.WriteString(w, c.exported); return err }
func (c *fakeConsole) Import(r io.Reader) error {
	b, err := io.ReadAll(r)
	c.imported = string(b)
	return err
}
func (c *fakeConsole) DB() *sql.DB { return nil }

// The console's controls drive the console the site is given, need the
// role for them and return to the page in scope; the layout's status tag
// and whole-page poll follow the console; export and import pass the
// book through.
func TestConsoleControls(t *testing.T) {
	console := &fakeConsole{exported: "book", settings: Settings{MaxCustomers: 10}}
	site := New(Config{Scope: "/demo/", Version: "v9", Console: console})

	for action, want := range map[string]string{
		"/start": "start", "/stop": "stop", "/advance": "advance", "/reset": "reset",
		"/add-customers": "add 7", "/payments/send": "send", "/payments/run": "payments run", "/payments/stop": "payments stop",
	} {
		console.calls = nil
		rr := post(site, action, "n=7", nil)
		if rr.Code != http.StatusSeeOther || !strings.HasPrefix(rr.Header().Get("Location"), "/demo/") {
			t.Errorf("POST %s: %d %s, want 303 in scope", action, rr.Code, rr.Header().Get("Location"))
		}
		if len(console.calls) != 1 || console.calls[0] != want {
			t.Errorf("POST %s: console was asked %v, want %q", action, console.calls, want)
		}
	}
	cookie := sessionCookie(post(site, "/role", "role=readonly&redirect=", nil))
	console.calls = nil
	if rr := post(site, "/start", "", cookie); rr.Code != http.StatusForbidden || len(console.calls) != 0 {
		t.Errorf("read-only starting the run: %d %v, want 403 and nothing done", rr.Code, console.calls)
	}

	rr := post(site, "/settings", "max_customers=50&day_length=2h", nil)
	if rr.Code != http.StatusSeeOther || console.settings.MaxCustomers != 50 || console.settings.DayLength != 2*time.Hour {
		t.Errorf("settings: %d %+v", rr.Code, console.settings)
	}

	rr = get(site, "/export.goluca", nil)
	if rr.Code != http.StatusOK || rr.Body.String() != "book" || !strings.Contains(rr.Header().Get("Content-Disposition"), "gobank.goluca") {
		t.Errorf("export: %d %q %s", rr.Code, rr.Body.String(), rr.Header().Get("Content-Disposition"))
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "x.goluca")
	io.WriteString(fw, "new book")
	mw.Close()
	rr = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	site.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther || console.imported != "new book" {
		t.Errorf("import: %d %q", rr.Code, console.imported)
	}

	// The layout follows the console: busy polls the whole page.
	page := get(site, "/about", nil).Body.String()
	if !strings.Contains(page, `>Stopped<`) || strings.Contains(page, `hx-trigger="every 1s"`) {
		t.Error("an idle console should show Stopped and not poll")
	}
	console.status.PaymentsRunning = true
	page = get(site, "/about", nil).Body.String()
	if !strings.Contains(page, `>Running<`) || !strings.Contains(page, `hx-get="about" hx-trigger="every 1s"`) {
		t.Error("a busy console should show Running and poll the page")
	}
}

// Without a console the site serves the bank's pages alone.
func TestNoConsoleNoConsolePages(t *testing.T) {
	site := newSite(t, "/")
	for _, p := range []string{"/", "/settings", "/about/runtime", "/about.json", "/export.goluca", "/internal/explorer"} {
		if rr := get(site, p, nil); rr.Code != http.StatusNotFound {
			t.Errorf("GET %s without a console: %d, want 404", p, rr.Code)
		}
	}
}
