package api_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/api"
)

// A cluster is the registrar's subject: the doc's first words up to the
// colon, the rest its note.
func TestClusterIsTheRegistrarsSubject(t *testing.T) {
	reg := api.Registrar{Name: "(*Site).simulationRoutes", Doc: "Mounts Simulation: the dashboard and its controls."}
	title, note := reg.Cluster()
	if title != "Simulation" || note != "the dashboard and its controls." {
		t.Errorf("cluster %q, note %q", title, note)
	}
}

// Every route sits in exactly one cluster, and no two registrars share a
// cluster's name: the OpenAPI tags are global, so a shared name would
// merge two servers' sections.
func TestEveryRouteIsInACluster(t *testing.T) {
	s, err := api.Load()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, reg := range s.Registrars {
		title, _ := reg.Cluster()
		if title == "" || title == reg.Name {
			t.Errorf("%s %s has no cluster in its doc", reg.Package, reg.Name)
		}
		if other, dup := seen[title]; dup {
			t.Errorf("cluster %q is both %s and %s", title, other, reg.Name)
		}
		seen[title] = reg.Name
	}
	for _, r := range s.Routes {
		if s.Cluster(r) == "" {
			t.Errorf("%s %s is in no cluster (registered in %q)", r.Method, r.Path, r.Registrar)
		}
	}
}

// The Simulation cluster holds the simulation controls and nothing else:
// what gobank-deploy drives.
func TestSimulationClusterHoldsTheControls(t *testing.T) {
	s, err := api.Load()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range s.Routes {
		if s.Cluster(r) == "Simulation" {
			got = append(got, r.Path)
		}
	}
	want := []string{"/", "/add-customers", "/advance", "/dashboard/update", "/payments/run", "/payments/send", "/payments/stop",
		"/reset", "/settings", "/settings/status", "/start", "/stop"}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("Simulation holds %v, want %v", got, want)
	}
}

// The OpenAPI tags are the clusters, in the order they are registered,
// each operation tagged with its own and naming its package.
func TestTagsAreTheClusters(t *testing.T) {
	const pkg = "example.com/app/bff"
	s := api.Surface{
		Packages: []api.Package{{Path: pkg, Name: "bff", Doc: "The BFF."}},
		Registrars: []api.Registrar{
			{Package: pkg, Name: "healthRoutes", Doc: "Mounts Health: whether the server is up."},
			{Package: pkg, Name: "screenRoutes", Doc: "Mounts Screens: the trees the app renders."},
		},
		Routes: []api.Route{
			{Package: pkg, Method: "GET", Path: "/health", Registrar: "healthRoutes"},
			{Package: pkg, Method: "GET", Path: "/screen", Registrar: "screenRoutes"},
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
	var tags []string
	for _, tg := range d["tags"].([]any) {
		tags = append(tags, tg.(map[string]any)["name"].(string))
	}
	if !slices.Equal(tags, []string{"Health", "Screens"}) {
		t.Errorf("tags %v", tags)
	}
	op := d["paths"].(map[string]any)["/screen"].(map[string]any)["get"].(map[string]any)
	if op["tags"].([]any)[0] != "Screens" || op["x-package"] != "bff" {
		t.Errorf("op tags %v, package %v", op["tags"], op["x-package"])
	}
}

// A route's page names its cluster beside its server.
func TestDetailNamesTheCluster(t *testing.T) {
	s, err := api.Load()
	if err != nil {
		t.Fatal(err)
	}
	r, ok := api.Find(s, "any", "/start")
	if !ok {
		t.Fatal("no /start")
	}
	if page := string(api.Detail(s, r, api.Options{})); !strings.Contains(page, "<code>staff</code> › Simulation") {
		t.Errorf("detail lacks its cluster")
	}
}
