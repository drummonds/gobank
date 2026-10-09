package main

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// The bank's components are packages under bank/, and the bank itself is
// package bank (ADR-0002 stage 5). A component knows the core, the other
// bank packages, the root module's internal helpers and the domain
// libraries it is built on; it knows nothing of the UI, the simulation or
// the demo's wiring, which are in package main and cmd/demo/sim. The
// compiler keeps the demo out (a main package cannot be imported); this
// test keeps the rest out.
func TestBankPackagesKnowOnlyTheCoreAndEachOther(t *testing.T) {
	files, err := filepath.Glob("../../bank/*/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no bank packages at bank/: %v", err)
	}
	composite, _ := filepath.Glob("../../bank/*.go")
	files = append(files, composite...)
	allowed := []string{
		"git.bytestone.uk/hum3/gobank/core",
		"git.bytestone.uk/hum3/gobank/bank/",
		"git.bytestone.uk/hum3/gobank/internal/",
		"git.bytestone.uk/hum3/go-luca",
		"git.bytestone.uk/hum3/gobank-products",
		"git.bytestone.uk/hum3/gobanks-customers",
		"golang.org/x/crypto/argon2", // the users component's password hash
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
			if strings.HasSuffix(name, "_test.go") && path == "git.bytestone.uk/hum3/go-postgres" {
				continue // tests open a pglike database
			}
			ok := false
			for _, a := range allowed {
				if path == a || (strings.HasSuffix(a, "/") && strings.HasPrefix(path, a)) {
					ok = true
				}
			}
			if !ok {
				t.Errorf("%s imports %s; a bank package knows only the core, the other bank packages and the domain libraries", name, path)
			}
		}
	}
}
