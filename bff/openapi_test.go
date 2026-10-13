package bff

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/api/walk"
)

// What the server registers at run time is exactly what the walker reads
// off its source, so the API document rendered from the walk cannot name
// a route that is not served or miss one that is.
func TestServedRoutesAreTheWalkedRoutes(t *testing.T) {
	s := NewServer(Config{})
	surface, err := walk.Walk("git.bytestone.uk/hum3/gobank/bff")
	if err != nil {
		t.Fatal(err)
	}
	var walked []string
	for _, r := range surface.Routes {
		walked = append(walked, strings.TrimSpace(r.Method+" "+r.Path))
	}
	served := append([]string(nil), s.mux.Patterns...)
	sort.Strings(walked)
	sort.Strings(served)
	if strings.Join(walked, "\n") != strings.Join(served, "\n") {
		t.Errorf("walked:\n%s\nserved:\n%s", strings.Join(walked, "\n"), strings.Join(served, "\n"))
	}
}

// /v1/openapi.json is served without a session and is an OpenAPI 3
// document.
func TestOpenAPIIsServedOpen(t *testing.T) {
	s := NewServer(Config{})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/openapi.json", nil)
	r.Header.Set("Accept", "text/html")
	s.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("GET /v1/openapi.json: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	var doc struct {
		OpenAPI string `json:"openapi"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil || !strings.HasPrefix(doc.OpenAPI, "3.") {
		t.Fatalf("body is not an OpenAPI 3 document: %v %q", err, doc.OpenAPI)
	}
}
