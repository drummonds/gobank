package walk

import (
	"go/ast"
	"go/constant"
	"go/types"
	"slices"
	"strings"

	"git.bytestone.uk/hum3/gobank/api"
)

// The facts of a route are read off the handler's body and, within the
// package, the bodies it calls, with the constants it passes bound to
// their parameters: writeScreen(w, r, http.StatusOK, sc) is scanned
// knowing status is 200. Only the primitives say anything — a status
// written, a Content-Type set, a value encoded, a form value read — so
// a helper of any shape is understood through what it finally does.
// Branches are not followed: every fact is something the handler can do.

// maxDepth bounds the calls followed from the handler.
const maxDepth = 6

// env binds a function's parameters to the constants its caller passed,
// and carries the note (the first constant string passed beside a status)
// a response written inside takes.
type env struct {
	bound map[*types.Var]constant.Value
	note  string
}

// scanner gathers one route's facts.
type scanner struct {
	w       *walker
	facts   *api.RouteFacts
	visited map[string]bool
	guard   bool // scanning a wrapper: what it reads is the route's security
}

// facts reads the route's facts from the handler expression it was
// registered with, or from the helper it was registered through.
func (w *walker) facts(call *ast.CallExpr, handlerExpr ast.Expr, helper *fn) api.RouteFacts {
	s := &scanner{w: w, facts: &api.RouteFacts{}, visited: map[string]bool{}}
	switch {
	case helper != nil:
		e := env{bound: map[*types.Var]constant.Value{}}
		for i, a := range call.Args {
			if i < len(helper.params) {
				if v, ok := w.constValue(a, e); ok {
					e.bound[helper.params[i]] = v
				}
			}
			if lit, ok := a.(*ast.FuncLit); ok {
				s.body(lit.Body, env{}, 1, "")
			}
		}
		s.body(helper.body, e, 1, helper.name)
	case handlerExpr != nil:
		s.handler(handlerExpr, env{}, 0)
	}
	if len(s.facts.Responses) == 0 {
		s.unknown("no response found")
	}
	return *s.facts
}

// handler scans the function a handler expression reaches, and the
// wrappers around it, each as a body in the package when it has one.
func (s *scanner) handler(e ast.Expr, en env, depth int) {
	switch e := e.(type) {
	case *ast.FuncLit:
		s.body(e.Body, en, depth, "")
	case *ast.ParenExpr:
		s.handler(e.X, en, depth)
	case *ast.Ident, *ast.SelectorExpr:
		id := e.(ast.Expr)
		var ident *ast.Ident
		if sel, ok := id.(*ast.SelectorExpr); ok {
			ident = sel.Sel
		} else {
			ident = id.(*ast.Ident)
		}
		obj := s.w.info.Uses[ident]
		if f := s.w.byObj[obj]; f != nil {
			s.body(f.body, en, depth, f.name)
			return
		}
		if fobj, ok := obj.(*types.Func); ok {
			s.unknown("handler " + s.w.funcName(fobj) + ": body outside the walk")
		} else {
			s.unknown("handler " + ident.Name + ": not a function the walk can see")
		}
	case *ast.CallExpr:
		if tv, ok := s.w.info.Types[e.Fun]; ok && tv.IsType() && len(e.Args) == 1 {
			s.handler(e.Args[0], en, depth)
			return
		}
		if s.w.isStatic(e) {
			s.facts.Responses = append(s.facts.Responses, api.Response{Status: 200, Note: "static files"})
			return
		}
		// A wrapper: its body (the closure it returns is inside it), then
		// the handler it was given.
		if f := s.w.callee(e); f != nil {
			was := s.guard
			s.guard = true
			s.body(f.body, s.bind(f, e, en), depth+1, f.name)
			s.guard = was
		} else if id := calleeIdent(e); id != nil {
			if fobj, ok := s.w.info.Uses[id].(*types.Func); ok {
				s.unknown("wrapper " + s.w.funcName(fobj) + ": body outside the walk")
			}
		}
		for _, a := range e.Args {
			if tv, ok := s.w.info.Types[a]; ok {
				if _, isFunc := tv.Type.Underlying().(*types.Signature); isFunc {
					s.handler(a, en, depth)
				}
			}
		}
	}
}

