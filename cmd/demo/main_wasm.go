//go:build js && wasm

package main

import (
	"log"

	wasmhttp "github.com/nlepage/go-wasm-http-server/v2"

	"git.bytestone.uk/hum3/lofigui"
)

// The WASM build is the same demo served in the tab (ADR-0002 stage 6,
// story 1.6.1): a service worker (docs/demo/sw.js, written by lofigui's
// wasm-deploy) runs this binary and hands it every request under its
// scope, and the handler the server listens on answers them. The scope
// is the directory the worker was loaded from (/demo/ on the docs site),
// which the pages carry as their <base>.
func main() {
	state := NewDemoState()
	state.SetMemoryLimit(defaultMemoryLimit) // the browser-sized default and its GC headroom
	scope := lofigui.WASMScopePath()
	if _, err := wasmhttp.Serve(newHandler(state, version, scope)); err != nil {
		log.Fatal(err)
	}
	select {} // the Go runtime stays up to answer fetch events
}
