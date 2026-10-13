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
}

// New is an empty recording mux.
func New() *Recording { return &Recording{ServeMux: http.NewServeMux()} }

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
