package staff

import (
	"net/http"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/internal/routestats"
)

// The staff web times its routes through the stats it is given, and the
// API pages show what the running process has measured: the index a
// requests and p95 column per route, the detail page the route's
// performance and capacity.
func TestAPIPagesShowEachRoutesPerformance(t *testing.T) {
	st := routestats.New()
	site := New(Config{Version: "test", Users: testUsers, Stats: st})
	admin := signIn(t, site, "admin")
	for range 3 {
		if rr := get(site, "/about", admin); rr.Code != http.StatusOK {
			t.Fatalf("about: %d", rr.Code)
		}
	}
	if s := st.Snapshot(); len(s) == 0 {
		t.Fatal("nothing recorded")
	}

	rr := get(site, "/about/api", admin)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), ">Requests</th>") {
		t.Errorf("index: %d\n%s", rr.Code, rr.Body.String())
	}
	rr = get(site, "/about/api/any/about", admin)
	body := rr.Body.String()
	if rr.Code != http.StatusOK || !strings.Contains(body, "Performance") || !strings.Contains(body, "3 requests") {
		t.Errorf("detail: %d\n%s", rr.Code, body)
	}
}

// The figures go out at /about.json too, a row per route, for a program
// to read beside the rates.
func TestAboutJSONCarriesTheRoutes(t *testing.T) {
	st := routestats.New()
	st.Record("GET /v1/health", 0)
	about := AboutStatus{Routes: AboutRoutes(st)}
	if len(about.Routes) != 1 || about.Routes[0].Pattern != "GET /v1/health" || about.Routes[0].Count != 1 || about.Routes[0].P95 != "0s" {
		t.Errorf("routes = %+v", about.Routes)
	}
	if AboutRoutes(nil) != nil {
		t.Error("no stats, no routes")
	}
}
