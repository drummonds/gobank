package walk

import (
	"slices"
	"strings"
	"testing"
)

// A wrapper the whole mux is passed through is a gate: the staff web's
// login, which reads the session cookie or the operator's bearer and lets
// a few paths through unread.
func TestGateInFrontOfTheMux(t *testing.T) {
	s := walkBoth(t)
	var found bool
	for _, g := range s.Gates {
		if g.Package != staffPkg {
			continue
		}
		found = true
		if g.Wrapper != "(*Site).requireLogin" {
			t.Errorf("wrapper %q", g.Wrapper)
		}
		if !slices.Contains(g.Reads.Cookies, "staff_session") || !slices.Contains(g.Reads.Headers, "Authorization") {
			t.Errorf("reads %+v", g.Reads)
		}
		for _, p := range []string{"/login", "/about.json", "/favicon.svg"} {
			if !slices.Contains(g.Open, p) {
				t.Errorf("open %v lacks %s", g.Open, p)
			}
		}
		if !strings.Contains(g.Doc, "session") {
			t.Errorf("doc %q", g.Doc)
		}
	}
	if !found {
		t.Fatal("no gate found for the staff web")
	}
	for _, g := range s.Gates {
		if g.Package == bffPkg {
			t.Errorf("the BFF has no gate in front of its mux, found %+v", g)
		}
	}
}

// A role check is the action it requires.
func TestRoleAction(t *testing.T) {
	s := walkBoth(t)
	for path, action := range map[string]string{"/start": "sim_controls", "/settings": "settings", "/export.goluca": "export", "/payments/send": "send_payment"} {
		if r := mustFind(t, s, staffPkg, "", path); r.Action != action {
			t.Errorf("%s action %q, want %q", path, r.Action, action)
		}
	}
}

// A static mount is files, not an unknown.
func TestStaticMount(t *testing.T) {
	r := mustFind(t, walkBoth(t), staffPkg, "", "/wasm/")
	if len(r.Unknowns) != 0 || !hasResponse(r, 200, "", "", "static files") {
		t.Errorf("wasm %+v %v", r.Responses, r.Unknowns)
	}
}

// The BFF's bodies are typed, so they have schemas.
func TestTypedBodies(t *testing.T) {
	r := mustFind(t, walkBoth(t), bffPkg, "POST", "/v1/login")
	if !hasResponse(r, 200, "application/json", "loginResponse", "") || !hasResponse(r, 400, "application/json", "errorBody", "bad_request") {
		t.Errorf("login %+v", r.Responses)
	}
}
