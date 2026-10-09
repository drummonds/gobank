package staff

import (
	"bytes"
	"context"
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

	"git.bytestone.uk/hum3/gobank/core"
)

// sameOriginURL matches an attribute carrying a same-origin absolute URL.
var sameOriginURL = regexp.MustCompile(`(?:href|action|hx-get|hx-post|src|value)="(/[^"]*)"`)

// fakeUsers is a users component of a few logins, each with the password
// "pw": one per staff role and a customer, who may not sign in here.
type fakeUsers map[string][]string

var testUsers = fakeUsers{"admin": {"admin"}, "aud": {"auditor"}, "cs": {"cs"}, "ro": {"readonly"}, "cust-001": {"customer"}}

func (f fakeUsers) user(login string) core.User {
	return core.User{ID: "id-" + login, Login: login, Roles: f[login]}
}

func (f fakeUsers) AuthenticateUser(_ context.Context, login, password string) (core.User, error) {
	if _, ok := f[login]; !ok || password != "pw" {
		return core.User{}, core.ErrBadCredentials
	}
	return f.user(login), nil
}

func (f fakeUsers) User(_ context.Context, id string) (core.User, error) {
	login := strings.TrimPrefix(id, "id-")
	if _, ok := f[login]; !ok {
		return core.User{}, core.ErrNotFound
	}
	return f.user(login), nil
}

// newSite is a site over no bank: enough for the pages that read none
// (about, models, docs), the login and the PII machinery.
func newSite(t *testing.T, scope string) *Site {
	t.Helper()
	return New(Config{Scope: scope, Version: "test", Users: testUsers})
}

