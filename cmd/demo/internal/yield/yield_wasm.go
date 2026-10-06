//go:build js && wasm

package yield

import "time"

// ToBrowser lets the browser's event loop run: the WASM host is
// single-threaded, so a long loop must give the page a moment now and then
// or it freezes.
func ToBrowser() { time.Sleep(time.Millisecond) }
