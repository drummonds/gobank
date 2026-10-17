package api

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// OpenAPI renders the surface as an OpenAPI 3.1 document. Everything in
// it comes from the walk: the operations from the routes, the schemas
// from the types, the security schemes from the cookies and headers the
// gates and wrappers read. Keys are sorted, so the document is stable
// and a change in it is a change in the code.
func OpenAPI(s Surface) ([]byte, error) {
	d := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   "Model Bank API",
			"version": "1",
			"description": "Read off the code by api/walk: every route the servers register, with what each handler can answer. " +
				"Branches are not followed, so a response listed is one the handler can write on some path through it; " +
				"a route whose handler the walk could not see into says so under x-unknowns.",
		},
	}
	gates := map[string]Gate{}
	for _, g := range s.Gates {
		gates[g.Package] = g
	}
	pkgName := map[string]string{}
	for _, p := range s.Packages {
		pkgName[p.Path] = p.Name
	}
	var tags []any
	for _, reg := range s.Registrars {
		title, note := reg.Cluster()
		tags = append(tags, map[string]any{"name": title, "description": note, "x-package": pkgName[reg.Package]})
	}
	d["tags"] = tags

	paths := map[string]any{}
	schemes := map[string]any{}
	for _, r := range s.Routes {
		item, _ := paths[r.Path].(map[string]any)
		if item == nil {
			item = map[string]any{}
			paths[r.Path] = item
		}
		op := operation(r, s.Cluster(r), pkgName[r.Package], gates[r.Package], schemes)
		for _, m := range methods(r) {
			item[strings.ToLower(m)] = op
		}
	}
	d["paths"] = paths

	schemas := map[string]any{}
	for _, t := range s.Types {
		if t.Package == "" {
			continue // arrays and maps are inlined where they appear
		}
		schemas[t.Name] = schemaOf(t)
	}
	d["components"] = map[string]any{"schemas": schemas, "securitySchemes": schemes}
	return json.MarshalIndent(d, "", "  ")
}

// methods are the methods an operation is documented under: the
// pattern's, else those the handler tells apart, else GET as the one a
// browser sends to a path that serves them all alike.
func methods(r Route) []string {
	if r.Method != "" {
		return []string{r.Method}
	}
	if len(r.Methods) > 0 {
		return r.Methods
	}
	return []string{"GET"}
}

var pathParam = regexp.MustCompile(`\{([^}]+)\}`)

