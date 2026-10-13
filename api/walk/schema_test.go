package walk

import (
	"slices"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/api"
)

func schema(t *testing.T, s api.Surface, name string) api.Schema {
	t.Helper()
	for _, x := range s.Types {
		if x.Name == name {
			return x
		}
	}
	t.Fatalf("no schema %q among %d", name, len(s.Types))
	return api.Schema{}
}

func field(t *testing.T, sc api.Schema, name string) api.Field {
	t.Helper()
	for _, f := range sc.Fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("%s has no field %q: %+v", sc.Name, name, sc.Fields)
	return api.Field{}
}

// Every type a body is made of has a schema, qualified by its package:
// a struct is an object with its JSON fields in order and their
// comments, a slice an array of its element, a pointer a nullable field.
func TestSchemasFollowTheTypes(t *testing.T) {
	s := walkBoth(t)
	screen := schema(t, s, "screen.Screen")
	if screen.Kind != "object" || screen.Package != "git.bytestone.uk/hum3/gobank/screen" {
		t.Errorf("screen %+v", screen)
	}
	if f := field(t, screen, "path"); f.Type != "string" || !strings.Contains(f.Doc, "GET path") {
		t.Errorf("path %+v: want the line comment", f)
	}
	if f := field(t, screen, "body"); f.Type != "[]screen.Component" {
		t.Errorf("body %+v", f)
	}
	if body := schema(t, s, "[]screen.Component"); body.Kind != "array" || body.Elem != "screen.Component" {
		t.Errorf("[]Component %+v", body)
	}
	if f := field(t, screen, "subtitle"); !f.Optional {
		t.Errorf("subtitle %+v: omitempty is optional", f)
	}
	if f := field(t, screen, "back"); !f.Nullable || !f.Optional || f.Type != "screen.Action" {
		t.Errorf("back %+v", f)
	}
	errorBody := schema(t, s, "bff.errorBody")
	if !strings.Contains(errorBody.Doc, "JSON client") || len(errorBody.Fields) != 3 {
		t.Errorf("errorBody %+v", errorBody)
	}
}

// A named string type with constants is an enum of their values; a
// time.Time is a date-time string; a named integer keeps its kind.
func TestEnumsAndScalars(t *testing.T) {
	s := walkBoth(t)
	tone := schema(t, s, "screen.Tone")
	if tone.Kind != "string" || !slices.Contains(tone.Enum, "savings") || !slices.Contains(tone.Enum, "") {
		t.Errorf("tone %+v", tone)
	}
	restart := schema(t, s, "staff.AboutRestart")
	if f := field(t, restart, "stopped_at"); f.Type != "time.Time" || !f.Nullable {
		t.Errorf("stopped_at %+v", f)
	}
	if tm := schema(t, s, "time.Time"); tm.Kind != "string" || tm.Format != "date-time" {
		t.Errorf("time %+v", tm)
	}
	about := schema(t, s, "staff.AboutStatus")
	if f := field(t, about, "restarts"); f.Type != "[]staff.AboutRestart" {
		t.Errorf("restarts %+v", f)
	}
}

// Body types are named by their package everywhere they appear.
func TestBodyTypesAreQualified(t *testing.T) {
	s := walkBoth(t)
	login := mustFind(t, s, bffPkg, "POST", "/v1/login")
	if login.Request.Type != "bff.loginRequest" || !hasResponse(login, 200, "application/json", "bff.loginResponse", "") {
		t.Errorf("login %+v %+v", login.Request, login.Responses)
	}
	about := mustFind(t, s, staffPkg, "", "/about.json")
	if !hasResponse(about, 200, "application/json", "staff.AboutStatus", "") {
		t.Errorf("about %+v", about.Responses)
	}
}

// The methods a handler tells apart are read off its checks of r.Method,
// through the page helper too.
func TestMethods(t *testing.T) {
	s := walkBoth(t)
	for path, want := range map[string]string{"/settings": "GET,POST", "/start": "POST", "/accounting/pnl": "GET", "/login": "GET,POST", "/view/{name}": ""} {
		r := mustFind(t, s, staffPkg, "", path)
		if got := strings.Join(r.Methods, ","); got != want {
			t.Errorf("%s methods %q, want %q", path, got, want)
		}
	}
}
