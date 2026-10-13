package walk

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"git.bytestone.uk/hum3/gobank/api"
)

// gates finds the wrappers a package's mux is passed through: a call in
// the package with the mux itself as an argument, to a function the
// package has the body of. What the wrapper reads and the paths it lets
// through unread are read off that body and the bodies it calls.
func (w *walker) gates() []api.Gate {
	var out []api.Gate
	for _, file := range w.pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			f := w.callee(call)
			if f == nil || f.decl == nil {
				return true
			}
			for _, a := range call.Args {
				tv, ok := w.info.Types[a]
				if !ok || !embedsServeMux(tv.Type, 0) {
					continue
				}
				g := api.Gate{Package: w.pkg.PkgPath, Wrapper: f.name}
				if f.decl.Doc != nil {
					g.Doc = strings.TrimSpace(f.decl.Doc.Text())
				}
				s := &scanner{w: w, facts: &api.RouteFacts{}, visited: map[string]bool{}}
				s.body(f.body, env{}, 1, f.name)
				g.Reads = s.facts.Reads
				g.Open = w.openPaths(f, 0, map[*fn]bool{})
				slices.Sort(g.Open)
				out = append(out, g)
				break
			}
			return true
		})
	}
	return out
}

// openPaths is every constant path a function or the functions it calls
// compares a request against: the cases of a switch on the path, or an
// equality with it.
func (w *walker) openPaths(f *fn, depth int, seen map[*fn]bool) []string {
	if f == nil || seen[f] || depth > maxDepth {
		return nil
	}
	seen[f] = true
	var out []string
	add := func(e ast.Expr) {
		if tv, ok := w.info.Types[e]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
			if p := constant.StringVal(tv.Value); strings.HasPrefix(p, "/") && !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	ast.Inspect(f.body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CaseClause:
			for _, e := range n.List {
				add(e)
			}
		case *ast.BinaryExpr:
			if n.Op == token.EQL || n.Op == token.NEQ {
				add(n.X)
				add(n.Y)
			}
		case *ast.CallExpr:
			if callee := w.callee(n); callee != nil {
				for _, p := range w.openPaths(callee, depth+1, seen) {
					if !slices.Contains(out, p) {
						out = append(out, p)
					}
				}
			}
		}
		return true
	})
	return out
}

// isStatic says whether a handler expression serves files: a FileServer
// somewhere in it, however wrapped.
func (w *walker) isStatic(e ast.Expr) bool {
	static := false
	ast.Inspect(e, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id := calleeIdent(call); id != nil {
			if fobj, ok := w.info.Uses[id].(*types.Func); ok {
				switch fobj.FullName() {
				case "net/http.FileServer", "net/http.FileServerFS":
					static = true
				}
			}
		}
		return true
	})
	return static
}
