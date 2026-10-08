//go:build !(js && wasm)

package main

import (
	"fmt"
	"net/http"
	"time"
)

// getSessionID returns the staff session ID from a cookie, creating one if
// needed. The role chosen and the PII authorisation hang off it.
func getSessionID(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie("session_id")
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}
	id := fmt.Sprintf("sess-%d", time.Now().UnixNano())
	http.SetCookie(w, &http.Cookie{
		Name:  "session_id",
		Value: id,
		Path:  "/",
	})
	return id
}

// customerSessions is how the customer web keeps its session on a server:
// the BFF's own cookie, which the browser keeps.
func customerSessions(bff http.Handler) http.Handler { return bff }