// bind is the callee's environment for one call: its parameters bound to
// the constants passed, and the note taken from the call.
func (s *scanner) bind(f *fn, call *ast.CallExpr, en env) env {
	out := env{bound: map[*types.Var]constant.Value{}, note: en.note}
	hasStatus := false
	note := ""
	for i, a := range call.Args {
		v, ok := s.w.constValue(a, en)
		if !ok {
			continue
		}
		if i < len(f.params) {
			out.bound[f.params[i]] = v
		}
		switch v.Kind() {
		case constant.Int:
			if n, ok := constant.Int64Val(v); ok && n >= 100 && n < 600 {
				hasStatus = true
			}
		case constant.String:
			if note == "" {
				note = constant.StringVal(v)
			}
		}
	}
	if hasStatus && note != "" {
		out.note = note
	}
	return out
}

// body scans one function body: the primitives in it are facts, and the
// calls into the package are followed.
func (s *scanner) body(body *ast.BlockStmt, en env, depth int, name string) {
	if body == nil || depth > maxDepth {
		return
	}
	key := name + "@" + envKey(en)
	if name != "" {
		if s.visited[key] {
			return
		}
		s.visited[key] = true
	}
	var cur api.Response // the response being built up: Content-Type set, then the status
	var last *api.Response
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.BinaryExpr:
			if isRequestMethod(s.w, n.X) {
				s.method(n.Y)
			} else if isRequestMethod(s.w, n.Y) {
				s.method(n.X)
			}
			return true
		case *ast.SwitchStmt:
			if n.Tag != nil && isRequestMethod(s.w, n.Tag) {
				for _, c := range n.Body.List {
					for _, e := range c.(*ast.CaseClause).List {
						s.method(e)
					}
				}
			}
			return true
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if s.primitive(call, en, &cur, &last) {
			return true
		}
		if f := s.w.callee(call); f != nil && f.body != body {
			s.body(f.body, s.bind(f, call, en), depth+1, f.name)
		}
		return true
	})
}

// primitive records the fact a call states, if it is one of the calls
// that state facts.
func (s *scanner) primitive(call *ast.CallExpr, en env, cur *api.Response, last **api.Response) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	fobj, ok := s.w.info.Uses[sel.Sel].(*types.Func)
	if !ok {
		return false
	}
	str := func(i int) string {
		if i < len(call.Args) {
			if v, ok := s.w.constValue(call.Args[i], en); ok && v.Kind() == constant.String {
				return constant.StringVal(v)
			}
		}
		return ""
	}
	status := func(i int) int {
		if i < len(call.Args) {
			if v, ok := s.w.constValue(call.Args[i], en); ok && v.Kind() == constant.Int {
				n, _ := constant.Int64Val(v)
				return int(n)
			}
		}
		return 0
	}
	// emit records a response; only one begun by WriteHeader is still
	// open for the Encode that follows it to name the body's type.
	emit := func(r api.Response, open bool) {
		if r.Note == "" {
			r.Note = en.note
		}
		s.facts.Responses = append(s.facts.Responses, r)
		if open {
			*last = &s.facts.Responses[len(s.facts.Responses)-1]
		}
	}
	// The package's own role check: the action it names is the route's.
	if fobj.Name() == "require" && fobj.Pkg() == s.w.pkg.Types {
		if a := str(2); a != "" && s.facts.Action == "" {
			s.facts.Action = a
		}
		return false
	}
	switch fobj.FullName() {
	case "(net/http.Header).Set":
		if str(0) == "Content-Type" && !onRequest(s.w, sel.X) {
			cur.Media = str(1)
		}
		return true
	case "(net/http.ResponseWriter).WriteHeader":
		emit(api.Response{Status: status(0), Media: cur.Media}, true)
		return true
	case "(net/http.ResponseWriter).Write", "fmt.Fprint", "fmt.Fprintf", "fmt.Fprintln":
		if *last == nil {
			emit(api.Response{Status: 200, Media: cur.Media}, false)
		}
		return true
	case "(*encoding/json.Encoder).Encode":
		typ := s.typeName(call.Args[0])
		if *last != nil && (*last).Type == "" {
			(*last).Type = typ
			return true
		}
		emit(api.Response{Status: 200, Media: cur.Media, Type: typ}, false)
		return true
	case "net/http.Error":
		emit(api.Response{Status: status(2), Media: "text/plain; charset=utf-8", Note: str(1)}, false)
		return true
	case "net/http.NotFound":
		emit(api.Response{Status: 404, Media: "text/plain; charset=utf-8"}, false)
		return true
	case "net/http.Redirect":
		emit(api.Response{Status: status(3), Redirect: s.pathIn(call.Args[2], en)}, false)
		return true
	case "(*encoding/json.Decoder).Decode":
		s.facts.Request = &api.Request{Type: s.typeName(call.Args[0])}
		return true
	case "(*net/http.Request).FormValue", "(*net/http.Request).PostFormValue":
		s.param(str(0), "form")
		return true
	case "(*net/http.Request).PathValue":
		s.param(str(0), "path")
		return true
	case "(*net/http.Request).FormFile":
		s.param(str(0), "file")
		return true
	case "(net/url.Values).Get":
		if isQuery(s.w, sel.X) {
			s.param(str(0), "query")
		}
		return true
	case "(net/http.Header).Get":
		if onRequest(s.w, sel.X) {
			s.read(&s.reads().Headers, str(0))
		}
		return true
	case "(*net/http.Request).Cookie":
		s.read(&s.reads().Cookies, str(0))
		return true
	}
	return false
}

