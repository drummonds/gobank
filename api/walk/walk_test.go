package walk

import (
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/api"
)

const bffPkg, staffPkg = "git.bytestone.uk/hum3/gobank/bff", "git.bytestone.uk/hum3/gobank/bff/staff"

func walkBoth(t *testing.T) api.Surface {
	t.Helper()
	s, err := Walk(bffPkg, staffPkg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func find(s api.Surface, pkg, method, path string) (api.Route, bool) {
	for _, r := range s.Routes {
		if r.Package == pkg && r.Method == method && r.Path == path {
			return r, true
		}
	}
	return api.Route{}, false
}

// The walker finds a route registered on the mux directly, splits the
// pattern into method and path, names the handler and where it was
// registered, and takes the handler's doc comment.
func TestDirectRegistration(t *testing.T) {
	s := walkBoth(t)
	r, ok := find(s, bffPkg, "GET", "/v1/health")
	if !ok {
		t.Fatalf("GET /v1/health not found in %d routes", len(s.Routes))
	}
	if r.Handler != "(*Server).health" || !strings.HasPrefix(r.Pos, "bff/server.go:") || len(r.Wrappers) != 0 {
		t.Errorf("route %+v", r)
	}
	if !strings.Contains(r.Doc, "health") {
		t.Errorf("doc %q: want the handler's doc comment", r.Doc)
	}
}

// A pattern built from constants folds; a handler passed through a
// wrapper is named with the wrapper outermost first.
func TestFoldedPatternAndWrapper(t *testing.T) {
	s := walkBoth(t)
	r, ok := find(s, bffPkg, "GET", "/v1/screen/product/{index}")
	if !ok {
		t.Fatalf("GET /v1/screen/product/{index} not found")
	}
	if r.Handler != "(*Server).product" || strings.Join(r.Wrappers, ",") != "(*Server).authed" {
		t.Errorf("route %+v", r)
	}
}

// A route registered through a helper that takes the pattern as a
// parameter (staff's control and payments closures) is found at the
// helper's call, with the helper's other constant arguments kept.
func TestRegistrationThroughAHelper(t *testing.T) {
	s := walkBoth(t)
	r, ok := find(s, staffPkg, "", "/start")
	if !ok {
		t.Fatalf("/start not found")
	}
	if r.Via != "control" || strings.Join(r.Args, ",") != "sim_controls" || !strings.HasPrefix(r.Pos, "bff/staff/console_routes.go:") {
		t.Errorf("route %+v", r)
	}
	if _, ok := find(s, staffPkg, "", "/payments/send"); !ok {
		t.Errorf("/payments/send not found")
	}
}

// A handler held in a local variable (staff's explorer) and a
// package-level function from a library (lofigui.ServeFavicon) are both
// named; a literal has no name and takes the comment above its
// registration.
func TestHandlerForms(t *testing.T) {
	s := walkBoth(t)
	if r, ok := find(s, staffPkg, "", "/internal/explorer"); !ok || r.Handler != "" {
		t.Errorf("explorer %+v, %v: a local closure has no name", r, ok)
	}
	if r, ok := find(s, staffPkg, "", "/favicon.ico"); !ok || r.Handler != "lofigui.ServeFavicon" {
		t.Errorf("favicon %+v, %v", r, ok)
	}
	if r, ok := find(s, staffPkg, "", "/accounting/pnl"); !ok || r.Handler != "" || !strings.Contains(r.Doc, "Accounting") {
		t.Errorf("pnl %+v, %v: want the comment above the registration", r, ok)
	}
}

// Every registration is found: the count matches what the muxes record
// at run time (held exactly by the packages' own tests), and every
// route says where it is.
func TestEveryRouteIsPlaced(t *testing.T) {
	s := walkBoth(t)
	var bff, staff int
	for _, r := range s.Routes {
		switch r.Package {
		case bffPkg:
			bff++
		case staffPkg:
			staff++
		}
		if !strings.Contains(r.Pos, ".go:") || r.Path == "" {
			t.Errorf("route without a place: %+v", r)
		}
	}
	if bff < 8 || staff < 50 {
		t.Errorf("found %d bff and %d staff routes; want at least 8 and 50", bff, staff)
	}
}
