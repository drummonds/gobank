package walk

import (
	"slices"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/api"
)

func mustFind(t *testing.T, s api.Surface, pkg, method, path string) api.Route {
	t.Helper()
	r, ok := find(s, pkg, method, path)
	if !ok {
		t.Fatalf("%s %s not found", method, path)
	}
	return r
}

func hasResponse(r api.Route, status int, media, typ, note string) bool {
	for _, x := range r.Responses {
		if x.Status == status && (media == "" || x.Media == media) && (typ == "" || x.Type == typ) && (note == "" || x.Note == note) {
			return true
		}
	}
	return false
}

func hasParam(r api.Route, name, in string) bool {
	for _, p := range r.Params {
		if p.Name == name && p.In == in {
			return true
		}
	}
	return false
}

// The login reads a JSON or form body, and answers with each error the
// code can write, named by its code, or a redirect to the home screen.
func TestLoginFacts(t *testing.T) {
	r := mustFind(t, walkBoth(t), bffPkg, "POST", "/v1/login")
	if r.Request == nil || r.Request.Type != "bff.loginRequest" {
		t.Errorf("request %+v", r.Request)
	}
	for _, want := range []struct {
		status int
		note   string
	}{{400, "bad_request"}, {401, "bad_credentials"}, {429, "rate_limited"}} {
		if !hasResponse(r, want.status, "application/json", "", want.note) {
			t.Errorf("no %d %s in %+v", want.status, want.note, r.Responses)
		}
	}
	redirected := false
	for _, x := range r.Responses {
		if x.Status == 303 && x.Redirect == "/v1/screen/accounts" {
			redirected = true
		}
	}
	if !redirected {
		t.Errorf("no 303 to the home screen in %+v", r.Responses)
	}
	if !hasParam(r, "customer_id", "form") || !hasParam(r, "password", "form") {
		t.Errorf("params %+v", r.Params)
	}
}

// A screen route answers the screen as JSON or HTML, reads its page
// parameter through a helper, and inherits the wrapper's 401.
func TestScreenFacts(t *testing.T) {
	r := mustFind(t, walkBoth(t), bffPkg, "GET", "/v1/screen/activity")
	if !hasResponse(r, 200, "application/json", "screen.Screen", "") {
		t.Errorf("no 200 JSON screen in %+v", r.Responses)
	}
	if !hasResponse(r, 200, "text/html; charset=utf-8", "", "") {
		t.Errorf("no 200 HTML in %+v", r.Responses)
	}
	if !hasResponse(r, 401, "", "", "unauthenticated") {
		t.Errorf("no 401 from the wrapper in %+v", r.Responses)
	}
	if !hasResponse(r, 500, "", "", "internal") {
		t.Errorf("no 500 in %+v", r.Responses)
	}
	if !hasParam(r, "page", "query") {
		t.Errorf("params %+v", r.Params)
	}
	if strings.Join(r.Guards.Cookies, ",") != "mb_session" || !slices.Contains(r.Guards.Headers, "Authorization") {
		t.Errorf("guards %+v: want the wrapper's cookie and bearer", r.Guards)
	}
	if len(r.Reads.Cookies) != 0 {
		t.Errorf("reads %+v: the handler itself reads no cookie", r.Reads)
	}
}

// A console control registered through a helper redirects when done
// and refuses a role that may not; the settings post reads its form.
func TestConsoleFacts(t *testing.T) {
	s := walkBoth(t)
	start := mustFind(t, s, staffPkg, "", "/start")
	if !hasResponse(start, 303, "", "", "") || !hasResponse(start, 403, "text/plain; charset=utf-8", "", "Forbidden") {
		t.Errorf("start %+v", start.Responses)
	}
	settings := mustFind(t, s, staffPkg, "", "/settings")
	for _, p := range []string{"max_customers", "payments_per_day", "day_length"} {
		if !hasParam(settings, p, "form") {
			t.Errorf("settings params %+v lack %s", settings.Params, p)
		}
	}
	about := mustFind(t, s, staffPkg, "", "/about.json")
	if !hasResponse(about, 200, "application/json", "staff.AboutStatus", "") || !hasResponse(about, 405, "", "", "Method not allowed") {
		t.Errorf("about %+v", about.Responses)
	}
	product := mustFind(t, s, bffPkg, "GET", "/v1/screen/product/{index}")
	if !hasParam(product, "index", "path") {
		t.Errorf("product params %+v", product.Params)
	}
}

// A handler the walk cannot see into is said so, not passed over.
func TestUnknownsAreNamed(t *testing.T) {
	r := mustFind(t, walkBoth(t), staffPkg, "", "/favicon.ico")
	if len(r.Unknowns) == 0 || !strings.Contains(r.Unknowns[0], "lofigui.ServeFavicon") {
		t.Errorf("unknowns %v", r.Unknowns)
	}
}