func (s *scanner) param(name, in string) {
	if name == "" {
		return
	}
	p := api.Param{Name: name, In: in}
	if !slices.Contains(s.facts.Params, p) {
		s.facts.Params = append(s.facts.Params, p)
	}
}

// reads is where what is read goes: the guards when a wrapper is being
// scanned, else the handler's own.
func (s *scanner) reads() *api.Reads {
	if s.guard {
		return &s.facts.Guards
	}
	return &s.facts.Reads
}

func (s *scanner) read(list *[]string, name string) {
	if name != "" && !slices.Contains(*list, name) {
		*list = append(*list, name)
	}
}

func (s *scanner) unknown(what string) {
	if !slices.Contains(s.facts.Unknowns, what) {
		s.facts.Unknowns = append(s.facts.Unknowns, what)
	}
}

// typeName names the type of an encoded or decoded value as its schema
// is named, the pointer taken off.
func (s *scanner) typeName(e ast.Expr) string {
	if u, ok := e.(*ast.UnaryExpr); ok {
		e = u.X
	}
	tv, ok := s.w.info.Types[e]
	if !ok {
		return ""
	}
	return s.w.schemas.name(tv.Type)
}

// method records a method the handler tells apart: a comparison of
// r.Method with a constant, or a switch on it.
func (s *scanner) method(e ast.Expr) {
	if tv, ok := s.w.info.Types[e]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
		m := constant.StringVal(tv.Value)
		if m != "" && !slices.Contains(s.facts.Methods, m) {
			s.facts.Methods = append(s.facts.Methods, m)
			slices.Sort(s.facts.Methods)
		}
	}
}

// isRequestMethod says whether an expression is r.Method.
func isRequestMethod(w *walker, e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Method" {
		return false
	}
	tv, ok := w.info.Types[sel.X]
	return ok && types.TypeString(tv.Type, nil) == "*net/http.Request"
}

// pathIn is the constant path an expression names: the constant itself,
// a bound parameter, or the first such found among a call's arguments
// (s.at(path) names path).
func (s *scanner) pathIn(e ast.Expr, en env) string {
	if v, ok := s.w.constValue(e, en); ok && v.Kind() == constant.String {
		return constant.StringVal(v)
	}
	if call, ok := e.(*ast.CallExpr); ok {
		for _, a := range call.Args {
			if p := s.pathIn(a, en); p != "" {
				return p
			}
		}
	}
	return ""
}

// constValue is an expression's constant: its own, or the one its
// parameter was bound to.
func (w *walker) constValue(e ast.Expr, en env) (constant.Value, bool) {
	if tv, ok := w.info.Types[e]; ok && tv.Value != nil {
		return tv.Value, true
	}
	if id, ok := e.(*ast.Ident); ok && en.bound != nil {
		if v, ok := w.info.Uses[id].(*types.Var); ok {
			if c, ok := en.bound[v]; ok {
				return c, true
			}
		}
	}
	return nil, false
}

// onRequest says whether an expression is a request's header map
// (r.Header) rather than the response's (w.Header()).
func onRequest(w *walker, e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Header" {
		return false
	}
	tv, ok := w.info.Types[sel.X]
	return ok && types.TypeString(tv.Type, nil) == "*net/http.Request"
}

// isQuery says whether a url.Values came from r.URL.Query().
func isQuery(w *walker, e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	fobj, ok := w.info.Uses[sel.Sel].(*types.Func)
	return ok && fobj.FullName() == "(*net/url.URL).Query"
}

func envKey(en env) string {
	if len(en.bound) == 0 {
		return en.note
	}
	var parts []string
	for v, c := range en.bound {
		parts = append(parts, v.Name()+"="+c.ExactString())
	}
	slices.Sort(parts)
	return en.note + "|" + strings.Join(parts, ",")
}
