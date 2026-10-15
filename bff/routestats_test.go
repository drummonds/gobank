package bff

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"git.bytestone.uk/hum3/gobank/internal/routestats"
)

// The customer BFF times its routes through the stats it is given, by
// the pattern each request matched.
func TestServerRecordsItsRoutes(t *testing.T) {
	st := routestats.New()
	s := NewServer(Config{Stats: st})
	for range 2 {
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, httptest.NewRequest("GET", "/v1/health", nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("health: %d", rr.Code)
		}
	}
	snap := st.Snapshot()
	if len(snap) != 1 || snap[0].Pattern != "GET /v1/health" || snap[0].Count != 2 {
		t.Errorf("snapshot = %+v; want GET /v1/health served twice", snap)
	}
}
