//go:build js && wasm

package main

import "time"

const runtimeEnv = "WebAssembly (WASM)"

// yieldToBrowser lets the browser's event loop run: the WASM host is
// single-threaded, so a long loop must give the page a moment now and
// then or it freezes. Call it every few dozen units of work.
func yieldToBrowser() { time.Sleep(time.Millisecond) }
