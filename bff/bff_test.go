package bff_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/gobank/bff"
	"git.bytestone.uk/hum3/gobank/bff/stubbank"
	"git.bytestone.uk/hum3/gobank/screen"
)

type fixture struct {
	t   *testing.T
	ts  *httptest.Server
	now time.Time
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{t: t, now: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)}
	bank := stubbank.New()
	srv := bff.NewServer(bff.Config{
		Bank: bank, Auth: bank,
		MaxLoginFailures: 3, LoginWindow: 10 * time.Minute,
		SessionIdle: 5 * time.Minute, SessionTTL: time.Hour,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:    func() time.Time { return f.now },
	})
	f.ts = httptest.NewServer(srv)
	t.Cleanup(f.ts.Close)
	return f
}

func (f *fixture) do(method, path, token, contentType, body string, hdr ...string) (*http.Response, map[string]any) {
	req, _ := http.NewRequest(method, f.ts.URL+path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	res, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		f.t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	var m map[string]any
	if strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(raw, &m); err != nil {
			f.t.Fatalf("bad json %q: %v", raw, err)
		}
	} else {
		m = map[string]any{"_raw": string(raw)}
	}
	return res, m
}

func (f *fixture) login(id, pw string) (*http.Response, map[string]any) {
	b, _ := json.Marshal(map[string]string{"customer_id": id, "password": pw})
	return f.do("POST", "/v1/login", "", "application/json", string(b))
}

func screenOf(m map[string]any) map[string]any {
	if _, ok := m["schema"]; ok {
		return m
	}
	s, _ := m["screen"].(map[string]any)
	return s
}

func TestUnauthenticatedIsRefused(t *testing.T) {
	f := newFixture(t)
	for _, p := range []string{"/v1/screen/accounts", "/v1/screen/activity", "/v1/screen/product/0"} {
		res, m := f.do("GET", p, "", "", "")
		if res.StatusCode != http.StatusUnauthorized || m["error"] != "unauthenticated" {
			t.Errorf("%s: got %d %v", p, res.StatusCode, m)
		}
		if sc := screenOf(m); sc == nil || sc["id"] != "login" {
			t.Errorf("%s: 401 should carry the login screen, got %v", p, m)
		}
		if res.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s: missing no-store", p)
		}
	}
	res, _ := f.do("GET", "/v1/screen/accounts", "not-a-token", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("bad token accepted: %d", res.StatusCode)
	}
	res, _ = f.do("POST", "/v1/logout", "", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("logout without session: %d", res.StatusCode)
	}
	res, _ = f.do("GET", "/api/customer/cust-001/accounts", "", "", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown path: %d", res.StatusCode)
	}
}

func TestPublicSurface(t *testing.T) {
	f := newFixture(t)
	res, m := f.do("GET", "/v1/health", "", "", "")
	if res.StatusCode != 200 || m["status"] != "ok" {
		t.Errorf("health: %d %v", res.StatusCode, m)
	}
	res, m = f.do("GET", "/v1/screen/login", "", "", "")
	if res.StatusCode != 200 || m["id"] != "login" || m["schema"] != float64(screen.SchemaVersion) {
		t.Errorf("login screen: %d %v", res.StatusCode, m)
	}
	if strings.Contains(fmtJSON(m), "Alice") {
		t.Errorf("login screen leaks customer data")
	}
}

func fmtJSON(m any) string { b, _ := json.Marshal(m); return string(b) }

func TestLoginAndScreens(t *testing.T) {
	f := newFixture(t)
	res, m := f.login("cust-001", "wrong")
	if res.StatusCode != http.StatusUnauthorized || m["error"] != "bad_credentials" || screenOf(m)["id"] != "login" {
		t.Fatalf("bad password: %d %v", res.StatusCode, m)
	}
	res, m = f.login("nobody", "password")
	if res.StatusCode != http.StatusUnauthorized || m["error"] != "bad_credentials" {
		t.Fatalf("unknown customer should look like bad password: %d %v", res.StatusCode, m)
	}

	res, m = f.login("cust-001", "password")
	if res.StatusCode != 200 {
		t.Fatalf("login: %d %v", res.StatusCode, m)
	}
	token, _ := m["token"].(string)
	if len(token) < 40 || strings.Contains(fmtJSON(m), "password") {
		t.Fatalf("bad token or password echoed: %v", m)
	}
	home := screenOf(m)
	if home["id"] != "accounts" || home["title"] != "Alice Example" {
		t.Errorf("login should return the accounts screen: %v", home)
	}
	body := home["body"].([]any)
	hero := body[0].(map[string]any)
	if hero["type"] != "hero" || !strings.HasPrefix(hero["value"].(string), "£") {
		t.Errorf("hero: %v", hero)
	}
	row := body[1].(map[string]any)
	if row["type"] != "row" || row["action"].(map[string]any)["screen"] != "/v1/screen/product/0" {
		t.Errorf("row: %v", row)
	}

	res, m = f.do("GET", "/v1/screen/activity", token, "", "")
	if res.StatusCode != 200 || m["id"] != "activity" {
		t.Errorf("activity: %d %v", res.StatusCode, m)
	}
	var sawHeading, sawTx bool
	for _, c := range m["body"].([]any) {
		cm := c.(map[string]any)
		switch cm["type"] {
		case "heading":
			sawHeading = true
		case "tx":
			sawTx = true
			v := cm["value"].(string)
			if !strings.HasPrefix(v, "+£") && !strings.HasPrefix(v, "-£") {
				t.Errorf("tx value must carry a sign: %q", v)
			}
		}
	}
	if !sawHeading || !sawTx {
		t.Errorf("activity should group tx rows under date headings: %v", m["body"])
	}

	res, m = f.do("GET", "/v1/screen/product/2?page=1", token, "", "")
	if res.StatusCode != 200 || m["id"] != "product" || m["subtitle"] != "Lending" || m["back"].(map[string]any)["screen"] != "/v1/screen/accounts" {
		t.Errorf("product: %d %v", res.StatusCode, m)
	}
	res, m = f.do("GET", "/v1/screen/product/9", token, "", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("product out of range: %d %v", res.StatusCode, m)
	}
	res, _ = f.do("GET", "/v1/screen/product/x", token, "", "")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("product non-numeric: %d", res.StatusCode)
	}

	res, m = f.do("POST", "/v1/logout", token, "", "")
	if res.StatusCode != 200 || screenOf(m)["id"] != "login" {
		t.Errorf("logout: %d %v", res.StatusCode, m)
	}
	res, _ = f.do("GET", "/v1/screen/accounts", token, "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("token still valid after logout: %d", res.StatusCode)
	}
}

