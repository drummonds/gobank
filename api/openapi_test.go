package api_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/api"
	"git.bytestone.uk/hum3/gobank/api/walk"
)

// The committed document is the one the code renders today; when it is
// not, the fix is `go generate ./api`.
func TestDocumentIsCurrent(t *testing.T) {
	s, err := walk.Walk(walk.Packages...)
	if err != nil {
		t.Fatal(err)
	}
	got, err := api.OpenAPI(s)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(api.Document)) {
		t.Fatal("api/openapi.json is behind the code: run `go generate ./api`")
	}
	sj, _ := json.MarshalIndent(s, "", "  ")
	if !bytes.Equal(bytes.TrimSpace(sj), bytes.TrimSpace(api.SurfaceJSON)) {
		t.Fatal("api/surface.json is behind the code: run `go generate ./api`")
	}
}

// The page renders every route and type of the generated surface, each
// route with what it needs and answers.
func TestHTMLPage(t *testing.T) {
	s, err := api.Load()
	if err != nil {
		t.Fatal(err)
	}
	page := string(api.HTML(s))
	for _, want := range []string{
		"<code>/v1/login</code>", "<code>/about.json</code>", "<code>/settings</code>",
		`id="type-screen-Screen"`, `href="#type-screen-Component"`,
		"cookie staff_session", "bearer token", "nothing: open",
		"<code>sim_controls</code>",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(page, "api: ") && strings.Contains(page, "template") {
		t.Errorf("template error in page")
	}
}

// A route renders as an operation under its method with its source, its
// security from what the gate and wrappers read, its responses by status
// with the schema of what each encodes; array types inline over their
// element, named types are components.
func TestRenderingOfOneRoute(t *testing.T) {
	s := api.Surface{
		Packages: []api.Package{{Path: "example.com/app/bff", Name: "bff", Doc: "The BFF."}},
		Gates:    []api.Gate{{Package: "example.com/app/bff", Wrapper: "gate", Reads: api.Reads{Cookies: []string{"sid"}}, Open: []string{"/open"}}},
		Routes: []api.Route{
			{Package: "example.com/app/bff", Method: "GET", Path: "/things/{id}", Pos: "bff/x.go:10", Handler: "(*S).thing", Doc: "thing is one thing.\nMore.",
				RouteFacts: api.RouteFacts{
					Params: []api.Param{{Name: "page", In: "query"}},
					Guards: api.Reads{Headers: []string{"Authorization"}},
					Responses: []api.Response{
						{Status: 200, Media: "application/json", Type: "bff.Thing"},
						{Status: 404, Media: "application/json", Type: "bff.Err", Note: "not_found"},
						{Status: 303, Redirect: "/login"},
					},
				}},
			{Package: "example.com/app/bff", Path: "/open", Pos: "bff/x.go:20", RouteFacts: api.RouteFacts{Responses: []api.Response{{Status: 200, Media: "text/html; charset=utf-8"}}}},
		},
		Types: []api.Schema{
			{Name: "bff.Thing", Package: "example.com/app/bff", Kind: "object", Doc: "A thing.", Fields: []api.Field{
				{Name: "id", Type: "string", Doc: "the id"}, {Name: "tags", Type: "[]string", Optional: true}, {Name: "when", Type: "time.Time", Nullable: true}}},
			{Name: "[]string", Kind: "array", Elem: "string"},
			{Name: "time.Time", Package: "time", Kind: "string", Format: "date-time"},
			{Name: "bff.Err", Package: "example.com/app/bff", Kind: "object", Fields: []api.Field{{Name: "error", Type: "string"}}},
		},
	}
	out, err := api.OpenAPI(s)
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatal(err)
	}
	paths := d["paths"].(map[string]any)
	op := paths["/things/{id}"].(map[string]any)["get"].(map[string]any)
	if op["summary"] != "thing is one thing" || op["x-source"] != "bff/x.go:10" {
		t.Errorf("op %v", op)
	}
	params := op["parameters"].([]any)
	if len(params) != 2 || params[0].(map[string]any)["in"] != "path" || params[1].(map[string]any)["name"] != "page" {
		t.Errorf("params %v", params)
	}
	sec := op["security"].([]any)
	if len(sec) != 2 { // the gate's cookie and the route's bearer
		t.Errorf("security %v", sec)
	}
	resp := op["responses"].(map[string]any)
	ok := resp["200"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	if ok["$ref"] != "#/components/schemas/bff.Thing" {
		t.Errorf("200 schema %v", ok)
	}
	if desc := resp["404"].(map[string]any)["description"]; desc != "not_found" {
		t.Errorf("404 %v", desc)
	}
	if desc := resp["303"].(map[string]any)["description"]; !strings.Contains(desc.(string), "/login") {
		t.Errorf("303 %v", desc)
	}
	open := paths["/open"].(map[string]any)["get"].(map[string]any)
	if len(open["security"].([]any)) != 0 || open["x-methods"] != "any" {
		t.Errorf("open %v", open)
	}
	schemas := d["components"].(map[string]any)["schemas"].(map[string]any)
	thing := schemas["bff.Thing"].(map[string]any)
	props := thing["properties"].(map[string]any)
	if props["tags"].(map[string]any)["type"] != "array" {
		t.Errorf("tags %v: an array inlines", props["tags"])
	}
	if _, inlined := schemas["[]string"]; inlined {
		t.Error("[]string is not a component")
	}
	if _, ok := props["when"].(map[string]any)["anyOf"]; !ok {
		t.Errorf("when %v: a nullable reference is anyOf with null", props["when"])
	}
	if req := thing["required"].([]any); len(req) != 2 {
		t.Errorf("required %v: id and when", req)
	}
	if schemes := d["components"].(map[string]any)["securitySchemes"].(map[string]any); schemes["sid"] == nil || schemes["bearer"] == nil {
		t.Errorf("schemes %v", schemes)
	}
}
