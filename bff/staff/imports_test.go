package staff

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// The staff web is a package of the BFF (ADR-0002 stage 6, story 1.6.3),
// built on the core alone: it reads the bank through core.StaffQueries
// and acts on it through core.Commands, and knows nothing else of it. It
// may use the UI libraries it renders with, the domain libraries the
// core's types come from, and the embedded ADRs; never a bank package or
// the demo.
func TestStaffWebKnowsOnlyTheCore(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no staff package here: %v", err)
	}
	allowed := []string{
		"git.bytestone.uk/hum3/gobank/core",
		"git.bytestone.uk/hum3/gobank/adr",
		"git.bytestone.uk/hum3/go-luca",
		"git.bytestone.uk/hum3/gobank-products",
		"git.bytestone.uk/hum3/lofigui",
		"git.bytestone.uk/hum3/gogal",
		"github.com/yuin/goldmark",
		"github.com/yuin/goldmark/extension",
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
				t.Errorf("%s imports %s; the staff web knows the bank only through core", name, path)
			}
		}
	}
}