func operation(r Route, cluster, pkg string, gate Gate, schemes map[string]any) map[string]any {
	if cluster == "" { // registered outside any declared function
		cluster = pkg
	}
	op := map[string]any{
		"tags":        []any{cluster},
		"summary":     summary(r),
		"operationId": operationID(r),
		"x-package":   pkg,
		"x-source":    r.Pos,
	}
	if r.Doc != "" {
		op["description"] = r.Doc
	}
	if r.Handler != "" || len(r.Wrappers) > 0 || r.Via != "" {
		h := map[string]any{}
		if r.Handler != "" {
			h["function"] = r.Handler
		}
		if len(r.Wrappers) > 0 {
			h["wrappers"] = anyStrings(r.Wrappers)
		}
		if r.Via != "" {
			h["via"] = r.Via
		}
		op["x-handler"] = h
	}
	if r.Method == "" && len(r.Methods) == 0 {
		op["x-methods"] = "any"
	}
	if strings.HasSuffix(r.Path, "/") && r.Path != "/" {
		op["x-prefix"] = "serves every path under it"
	}
	if r.Path == "/" {
		op["x-prefix"] = "serves every path no other route does"
	}

	// Parameters: the pattern's, the query's.
	var params []any
	seen := map[string]bool{}
	for _, m := range pathParam.FindAllStringSubmatch(r.Path, -1) {
		name := strings.TrimSuffix(m[1], "...")
		params = append(params, map[string]any{"name": name, "in": "path", "required": true, "schema": map[string]any{"type": "string"}})
		seen["path "+name] = true
	}
	for _, p := range r.Params {
		if p.In != "query" || seen["query "+p.Name] {
			continue
		}
		seen["query "+p.Name] = true
		params = append(params, map[string]any{"name": p.Name, "in": "query", "schema": map[string]any{"type": "string"}})
	}
	if len(params) > 0 {
		op["parameters"] = params
	}

	// Request body: a JSON type decoded, a form read, a file uploaded.
	content := map[string]any{}
	if r.Request != nil {
		content["application/json"] = map[string]any{"schema": ref(r.Request.Type)}
	}
	form := map[string]any{}
	files := map[string]any{}
	for _, p := range r.Params {
		switch p.In {
		case "form":
			form[p.Name] = map[string]any{"type": "string"}
		case "file":
			files[p.Name] = map[string]any{"type": "string", "format": "binary"}
		}
	}
	if len(form) > 0 {
		content["application/x-www-form-urlencoded"] = map[string]any{"schema": map[string]any{"type": "object", "properties": form}}
	}
	if len(files) > 0 {
		content["multipart/form-data"] = map[string]any{"schema": map[string]any{"type": "object", "properties": files}}
	}
	if len(content) > 0 {
		op["requestBody"] = map[string]any{"content": content}
	}

	op["responses"] = responses(r)

	// Security: what the gate reads unless the path is open to it, and
	// what the route's own wrappers read.
	var security []any
	add := func(reads Reads) {
		for _, c := range reads.Cookies {
			schemes[c] = map[string]any{"type": "apiKey", "in": "cookie", "name": c}
			security = append(security, map[string]any{c: []any{}})
		}
		if slices.Contains(reads.Headers, "Authorization") {
			schemes["bearer"] = map[string]any{"type": "http", "scheme": "bearer"}
			security = append(security, map[string]any{"bearer": []any{}})
		}
	}
	if gate.Wrapper != "" && !slices.Contains(gate.Open, r.Path) {
		add(gate.Reads)
	}
	add(r.Guards)
	if security == nil {
		security = []any{}
	}
	op["security"] = security
	if r.Action != "" {
		op["x-role-action"] = r.Action
	}
	if len(r.Unknowns) > 0 {
		op["x-unknowns"] = anyStrings(r.Unknowns)
	}
	return op
}

// responses groups the handler's responses by status: the notes as the
// description, the media each can carry with the schema of what it
// encodes, and the redirect's target.
func responses(r Route) map[string]any {
	byStatus := map[int][]Response{}
	for _, x := range r.Responses {
		byStatus[x.Status] = append(byStatus[x.Status], x)
	}
	out := map[string]any{}
	for status, list := range byStatus {
		var notes, targets []string
		media := map[string][]string{} // media → types encoded
		for _, x := range list {
			if x.Note != "" && !slices.Contains(notes, x.Note) {
				notes = append(notes, x.Note)
			}
			if x.Redirect != "" && !slices.Contains(targets, x.Redirect) {
				targets = append(targets, x.Redirect)
			}
			if x.Media != "" {
				m := mediaType(x.Media)
				if x.Type != "" && !slices.Contains(media[m], x.Type) {
					media[m] = append(media[m], x.Type)
				} else if _, ok := media[m]; !ok {
					media[m] = nil
				}
			}
		}
		resp := map[string]any{}
		desc := strings.Join(notes, "; ")
		if status >= 300 && status < 400 {
			if len(targets) > 0 {
				desc = strings.TrimSpace("Redirect to " + strings.Join(targets, " or ") + ". " + desc)
			} else if desc == "" {
				desc = "Redirect"
			}
			resp["headers"] = map[string]any{"Location": map[string]any{"schema": map[string]any{"type": "string"}}}
		}
		if desc == "" {
			desc = statusText(status)
		}
		resp["description"] = desc
		if len(media) > 0 {
			content := map[string]any{}
			for m, typesEncoded := range media {
				switch {
				case len(typesEncoded) == 1:
					content[m] = map[string]any{"schema": ref(typesEncoded[0])}
				case len(typesEncoded) > 1:
					sort.Strings(typesEncoded)
					var oneOf []any
					for _, t := range typesEncoded {
						oneOf = append(oneOf, ref(t))
					}
					content[m] = map[string]any{"schema": map[string]any{"oneOf": oneOf}}
				case strings.HasPrefix(m, "application/json"):
					content[m] = map[string]any{"schema": map[string]any{"type": "object"}}
				default:
					content[m] = map[string]any{"schema": map[string]any{"type": "string"}}
				}
			}
			resp["content"] = content
		}
		out[fmt.Sprint(status)] = resp
	}
	return out
}

