//go:build !(js && wasm)

package main

const runtimeEnv = "HTTP Server"

// yieldToBrowser does nothing on a native host: the runtime has threads
// and the HTTP server is never starved by the simulation.
func yieldToBrowser() {}
