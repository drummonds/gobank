package walk

import (
	"go/ast"
	"go/constant"
	"go/types"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"git.bytestone.uk/hum3/gobank/api"
)

// schemas names the types bodies are made of and builds a Schema for
// each, with the doc comments from the type's own package when that
// package is in the module and can be loaded with its syntax.
type schemas struct {
	loader *loader
	named  map[string]types.Type // every composite or named type named so far
}

func newSchemas(ld *loader) *schemas { return &schemas{loader: ld, named: map[string]types.Type{}} }

// name is the schema name of a type, or its scalar kind; a composite or
// named type is remembered so its schema is built.
func (s *schemas) name(t types.Type) string {
	switch t := t.(type) {
	case *types.Pointer:
		return s.name(t.Elem())
	case *types.Named:
		obj := t.Obj()
		if obj.Pkg() == nil {
			return s.name(t.Underlying()) // error, comparable
		}
		n := obj.Pkg().Name() + "." + obj.Name()
		if obj.Pkg().Path() == "time" && obj.Name() == "Time" {
			s.named[n] = t
			return n
		}
		if _, ok := t.Underlying().(*types.Interface); ok {
			return "any"
		}
		s.named[n] = t
		return n
	case *types.Alias:
		return s.name(types.Unalias(t))
	case *types.Slice:
		n := "[]" + s.name(t.Elem())
		s.named[n] = t
		return n
	case *types.Array:
		n := "[]" + s.name(t.Elem())
		s.named[n] = t
		return n
	case *types.Map:
		n := "map[string]" + s.name(t.Elem())
		s.named[n] = t
		return n
	case *types.Basic:
		return kindOf(t)
	case *types.Interface:
		return "any"
	case *types.Struct:
		return "object"
	}
	return "any"
}

func kindOf(b *types.Basic) string {
	switch {
	case b.Info()&types.IsBoolean != 0:
		return "boolean"
	case b.Info()&types.IsInteger != 0:
		return "integer"
	case b.Info()&types.IsFloat != 0:
		return "number"
	case b.Info()&types.IsString != 0:
		return "string"
	}
	return "any"
}