func TestSessionIdleExpiry(t *testing.T) {
	f := newFixture(t)
	_, m := f.login("cust-002", "password")
	token := m["token"].(string)
	f.now = f.now.Add(4 * time.Minute)
	if res, _ := f.do("GET", "/v1/screen/accounts", token, "", ""); res.StatusCode != 200 {
		t.Fatalf("session should still be live: %d", res.StatusCode)
	}
	f.now = f.now.Add(6 * time.Minute)
	if res, _ := f.do("GET", "/v1/screen/accounts", token, "", ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("session should have idled out: %d", res.StatusCode)
	}
}

func TestLoginLockout(t *testing.T) {
	f := newFixture(t)
	for i := range 3 {
		if res, _ := f.login("cust-001", "wrong"); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i, res.StatusCode)
		}
	}
	res, m := f.login("cust-001", "password")
	if res.StatusCode != http.StatusTooManyRequests || m["error"] != "rate_limited" || res.Header.Get("Retry-After") == "" {
		t.Fatalf("should be locked even with the right password: %d %v", res.StatusCode, m)
	}
	if sc := screenOf(m); sc == nil || sc["id"] != "login" {
		t.Errorf("429 should carry the login screen with a notice: %v", m)
	}
	f.now = f.now.Add(11 * time.Minute)
	if res, _ := f.login("cust-001", "password"); res.StatusCode != 200 {
		t.Fatalf("lock should have expired: %d", res.StatusCode)
	}
}

func TestHTMLClient(t *testing.T) {
	f := newFixture(t)
	res, m := f.do("GET", "/v1/screen/login", "", "", "", "Accept", "text/html")
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") || !strings.Contains(m["_raw"].(string), `action="/v1/login"`) {
		t.Fatalf("html login screen: %d %v", res.StatusCode, res.Header)
	}
	if res.Header.Get("Content-Security-Policy") == "" {
		t.Errorf("html should carry a CSP")
	}
	// Browser GET of a protected screen redirects to login.
	res, _ = f.do("GET", "/v1/screen/accounts", "", "", "", "Accept", "text/html")
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/v1/screen/login" {
		t.Errorf("html unauthenticated: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	// Form login sets a cookie and redirects home.
	form := url.Values{"customer_id": {"cust-001"}, "password": {"password"}}.Encode()
	res, _ = f.do("POST", "/v1/login", "", "application/x-www-form-urlencoded", form, "Accept", "text/html")
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/v1/screen/accounts" {
		t.Fatalf("form login: %d %v", res.StatusCode, res.Header)
	}
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "mb_session" {
			cookie = c
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie: %+v", cookie)
	}
	res, m = f.do("GET", "/v1/screen/accounts", "", "", "", "Accept", "text/html", "Cookie", cookie.Name+"="+cookie.Value)
	if res.StatusCode != 200 || !strings.Contains(m["_raw"].(string), "Alice Example") {
		t.Errorf("html accounts via cookie: %d", res.StatusCode)
	}
}

func TestJourneys(t *testing.T) {
	bank := stubbank.New()
	j, err := bff.Journeys(t.Context(), bank, "cust-001")
	if err != nil {
		t.Fatal(err)
	}
	if len(j.Screens) != 6 { // login, accounts, activity, 3 products
		t.Errorf("screens = %d", len(j.Screens))
	}
	d2 := j.D2()
	for _, want := range []string{"v1_screen_login", "v1_screen_accounts", "v1_screen_product_n", "v1_login", "-> v1_screen_accounts: ok", "tab: Activity", "Log out"} {
		if !strings.Contains(d2, want) {
			t.Errorf("d2 missing %q", want)
		}
	}
}

func TestFormatMoney(t *testing.T) {
	cases := map[int64]string{0: "£0.00", 5: "£0.05", 123456789: "£1,234,567.89", -150: "-£1.50", 100000: "£1,000.00"}
	for in, want := range cases {
		if got := bff.FormatMoney(luca(in)); got != want {
			t.Errorf("FormatMoney(%d) = %q, want %q", in, got, want)
		}
	}
}
