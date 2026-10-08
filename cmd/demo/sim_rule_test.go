package main

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// The simulation is a package of its own (ADR-0002 stage 4, story c),
// built on the core alone: it drives the bank through core.Commands and
// reads it through core.StaffQueries, and knows nothing else of it. The
// compiler enforces that it cannot reach the bank's internals (they are in
// package main); this test holds its imports to the core, the standard
// library, go-luca (the core's money type) and the demo's yield helper.
func TestSimulationImportsOnlyTheCore(t *testing.T) {
	files, err := filepath.Glob("sim/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no simulation package at sim/: %v", err)
	}
	allowed := []string{
		"git.bytestone.uk/hum3/gobank/core",
		"git.bytestone.uk/hum3/go-luca",
		"git.bytestone.uk/hum3/gobank/internal/yield",
		"git.bytestone.uk/hum3/gobank/internal/daylength",
	}
	fset := token.NewFileSet()
	for _, name := range files {
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if !strings.Contains(strings.SplitN(path, "/", 2)[0], ".") {
				continue // standard library
			}
			ok := false
			for _, a := range allowed {
				if path == a {
					ok = true
				}
			}
			if !ok {
				t.Errorf("%s imports %s; the simulation knows the bank only through core", name, path)
			}
		}
	}
}
