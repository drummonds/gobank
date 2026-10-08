//go:build js && wasm

package staff

import "net/http"

// sessionID is the one staff session of the tab. A service worker's
// response cannot set a cookie (the browser drops Set-Cookie on a
// synthetic response), and the tab holds the whole bank for one person
// anyway, so the role chosen and the PII authorisation hang off a fixed ID.
func sessionID(http.ResponseWriter, *http.Request) string { return "tab" }