// mediaType is a Content-Type without its parameters.
func mediaType(ct string) string {
	m, _, _ := strings.Cut(ct, ";")
	return strings.TrimSpace(m)
}

func statusText(status int) string {
	switch status {
	case 200:
		return "OK"
	case 303:
		return "Redirect"
	case 400:
		return "Bad request"
	case 401:
		return "Unauthenticated"
	case 403:
		return "Forbidden"
	case 404:
		return "Not found"
	case 405:
		return "Method not allowed"
	case 429:
		return "Too many requests"
	case 500:
		return "Internal error"
	case 503:
		return "Unavailable"
	}
	return fmt.Sprintf("Status %d", status)
}

// ref is the schema for a type name: a reference for a named type, an
// array or map inlined over its element, a scalar as itself.
func ref(name string) map[string]any {
	switch {
	case strings.HasPrefix(name, "[]"):
		return map[string]any{"type": "array", "items": ref(name[2:])}
	case strings.HasPrefix(name, "map[string]"):
		return map[string]any{"type": "object", "additionalProperties": ref(strings.TrimPrefix(name, "map[string]"))}
	case name == "string", name == "integer", name == "number", name == "boolean":
		return map[string]any{"type": name}
	case name == "any", name == "object", name == "":
		return map[string]any{}
	}
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

// schemaOf is a named type's component schema.
func schemaOf(t Schema) map[string]any {
	sc := map[string]any{}
	if t.Doc != "" {
		sc["description"] = t.Doc
	}
	switch t.Kind {
	case "object":
		props := map[string]any{}
		var required []any
		for _, f := range t.Fields {
			p := ref(f.Type)
			if f.Nullable {
				p = nullable(p)
			}
			if f.Doc != "" {
				p = withDescription(p, f.Doc)
			}
			props[f.Name] = p
			if !f.Optional {
				required = append(required, f.Name)
			}
		}
		sc["type"] = "object"
		sc["properties"] = props
		if len(required) > 0 {
			sc["required"] = required
		}
	case "array":
		sc["type"] = "array"
		sc["items"] = ref(t.Elem)
	case "map":
		sc["type"] = "object"
		sc["additionalProperties"] = ref(t.Elem)
	case "any":
	default:
		sc["type"] = t.Kind
		if t.Format != "" {
			sc["format"] = t.Format
		}
		if len(t.Enum) > 0 {
			sc["enum"] = anyStrings(t.Enum)
		}
	}
	return sc
}

// nullable admits null beside a schema: a type list for a scalar, an
// anyOf for a reference.
func nullable(p map[string]any) map[string]any {
	if t, ok := p["type"].(string); ok && len(p) == 1 {
		return map[string]any{"type": []any{t, "null"}}
	}
	return map[string]any{"anyOf": []any{p, map[string]any{"type": "null"}}}
}

// withDescription puts a description on a schema; a bare reference is
// wrapped, since a $ref carries nothing beside it.
func withDescription(p map[string]any, doc string) map[string]any {
	if _, isRef := p["$ref"]; isRef {
		return map[string]any{"allOf": []any{p}, "description": doc}
	}
	out := map[string]any{}
	maps.Copy(out, p)
	out["description"] = doc
	return out
}

// summary is the first sentence of the doc, else the path.
func summary(r Route) string {
	doc := strings.TrimSpace(r.Doc)
	if doc == "" {
		return strings.TrimSpace(r.Method + " " + r.Path)
	}
	// The first sentence: up to a full stop that ends one (not the dot of
	// .goluca) or the first paragraph break; a comment's line wrapping is
	// not a break.
	doc = strings.ReplaceAll(doc, "\n\n", "\x00")
	doc = strings.ReplaceAll(doc, "\n", " ")
	for i := 0; i < len(doc); i++ {
		if doc[i] == 0 || doc[i] == '.' && (i+1 == len(doc) || doc[i+1] == ' ' || doc[i+1] == 0) {
			doc = doc[:i]
			break
		}
	}
	return strings.TrimSpace(doc)
}

var nonWord = regexp.MustCompile(`[^A-Za-z0-9]+`)

func operationID(r Route) string {
	return strings.Trim(nonWord.ReplaceAllString(strings.ToLower(strings.Join(methods(r), "-")+" "+r.Path), "-"), "-")
}

func anyStrings(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
