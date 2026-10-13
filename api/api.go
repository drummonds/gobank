// Package api is the Model Bank's HTTP surface as one OpenAPI 3 document:
// the customer BFF under /v1/, the console routes the operator token
// reaches, /about.json and the health check, each with its auth and its
// errors. The document is written by hand and served at /v1/openapi.json;
// the routers' tests walk it, so a route missing from it, or one it names
// that is not served, fails the build.
package api

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// Document is the OpenAPI document, served as it is.
//
//go:embed openapi.json
var Document []byte

// Operation is one method on one path template of the document.
type Operation struct {
	Method, Path string
}

func (o Operation) String() string { return o.Method + " " + o.Path }

// Operations lists the document's operations, sorted.
func Operations() ([]Operation, error) {
	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(Document, &doc); err != nil {
		return nil, err
	}
	var ops []Operation
	for path, item := range doc.Paths {
		for key := range item {
			switch key {
			case "get", "post", "put", "patch", "delete", "head", "options":
				ops = append(ops, Operation{Method: strings.ToUpper(key), Path: path})
			}
		}
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].String() < ops[j].String() })
	return ops, nil
}

var pathParam = regexp.MustCompile(`\{[^}]+\}`)

// Example is a concrete path for a path template: every parameter
// replaced by a value, so a router can be asked which pattern serves it.
func Example(template string) string { return pathParam.ReplaceAllString(template, "1") }
