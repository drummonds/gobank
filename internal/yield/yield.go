//go:build !(js && wasm)

// Package yield lets a long loop give the browser's event loop a moment in
// the WASM build, where the host is single-threaded; on a native host it
// does nothing. Both the bank's daily pass and the simulation's generators
// call it every few dozen units of work.
package yield

// ToBrowser does nothing on a native host: the runtime has threads and the
// HTTP server is never starved by the simulation.
func ToBrowser() {}
