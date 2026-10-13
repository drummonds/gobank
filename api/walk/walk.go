// Package walk reads the HTTP surface off the code. It loads packages
// with go/packages and finds every registration on a ServeMux: a call to
// HandleFunc or Handle on net/http's mux or one that embeds it, or a
// call to any function that passes one of its own parameters on as the
// pattern (a registration helper, to any depth). Patterns are folded from
// constants, so "GET "+PathScreenLogin is found as it is served, and the
// handler is named through the wrappers it was passed through. Nothing is
// declared for the walker's benefit: what it finds is what the servers
// register, which is why the API document rendered from it cannot drift.
package walk

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"

	"git.bytestone.uk/hum3/gobank/api"
)

// Walk loads the packages the patterns name and returns every route they
// register, sorted by package, path and method.
func Walk(patterns ...string) (api.Surface, error) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
		packages.NeedImports | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return api.Surface{}, fmt.Errorf("walk: %w", err)
	}
	var s api.Surface
	sch := newSchemas(newLoader(pkgs))
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			return api.Surface{}, fmt.Errorf("walk: %s: %v", p.PkgPath, p.Errors[0])
		}
		w := newWalker(p, sch)
		s.Packages = append(s.Packages, api.Package{Path: p.PkgPath, Name: p.Name, Doc: w.packageDoc()})
		s.Routes = append(s.Routes, w.routes()...)
		s.Gates = append(s.Gates, w.gates()...)
	}
	s.Types = sch.build()
	sort.Slice(s.Routes, func(i, j int) bool {
		a, b := s.Routes[i], s.Routes[j]
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Method < b.Method
	})
	return s, nil
}

// fn is a function body the walker reasons about: a declaration or a
// literal, with the parameters a registration helper would pass on.
type fn struct {
	name   string // "(*Site).routes", "control" for a closure held in a variable, "" for an anonymous literal
	params []*types.Var
	body   *ast.BlockStmt
	decl   *ast.FuncDecl // nil for a literal
}

type walker struct {
	pkg      *packages.Package
	fset     *token.FileSet
	info     *types.Info
	comments map[*ast.File]ast.CommentMap
	fns      map[ast.Node]*fn     // every FuncDecl and FuncLit, by its node
	byObj    map[types.Object]*fn // a declared function or method, or a variable holding a literal
	helpers  map[*fn]int          // registration helpers → the index of the pattern parameter
	modDir   string
	schemas  *schemas
}

func newWalker(p *packages.Package, sch *schemas) *walker {
	w := &walker{pkg: p, fset: p.Fset, info: p.TypesInfo, schemas: sch,
		comments: map[*ast.File]ast.CommentMap{}, fns: map[ast.Node]*fn{}, byObj: map[types.Object]*fn{}, helpers: map[*fn]int{}}
	if p.Module != nil {
		w.modDir = p.Module.Dir
	}
	for _, file := range p.Syntax {
		w.comments[file] = ast.NewCommentMap(w.fset, file, file.Comments)
		w.index(file)
	}
	w.findHelpers()
	return w
}

// index records every function body in the file and what names it.
func (w *walker) index(file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			if n.Body == nil {
				return true
			}
			obj, _ := w.info.Defs[n.Name].(*types.Func)
			f := &fn{name: w.funcName(obj), params: w.params(n.Type), body: n.Body, decl: n}
			w.fns[n] = f
			if obj != nil {
				w.byObj[obj] = f
			}
		case *ast.FuncLit:
			if _, named := w.fns[n]; !named { // a literal held in a variable was indexed at the assignment
				w.fns[n] = &fn{params: w.params(n.Type), body: n.Body}
			}
		case *ast.AssignStmt:
			// name := func(...) {...}  — a closure held in a variable
			for i, rhs := range n.Rhs {
				lit, ok := rhs.(*ast.FuncLit)
				if !ok || i >= len(n.Lhs) {
					continue
				}
				id, ok := n.Lhs[i].(*ast.Ident)
				if !ok {
					continue
				}
				obj := w.info.Defs[id]
				if obj == nil {
					obj = w.info.Uses[id]
				}
				if obj != nil {
					f := &fn{name: id.Name, params: w.params(lit.Type), body: lit.Body}
					w.fns[lit] = f
					w.byObj[obj] = f
				}
			}
		}
		return true
	})
}