// signIn logs a test user in and returns the session cookie.
func signIn(t *testing.T, site *Site, login string) *http.Cookie {
	t.Helper()
	rr := post(site, "/login", "login="+login+"&password=pw&redirect=", nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("login %s: %d %s", login, rr.Code, rr.Body.String())
	}
	c := sessionCookie(rr)
	if c == nil {
		t.Fatalf("login %s set no session cookie", login)
	}
	return c
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
		if c.Name == "staff_session" && c.Value != "" {
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
	admin := signIn(t, site, "admin")
	if admin.Path != scope {
		t.Errorf("session cookie path %q, want the scope %q", admin.Path, scope)
	}
	for _, page := range []string{"/login", "/about", "/about/models", "/about/docs", "/about/docs/adr/contract-views", "/favicon.svg"} {
		cookie := admin
		if page == "/login" {
			cookie = nil
		}
		rr := get(site, page, cookie)
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
		rr := post(site, "/login", "login=admin&password=pw&redirect="+target, nil)
		if loc := rr.Header().Get("Location"); rr.Code != http.StatusSeeOther || loc != scope {
			t.Errorf("redirect=%q: %d %q, want 303 %s", target, rr.Code, loc, scope)
		}
		rr = post(site, "/auth/authorize", "redirect="+target, admin)
		if loc := rr.Header().Get("Location"); rr.Code != http.StatusSeeOther || loc != scope {
			t.Errorf("authorize redirect=%q: %d %q, want 303 %s", target, rr.Code, loc, scope)
		}
	}
	if rr := get(site, "/no/such/page", admin); rr.Code != http.StatusNotFound {
		t.Errorf("unknown page: %d", rr.Code)
	}
}

// Every page needs a signed-in member of staff (story 1.7.1): without a
// session the browser is sent to the login page and comes back to the
// page it wanted; a wrong password is refused and, repeated, locked out;
// a customer's login is not a staff login; the layout shows who is signed
// in and what they may see; logging out ends the session.
func TestPagesNeedALogin(t *testing.T) {
	const scope = "/demo/"
	site := newSite(t, scope)

	rr := get(site, "/about/models?x=1", nil)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != scope+"login?redirect=about%2Fmodels%3Fx%3D1" {
		t.Errorf("page without a session: %d %s, want 303 to the login page with the page to come back to", rr.Code, rr.Header().Get("Location"))
	}
	rr = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/about/models", nil)
	req.Header.Set("HX-Request", "true")
	site.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized || rr.Header().Get("HX-Redirect") != scope+"login?redirect=about%2Fmodels" {
		t.Errorf("HTMX request without a session: %d HX-Redirect %q, want 401 with the login page", rr.Code, rr.Header().Get("HX-Redirect"))
	}
	if rr := post(site, "/start", "", nil); rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != scope+"login" {
		t.Errorf("POST without a session: %d %s, want 303 to the login page", rr.Code, rr.Header().Get("Location"))
	}

	rr = get(site, "/login?redirect=about%2Fmodels", nil)
	body := rr.Body.String()
	if rr.Code != http.StatusOK || !strings.Contains(body, `name="password"`) || !strings.Contains(body, `name="redirect" value="about/models"`) {
		t.Errorf("login page: %d, want the form carrying the page to come back to: %.400s", rr.Code, body)
	}
	if !strings.Contains(body, "Model Bank test") {
		t.Error("the login page lacks the version footer gobank-deploy probes")
	}
	if strings.Contains(body, `href="customers"`) || strings.Contains(body, `action="logout"`) {
		t.Error("the login page shows the navigation of a signed-in user")
	}

	rr = post(site, "/login", "login=admin&password=wrong&redirect=about%2Fmodels", nil)
	if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "Unknown login or wrong password") || sessionCookie(rr) != nil {
		t.Errorf("wrong password: %d, cookie %v", rr.Code, sessionCookie(rr))
	}
	if rr := post(site, "/login", "login=cust-001&password=pw", nil); rr.Code != http.StatusUnauthorized || sessionCookie(rr) != nil {
		t.Errorf("a customer signing in to the staff web: %d, want 401", rr.Code)
	}

	rr = post(site, "/login", "login=admin&password=pw&redirect=about%2Fmodels", nil)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != scope+"about/models" {
		t.Fatalf("login: %d %s, want 303 back to the page", rr.Code, rr.Header().Get("Location"))
	}
	admin := sessionCookie(rr)
	if admin == nil || !admin.HttpOnly || admin.Path != scope {
		t.Fatalf("session cookie %+v, want HttpOnly in scope", admin)
	}
	body = get(site, "/about", admin).Body.String()
	if !strings.Contains(body, `<span class="signed-in">admin <span class="tag is-light">Admin</span></span>`) {
		t.Error("the layout does not show who is signed in")
	}
	if !strings.Contains(body, `href="settings"`) || !strings.Contains(body, `action="logout"`) {
		t.Error("an admin lacks the Internal menu or the logout")
	}
	if strings.Contains(body, `<select name="role"`) {
		t.Error("the role switch is still there")
	}
	if rr := get(site, "/login", admin); rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != scope {
		t.Errorf("the login page for a signed-in user: %d %s, want 303 to the dashboard", rr.Code, rr.Header().Get("Location"))
	}

	ro := signIn(t, site, "ro")
	if body := get(site, "/about", ro).Body.String(); strings.Contains(body, `href="settings"`) || !strings.Contains(body, "Read Only") {
		t.Error("a read-only user sees the admin's Internal menu or not their role")
	}

	rr = post(site, "/logout", "", admin)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != scope+"login" {
		t.Errorf("logout: %d %s", rr.Code, rr.Header().Get("Location"))
	}
	if rr := get(site, "/about", admin); rr.Code != http.StatusSeeOther {
		t.Errorf("page after logout: %d, want 303 to the login page", rr.Code)
	}

	// Lockout: five wrong passwords for one login lock it, right one or not.
	for i := 0; i < 5; i++ {
		post(site, "/login", "login=aud&password=wrong", nil)
	}
	rr = post(site, "/login", "login=aud&password=pw", nil)
	if rr.Code != http.StatusTooManyRequests || rr.Header().Get("Retry-After") == "" {
		t.Errorf("login after five failures: %d, want 429 with Retry-After", rr.Code)
	}
}

// Without a users component nobody signs in.
func TestNoUsersNoLogin(t *testing.T) {
	site := New(Config{Scope: "/", Version: "test"})
	if rr := post(site, "/login", "login=admin&password=pw", nil); rr.Code != http.StatusServiceUnavailable || sessionCookie(rr) != nil {
		t.Errorf("login with no users: %d, want 503", rr.Code)
	}
}

