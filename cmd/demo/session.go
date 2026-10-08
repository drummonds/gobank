//go:build !(js && wasm)

package main

import "net/http"

// customerSessions is how the customer web keeps its session on a server:
// the BFF's own cookie, which the browser keeps.
func customerSessions(bff http.Handler) http.Handler { return bff }
