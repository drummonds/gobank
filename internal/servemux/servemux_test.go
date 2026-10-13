package servemux

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// A recording mux serves like a ServeMux and remembers every pattern
// registered on it, in order, whichever way it was registered.
func TestRecordsEveryPattern(t *testing.T) {
	m := New()
	m.HandleFunc("GET /a", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(201) })
	m.Handle("/b/{id}", http.NotFoundHandler())
	if want := []string{"GET /a", "/b/{id}"}; !reflect.DeepEqual(m.Patterns, want) {
		t.Errorf("patterns %v, want %v", m.Patterns, want)
	}
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest("GET", "/a", nil))
	if rec.Code != 201 {
		t.Errorf("GET /a served %d", rec.Code)
	}
}
