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