// build makes the schema of every type named, and of the types those
// reach, until none is left.
func (s *schemas) build() []api.Schema {
	done := map[string]bool{}
	var out []api.Schema
	for {
		var pending []string
		for n := range s.named {
			if !done[n] {
				pending = append(pending, n)
			}
		}
		if len(pending) == 0 {
			break
		}
		sort.Strings(pending)
		for _, n := range pending {
			done[n] = true
			out = append(out, s.schema(n, s.named[n]))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *schemas) schema(name string, t types.Type) api.Schema {
	sc := api.Schema{Name: name}
	switch tt := t.(type) {
	case *types.Slice:
		sc.Kind, sc.Elem = "array", s.name(tt.Elem())
		return sc
	case *types.Array:
		sc.Kind, sc.Elem = "array", s.name(tt.Elem())
		return sc
	case *types.Map:
		sc.Kind, sc.Elem = "map", s.name(tt.Elem())
		return sc
	}
	named := t.(*types.Named)
	obj := named.Obj()
	sc.Package = obj.Pkg().Path()
	if sc.Package == "time" && obj.Name() == "Time" {
		sc.Kind, sc.Format = "string", "date-time"
		sc.Doc = "RFC 3339"
		return sc
	}
	spec, fieldDocs := s.loader.typeDecl(sc.Package, obj.Name())
	sc.Doc = spec
	switch u := named.Underlying().(type) {
	case *types.Struct:
		sc.Kind = "object"
		sc.Fields = s.fields(u, fieldDocs)
	case *types.Basic:
		sc.Kind = kindOf(u)
		sc.Enum = enumOf(named)
	case *types.Slice:
		sc.Kind, sc.Elem = "array", s.name(u.Elem())
	case *types.Map:
		sc.Kind, sc.Elem = "map", s.name(u.Elem())
	default:
		sc.Kind = "any"
	}
	return sc
}

// fields are a struct's JSON properties in declaration order, embedded
// structs flattened as encoding/json flattens them.
func (s *schemas) fields(st *types.Struct, docs map[string]string) []api.Field {
	var out []api.Field
	for i := range st.NumFields() {
		f := st.Field(i)
		tag := reflect.StructTag(st.Tag(i)).Get("json")
		if tag == "-" {
			continue
		}
		jsonName, opts, _ := strings.Cut(tag, ",")
		if f.Embedded() && jsonName == "" {
			if inner, ok := f.Type().Underlying().(*types.Struct); ok {
				_, innerDocs := s.loader.typeDecl(pkgPathOf(f.Type()), typeNameOf(f.Type()))
				out = append(out, s.fields(inner, innerDocs)...)
				continue
			}
		}
		if !f.Exported() {
			continue
		}
		if jsonName == "" {
			jsonName = f.Name()
		}
		_, nullable := f.Type().(*types.Pointer)
		out = append(out, api.Field{
			Name: jsonName, Type: s.name(f.Type()), Doc: docs[f.Name()],
			Optional: strings.Contains(","+opts+",", ",omitempty,"), Nullable: nullable,
		})
	}
	return out
}

func pkgPathOf(t types.Type) string {
	if n, ok := t.(*types.Named); ok && n.Obj().Pkg() != nil {
		return n.Obj().Pkg().Path()
	}
	return ""
}

func typeNameOf(t types.Type) string {
	if n, ok := t.(*types.Named); ok {
		return n.Obj().Name()
	}
	return ""
}

// enumOf is the values of the constants of a named type in its package.
func enumOf(named *types.Named) []string {
	scope := named.Obj().Pkg().Scope()
	var out []string
	for _, name := range scope.Names() {
		c, ok := scope.Lookup(name).(*types.Const)
		if !ok || !types.Identical(c.Type(), named) {
			continue
		}
		if c.Val().Kind() == constant.String {
			out = append(out, constant.StringVal(c.Val()))
		} else {
			out = append(out, c.Val().ExactString())
		}
	}
	return out
}

// loader holds the packages loaded, and loads a package of the module
// with its syntax when a type's doc comments are wanted.
type loader struct {
	module string // the main module's path; packages outside it are not loaded for docs
	pkgs   map[string]*packages.Package
}

func newLoader(initial []*packages.Package) *loader {
	ld := &loader{pkgs: map[string]*packages.Package{}}
	for _, p := range initial {
		ld.pkgs[p.PkgPath] = p
		if p.Module != nil && ld.module == "" {
			ld.module = p.Module.Path
		}
	}
	return ld
}

func (ld *loader) syntax(path string) *packages.Package {
	if p, ok := ld.pkgs[path]; ok {
		return p
	}
	if ld.module == "" || !strings.HasPrefix(path, ld.module) {
		ld.pkgs[path] = nil
		return nil
	}
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo}
	pkgs, err := packages.Load(cfg, path)
	if err != nil || len(pkgs) != 1 {
		ld.pkgs[path] = nil
		return nil
	}
	ld.pkgs[path] = pkgs[0]
	return pkgs[0]
}

// typeDecl is a type's doc comment and its fields' comments (doc above
// or line beside, by Go field name), from the package's syntax.
func (ld *loader) typeDecl(pkgPath, name string) (doc string, fields map[string]string) {
	fields = map[string]string{}
	p := ld.syntax(pkgPath)
	if p == nil {
		return "", fields
	}
	for _, file := range p.Syntax {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Name.Name != name {
					continue
				}
				if ts.Doc != nil {
					doc = ts.Doc.Text()
				} else if gd.Doc != nil && len(gd.Specs) == 1 {
					doc = gd.Doc.Text()
				}
				doc = strings.TrimSpace(doc)
				if st, ok := ts.Type.(*ast.StructType); ok {
					for _, f := range st.Fields.List {
						text := ""
						if f.Doc != nil {
							text = f.Doc.Text()
						} else if f.Comment != nil {
							text = f.Comment.Text()
						}
						for _, n := range f.Names {
							fields[n.Name] = strings.TrimSpace(text)
						}
					}
				}
				return doc, fields
			}
		}
	}
	return "", fields
}
