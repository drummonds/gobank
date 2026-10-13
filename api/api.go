// Package api is the Model Bank's HTTP surface as one OpenAPI 3 document,
// read off the code: api/walk finds every route the servers register and
// what each handler does, OpenAPI renders it, and cmd/apidoc writes it
// here. The BFF serves the document at /v1/openapi.json. The test in this
// package regenerates it and fails when the committed document is behind
// the code, so it cannot drift.
package api

import _ "embed"

//go:generate go run ../cmd/apidoc -o openapi.json

// Document is the OpenAPI document as generated, served as it is.
//
//go:embed openapi.json
var Document []byte
