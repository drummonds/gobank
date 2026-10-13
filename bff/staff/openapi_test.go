package staff

import (
	"sort"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/api/walk"
)

// What the staff web registers at run time, console included, is exactly
// what the walker reads off its source.
func TestServedRoutesAreTheWalkedRoutes(t *testing.T) {
	site := New(Config{Version: "test", Users: testUsers, Console: &fakeConsole{}})
	surface, err := walk.Walk("git.bytestone.uk/hum3/gobank/bff/staff")
	if err != nil {
		t.Fatal(err)
	}
	var walked []string
	for _, r := range surface.Routes {
		walked = append(walked, strings.TrimSpace(r.Method+" "+r.Path))
	}
	served := append([]string(nil), site.mux.Patterns...)
	sort.Strings(walked)
	sort.Strings(served)
	if strings.Join(walked, "\n") != strings.Join(served, "\n") {
		t.Errorf("walked:\n%s\nserved:\n%s", strings.Join(walked, "\n"), strings.Join(served, "\n"))
	}
}

// About → API is a static index (no poll to close anything) whose rows
// link to a page per route, with the source linked at the build's tag;
// an unknown route is not found.
func TestAPIPages(t *testing.T) {
	site := New(Config{Scope: "/", Version: "v0.44.0", Users: testUsers, Console: &fakeConsole{}})
	cookie := signIn(t, site, "ro")
	rec := get(site, "/about/api", cookie)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `href="about/api/get/v1/health"`) || !strings.Contains(body, `href="about/api/any/settings"`) {
		t.Errorf("GET /about/api: %d, index lacks the detail links", rec.Code)
	}
	if !strings.Contains(body, "The simulation console") {
		t.Errorf("index lacks the console section")
	}
	if strings.Contains(body, "hx-trigger=\"every") && strings.Contains(body, `hx-get="about/api"`) {
		t.Errorf("the index polls itself")
	}
	rec = get(site, "/about/api/any/settings", cookie)
	body = rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "<code>/settings</code>") || !strings.Contains(body, "max_customers") {
		t.Errorf("GET /about/api/any/settings: %d", rec.Code)
	}
	if !strings.Contains(body, `https://git.bytestone.uk/hum3/gobank/src/tag/v0.44.0/bff/staff/console_routes.go#L`) {
		t.Errorf("detail lacks the source link at the tag")
	}
	if rec := get(site, "/about/api/get/no/such", cookie); rec.Code != 404 {
		t.Errorf("unknown route: %d, want 404", rec.Code)
	}
}
