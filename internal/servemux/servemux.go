// Package servemux is net/http's ServeMux remembering what was
// registered on it: the patterns a server serves, read off the running
// mux rather than off a list kept beside it, so a test can hold the API
// document to exactly what is served.
package servemux

import "net/http"

// Recording is a ServeMux that keeps its patterns in registration order.
type Recording struct {
	*http.ServeMux
	Patterns []string

	// Observe, when set, is called with the pattern a request matched
	// before its handler runs, and what it returns after: how a server's
	// route figures are kept (internal/routestats) without a wrapper
	// between the mux and what serves it. A request no pattern matches
	// is served unobserved.
	Observe func(pattern string) (done func())
}

// New is an empty recording mux.
func New() *Recording { return &Recording{ServeMux: http.NewServeMux()} }

// ServeHTTP serves the request as the ServeMux does, observed when
// Observe is set.
func (m *Recording) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.Observe != nil {
		if _, pattern := m.Handler(r); pattern != "" {
			defer m.Observe(pattern)()
		}
	}
	m.ServeMux.ServeHTTP(w, r)
}

// Handle registers the handler for the pattern and records the pattern.
func (m *Recording) Handle(pattern string, handler http.Handler) {
	m.Patterns = append(m.Patterns, pattern)
	m.ServeMux.Handle(pattern, handler)
}

// HandleFunc registers the handler function for the pattern and records
// the pattern.
func (m *Recording) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	m.Patterns = append(m.Patterns, pattern)
	m.ServeMux.HandleFunc(pattern, handler)
}