func (w *walker) params(t *ast.FuncType) []*types.Var {
	var out []*types.Var
	if t.Params == nil {
		return nil
	}
	for _, field := range t.Params.List {
		for _, name := range field.Names {
			if v, ok := w.info.Defs[name].(*types.Var); ok {
				out = append(out, v)
			}
		}
	}
	return out
}

// findHelpers marks, to a fixpoint, every function that registers one of
// its own parameters as a pattern — on the mux or through another helper.
func (w *walker) findHelpers() {
	for changed := true; changed; {
		changed = false
		for _, f := range w.fns {
			if _, done := w.helpers[f]; done {
				continue
			}
			ast.Inspect(f.body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				patternExpr, _, ok := w.registration(call)
				if !ok {
					return true
				}
				id, ok := patternExpr.(*ast.Ident)
				if !ok {
					return true
				}
				v, _ := w.info.Uses[id].(*types.Var)
				for i, p := range f.params {
					if p == v {
						w.helpers[f] = i
						changed = true
					}
				}
				return true
			})
		}
	}
}

// registration says whether a call registers a route, and which of its
// arguments are the pattern and (for a mux call) the handler. A call to a
// helper is a registration with no handler argument of its own.
func (w *walker) registration(call *ast.CallExpr) (pattern, handler ast.Expr, ok bool) {
	if w.isMuxRegistration(call) && len(call.Args) >= 2 {
		return call.Args[0], call.Args[1], true
	}
	if f := w.callee(call); f != nil {
		if i, ok := w.helpers[f]; ok && i < len(call.Args) {
			return call.Args[i], nil, true
		}
	}
	return nil, nil, false
}

// isMuxRegistration is a call to Handle or HandleFunc on net/http's
// ServeMux, a type embedding it, or the package-level DefaultServeMux
// functions.
func (w *walker) isMuxRegistration(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") {
		return false
	}
	obj, _ := w.info.Uses[sel.Sel].(*types.Func)
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	sig := obj.Type().(*types.Signature)
	if sig.Recv() == nil {
		return obj.Pkg().Path() == "net/http"
	}
	return embedsServeMux(sig.Recv().Type(), 0)
}

func embedsServeMux(t types.Type, depth int) bool {
	if depth > 3 {
		return false
	}
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	if named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "net/http" && named.Obj().Name() == "ServeMux" {
		return true
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for i := range st.NumFields() {
		if f := st.Field(i); f.Embedded() && embedsServeMux(f.Type(), depth+1) {
			return true
		}
	}
	return false
}

// callee is the function a call reaches when it is one the walker has a
// body for: a declaration, a method, or a closure held in a variable.
func (w *walker) callee(call *ast.CallExpr) *fn {
	var id *ast.Ident
	switch f := call.Fun.(type) {
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		id = f.Sel
	default:
		return nil
	}
	if obj := w.info.Uses[id]; obj != nil {
		return w.byObj[obj]
	}
	return nil
}

// routes finds every registration with a constant pattern.
func (w *walker) routes() []api.Route {
	var out []api.Route
	for _, file := range w.pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			patternExpr, handlerExpr, ok := w.registration(call)
			if !ok {
				return true
			}
			pattern, ok := w.constString(patternExpr)
			if !ok {
				return true // inside a helper: found at the helper's callers
			}
			r := api.Route{Package: w.pkg.PkgPath, Pos: w.pos(call.Pos())}
			r.Method, r.Path = splitPattern(pattern)
			var helper *fn
			if handlerExpr != nil {
				var lit *ast.FuncLit
				r.Handler, r.Wrappers, lit = w.handler(handlerExpr)
				r.Doc = w.docOf(r.Handler, lit)
			} else {
				helper = w.callee(call)
				r.Via = helper.name
				for i, a := range call.Args {
					if i == w.helpers[helper] {
						continue
					}
					if s, ok := w.constString(a); ok {
						r.Args = append(r.Args, s)
					}
				}
			}
			r.RouteFacts = w.facts(call, handlerExpr, helper)
			if r.Doc == "" {
				r.Doc = w.commentAbove(file, call)
			}
			out = append(out, r)
			return true
		})
	}
	return out
}

