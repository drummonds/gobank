//go:build js && wasm

package main

import "net/http"

// getSessionID is the one staff session of the tab. A service worker's
// response cannot set a cookie (the browser drops Set-Cookie on a
// synthetic response), and the tab holds the whole bank for one person
// anyway, so the role chosen and the PII authorisation hang off a fixed ID.
func getSessionID(http.ResponseWriter, *http.Request) string { return "tab" }

// customerSessions is how the customer web keeps its session in the tab:
// the browser drops the cookie a service worker's response sets, so the
// tab keeps it and presents it on every request, as the browser would.
func customerSessions(bff http.Handler) http.Handler { return keepCookiesInTab(bff) }
