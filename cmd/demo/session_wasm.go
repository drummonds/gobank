//go:build js && wasm

package main

import "net/http"

// customerSessions is how the customer web keeps its session in the tab:
// the browser drops the cookie a service worker's response sets, so the
// tab keeps it and presents it on every request, as the browser would.
func customerSessions(bff http.Handler) http.Handler { return keepCookiesInTab(bff) }