// handler names the function a handler expression reaches, through any
// wrappers (outermost first) and conversions; a literal has no name.
func (w *walker) handler(e ast.Expr) (name string, wrappers []string, lit *ast.FuncLit) {
	switch e := e.(type) {
	case *ast.FuncLit:
		return "", nil, e
	case *ast.ParenExpr:
		return w.handler(e.X)
	case *ast.Ident:
		switch obj := w.info.Uses[e].(type) {
		case *types.Func:
			return w.funcName(obj), nil, nil
		case *types.Var:
			if f := w.byObj[obj]; f != nil {
				return "", nil, nil // a closure held in a variable
			}
			return e.Name, nil, nil
		}
	case *ast.SelectorExpr:
		if obj, ok := w.info.Uses[e.Sel].(*types.Func); ok {
			return w.funcName(obj), nil, nil
		}
	case *ast.CallExpr:
		if tv, ok := w.info.Types[e.Fun]; ok && tv.IsType() && len(e.Args) == 1 {
			return w.handler(e.Args[0]) // a conversion such as http.HandlerFunc(f)
		}
		wrapper := ""
		if f := w.callee(e); f != nil {
			wrapper = f.name
		} else if id := calleeIdent(e); id != nil {
			if obj, ok := w.info.Uses[id].(*types.Func); ok {
				wrapper = w.funcName(obj)
			}
		}
		for _, a := range e.Args {
			if tv, ok := w.info.Types[a]; ok {
				if _, isFunc := tv.Type.Underlying().(*types.Signature); isFunc {
					name, inner, lit := w.handler(a)
					return name, append([]string{wrapper}, inner...), lit
				}
			}
		}
		return "", []string{wrapper}, nil
	}
	return "", nil, nil
}

func calleeIdent(call *ast.CallExpr) *ast.Ident {
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return f
	case *ast.SelectorExpr:
		return f.Sel
	}
	return nil
}

// funcName is "(*T).m" for a method, "pkg.f" for another package's
// function, "f" for this package's.
func (w *walker) funcName(obj *types.Func) string {
	if obj == nil {
		return ""
	}
	sig := obj.Type().(*types.Signature)
	if recv := sig.Recv(); recv != nil {
		return "(" + types.TypeString(recv.Type(), func(*types.Package) string { return "" }) + ")." + obj.Name()
	}
	if obj.Pkg() != nil && obj.Pkg() != w.pkg.Types {
		return obj.Pkg().Name() + "." + obj.Name()
	}
	return obj.Name()
}

// docOf is the doc comment of a named handler declared in this package.
func (w *walker) docOf(name string, lit *ast.FuncLit) string {
	if name == "" {
		return ""
	}
	for _, f := range w.fns {
		if f.decl != nil && f.name == name && f.decl.Doc != nil {
			return strings.TrimSpace(f.decl.Doc.Text())
		}
	}
	return ""
}

// commentAbove is the comment group on the statement that makes the
// registration, when there is one.
func (w *walker) commentAbove(file *ast.File, call *ast.CallExpr) string {
	path, _ := astutil.PathEnclosingInterval(file, call.Pos(), call.End())
	for _, n := range path {
		stmt, ok := n.(ast.Stmt)
		if !ok {
			continue
		}
		var parts []string
		for _, g := range w.comments[file][stmt] {
			if g.End() <= stmt.Pos() {
				parts = append(parts, strings.TrimSpace(g.Text()))
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func (w *walker) constString(e ast.Expr) (string, bool) {
	tv, ok := w.info.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

func (w *walker) pos(p token.Pos) string {
	pos := w.fset.Position(p)
	file := pos.Filename
	if w.modDir != "" {
		if rel, err := filepath.Rel(w.modDir, file); err == nil {
			file = filepath.ToSlash(rel)
		}
	}
	return fmt.Sprintf("%s:%d", file, pos.Line)
}

// splitPattern separates a ServeMux pattern's method from its path; a
// host prefix on the path is kept as served.
func splitPattern(pattern string) (method, path string) {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok {
		return "", pattern
	}
	return method, strings.TrimSpace(path)
}

// Packages are the packages that serve HTTP: what the API document is
// walked from.
var Packages = []string{"git.bytestone.uk/hum3/gobank/bff", "git.bytestone.uk/hum3/gobank/bff/staff"}

// packageDoc is the package's doc comment, from whichever file carries it.
func (w *walker) packageDoc() string {
	for _, file := range w.pkg.Syntax {
		if file.Doc != nil {
			return strings.TrimSpace(file.Doc.Text())
		}
	}
	return ""
}
