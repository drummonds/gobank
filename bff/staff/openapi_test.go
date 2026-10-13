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

// About → API renders the generated surface for a signed-in user.
func TestAPIPage(t *testing.T) {
	site := newSite(t, "/")
	rec := get(site, "/about/api", signIn(t, site, "ro"))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<code>/v1/openapi.json</code>") || !strings.Contains(rec.Body.String(), "<code>/about/api</code>") {
		t.Errorf("GET /about/api: %d, page lacks the routes", rec.Code)
	}
}