// The PII authorisation hangs off the staff session and follows the role:
// an admin is authorised for a while after asking, an auditor always, a
// read-only user never, and one session's authorisation is not another's.
func TestPIIFollowsTheSession(t *testing.T) {
	site := newSite(t, "/")
	pii := func(c *http.Cookie) bool {
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(c)
		sess, ok := site.lookup(req)
		if !ok {
			t.Fatal("no session")
		}
		return site.piiAuthorised(req.WithContext(context.WithValue(req.Context(), sessionKey{}, sess)))
	}
	admin, ro, aud := signIn(t, site, "admin"), signIn(t, site, "ro"), signIn(t, site, "aud")
	if pii(ro) {
		t.Error("read-only session is authorised for PII")
	}
	if !pii(aud) {
		t.Error("an auditor is not authorised for PII")
	}
	if pii(admin) {
		t.Error("admin is authorised before authorising")
	}
	rr := post(site, "/auth/authorize", "redirect=customers", admin)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/customers" {
		t.Fatalf("authorize: %d %s", rr.Code, rr.Header().Get("Location"))
	}
	if !pii(admin) {
		t.Error("admin is not authorised after authorising")
	}
	if other := signIn(t, site, "admin"); pii(other) {
		t.Error("another session of the same user is authorised too")
	}
	post(site, "/auth/revoke", "", admin)
	if pii(admin) {
		t.Error("admin is still authorised after revoking")
	}
	post(site, "/auth/authorize", "redirect=", ro)
	if pii(ro) {
		t.Error("a read-only user authorised themselves")
	}
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
	site := New(Config{Scope: "/demo/", Version: "v9", Console: console, Users: testUsers})
	admin := signIn(t, site, "admin")

	for action, want := range map[string]string{
		"/start": "start", "/stop": "stop", "/advance": "advance", "/reset": "reset",
		"/add-customers": "add 7", "/payments/send": "send", "/payments/run": "payments run", "/payments/stop": "payments stop",
	} {
		console.calls = nil
		rr := post(site, action, "n=7", admin)
		if rr.Code != http.StatusSeeOther || !strings.HasPrefix(rr.Header().Get("Location"), "/demo/") {
			t.Errorf("POST %s: %d %s, want 303 in scope", action, rr.Code, rr.Header().Get("Location"))
		}
		if len(console.calls) != 1 || console.calls[0] != want {
			t.Errorf("POST %s: console was asked %v, want %q", action, console.calls, want)
		}
	}
	ro := signIn(t, site, "ro")
	console.calls = nil
	if rr := post(site, "/start", "", ro); rr.Code != http.StatusForbidden || len(console.calls) != 0 {
		t.Errorf("read-only starting the run: %d %v, want 403 and nothing done", rr.Code, console.calls)
	}

	// about.json is read by another program (gobank-deploy's drill) and
	// stays open; the demo's handler test reads it without a session.
	if !open("/about.json") || open("/about") {
		t.Error("about.json should be open and about not")
	}

	rr := post(site, "/settings", "max_customers=50&day_length=2h", admin)
	if rr.Code != http.StatusSeeOther || console.settings.MaxCustomers != 50 || console.settings.DayLength != 2*time.Hour {
		t.Errorf("settings: %d %+v", rr.Code, console.settings)
	}

	rr = get(site, "/export.goluca", admin)
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
	req.AddCookie(admin)
	site.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther || console.imported != "new book" {
		t.Errorf("import: %d %q", rr.Code, console.imported)
	}

	// The layout follows the console: busy polls the whole page.
	page := get(site, "/about", admin).Body.String()
	if !strings.Contains(page, `>Stopped<`) || strings.Contains(page, `hx-trigger="every 1s"`) {
		t.Error("an idle console should show Stopped and not poll")
	}
	console.status.PaymentsRunning = true
	page = get(site, "/about", admin).Body.String()
	if !strings.Contains(page, `>Running<`) || !strings.Contains(page, `hx-get="about" hx-trigger="every 1s"`) {
		t.Error("a busy console should show Running and poll the page")
	}
}

// Without a console the site serves the bank's pages alone.
func TestNoConsoleNoConsolePages(t *testing.T) {
	site := newSite(t, "/")
	admin := signIn(t, site, "admin")
	for _, p := range []string{"/", "/settings", "/about/runtime", "/about.json", "/export.goluca", "/internal/explorer"} {
		if rr := get(site, p, admin); rr.Code != http.StatusNotFound {
			t.Errorf("GET %s without a console: %d, want 404", p, rr.Code)
		}
	}
}
