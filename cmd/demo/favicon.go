package main

import _ "embed"

// faviconSVG is the orange-bank site icon, shared by the demo server, the
// WASM demo page and the docs site (copied into docs/ by `task docs:build`).
//
//go:embed templates/favicon.svg
var faviconSVG []byte
