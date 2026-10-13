package api

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"html/template"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// The surface as a page: the same walk the OpenAPI document is rendered
// from, as HTML for the staff web's About → API and the docs site. One
// table per server, a row per route opening on what the handler reads
// and answers, then every type the bodies are made of. Plain HTML and
// Bulma classes, no script: the lofigui first rung.

// SurfaceJSON is the walked surface, generated beside the OpenAPI
// document, for the page to render from.
//
//go:embed surface.json
var SurfaceJSON []byte

// Load is the generated surface.
func Load() (Surface, error) {
	var s Surface
	err := json.Unmarshal(SurfaceJSON, &s)
	return s, err
}

// HTML renders the surface as a Bulma fragment.
func HTML(s Surface) template.HTML {
	v := pageView{Packages: make([]packageView, 0, len(s.Packages))}
	gates := map[string]Gate{}
	for _, g := range s.Gates {
		gates[g.Package] = g
	}
	for _, p := range s.Packages {
		pv := packageView{Package: p, Gate: gates[p.Path]}
		for _, r := range s.Routes {
			if r.Package == p.Path {
				pv.Routes = append(pv.Routes, routeView(r, gates[p.Path]))
			}
		}
		v.Packages = append(v.Packages, pv)
	}
	for _, t := range s.Types {
		if t.Package != "" {
			v.Types = append(v.Types, t)
		}
	}
	var b bytes.Buffer
	if err := page.Execute(&b, v); err != nil {
		return template.HTML("<p>api: " + template.HTMLEscapeString(err.Error()) + "</p>")
	}
	return template.HTML(b.String())
}

type pageView struct {
	Packages []packageView
	Types    []Schema
}

type packageView struct {
	Package Package
	Gate    Gate
	Routes  []rview
}

// rview is one route as the page shows it.
type rview struct {
	Route
	Methods   []string
	Summary   string
	Rest      string // the doc after its first sentence
	Security  []string
	Responses []respView
	Form      []Param
	Query     []Param
	Files     []Param
	PathParam []string
}

type respView struct {
	Status   int
	Media    []string
	Types    []string
	Notes    []string
	Redirect []string
}

var pathParamNames = regexp.MustCompile(`\{([^}]+)\}`)

func routeView(r Route, gate Gate) rview {
	v := rview{Route: r, Methods: methods(r), Summary: summary(r)}
	if rest := strings.TrimSpace(strings.TrimPrefix(strings.ReplaceAll(strings.ReplaceAll(r.Doc, "\n\n", "\x00"), "\n", " "), strings.ReplaceAll(v.Summary, "\n", " "))); rest != "" {
		v.Rest = strings.ReplaceAll(strings.TrimLeft(rest, ". "), "\x00", "\n\n")
	}
	for _, m := range pathParamNames.FindAllStringSubmatch(r.Path, -1) {
		v.PathParam = append(v.PathParam, strings.TrimSuffix(m[1], "..."))
	}
	for _, p := range r.Params {
		switch p.In {
		case "query":
			v.Query = append(v.Query, p)
		case "form":
			v.Form = append(v.Form, p)
		case "file":
			v.Files = append(v.Files, p)
		}
	}
	add := func(reads Reads) {
		for _, c := range reads.Cookies {
			v.Security = append(v.Security, "cookie "+c)
		}
		for _, h := range reads.Headers {
			if h == "Authorization" {
				v.Security = append(v.Security, "bearer token")
			}
		}
	}
	if gate.Wrapper != "" {
		open := false
		for _, p := range gate.Open {
			open = open || p == r.Path
		}
		if !open {
			add(gate.Reads)
		}
	}
	add(r.Guards)
	byStatus := map[int]*respView{}
	for _, x := range r.Responses {
		rv := byStatus[x.Status]
		if rv == nil {
			rv = &respView{Status: x.Status}
			byStatus[x.Status] = rv
		}
		if m := mediaType(x.Media); m != "" && !contains(rv.Media, m) {
			rv.Media = append(rv.Media, m)
		}
		if x.Type != "" && !contains(rv.Types, x.Type) {
			rv.Types = append(rv.Types, x.Type)
		}
		if x.Note != "" && !contains(rv.Notes, x.Note) {
			rv.Notes = append(rv.Notes, x.Note)
		}
		if x.Redirect != "" && !contains(rv.Redirect, x.Redirect) {
			rv.Redirect = append(rv.Redirect, x.Redirect)
		}
	}
	for _, rv := range byStatus {
		sort.Strings(rv.Media)
		v.Responses = append(v.Responses, *rv)
	}
	sort.Slice(v.Responses, func(i, j int) bool { return v.Responses[i].Status < v.Responses[j].Status })
	return v
}

func contains(list []string, s string) bool {
	return slices.Contains(list, s)
}

// prose turns a Go comment into paragraphs: a blank line is a break, a
// line wrap is not.
func prose(doc string) template.HTML {
	var b strings.Builder
	for para := range strings.SplitSeq(strings.TrimSpace(doc), "\n\n") {
		b.WriteString("<p>" + template.HTMLEscapeString(strings.Join(strings.Fields(para), " ")) + "</p>")
	}
	return template.HTML(b.String())
}

var page = template.Must(template.New("api").Funcs(template.FuncMap{
	"join":  strings.Join,
	"prose": prose,
	"status": func(code int) string {
		return statusText(code)
	},
	"anchor": func(name string) string {
		return "type-" + strings.NewReplacer(".", "-", "[", "", "]", "").Replace(name)
	},
	"typeLink": typeLink,
}).Parse(`
<h2 class="title is-4">API</h2>
<p class="subtitle is-6 has-text-grey">Read off the code at build time: every route the servers register and what each handler reads and answers. Branches are not followed, so a response listed is one the handler can write on some path through it. The same walk as <a href="v1/openapi.json"><code>/v1/openapi.json</code></a>.</p>
{{range .Packages}}
<div class="box">
<h3 class="title is-5"><code>{{.Package.Path}}</code></h3>
{{if .Package.Doc}}<div class="content is-small">{{prose .Package.Doc}}</div>{{end}}
{{if .Gate.Wrapper}}<p class="is-size-7 has-text-grey mb-3"><strong>Gate</strong> <code>{{.Gate.Wrapper}}</code>: every route but {{range $i, $p := .Gate.Open}}{{if $i}}, {{end}}<code>{{$p}}</code>{{end}} needs {{range $i, $c := .Gate.Reads.Cookies}}{{if $i}} or {{end}}the <code>{{$c}}</code> cookie{{end}}{{range .Gate.Reads.Headers}}{{if eq . "Authorization"}} or a bearer token{{end}}{{end}}.</p>{{if .Gate.Doc}}<div class="content is-small">{{prose .Gate.Doc}}</div>{{end}}{{end}}
<table class="table is-fullwidth is-narrow is-hoverable">
<thead><tr><th style="width:7em">Method</th><th style="width:22em">Path</th><th>Route</th></tr></thead>
<tbody>
{{range .Routes}}
<tr><td><code>{{join .Methods " "}}</code></td><td><code>{{.Path}}</code></td><td>
<details>
<summary>{{.Summary}}{{if .Action}} <span class="tag is-light is-warning is-small">{{.Action}}</span>{{end}}{{if .Unknowns}} <span class="tag is-light is-danger is-small">unknown</span>{{end}}</summary>
<div class="content is-small mt-2">
{{if .Rest}}{{prose .Rest}}{{end}}
<table class="table is-narrow is-size-7">
{{if .Security}}<tr><th>Needs</th><td>{{join .Security ", "}}{{if .Action}}; role action <code>{{.Action}}</code>{{end}}</td></tr>{{else}}<tr><th>Needs</th><td>nothing: open</td></tr>{{end}}
{{if .PathParam}}<tr><th>Path</th><td>{{range .PathParam}}<code>{{.}}</code> {{end}}</td></tr>{{end}}
{{if .Query}}<tr><th>Query</th><td>{{range .Query}}<code>{{.Name}}</code> {{end}}</td></tr>{{end}}
{{if .Form}}<tr><th>Form</th><td>{{range .Form}}<code>{{.Name}}</code> {{end}}</td></tr>{{end}}
{{if .Files}}<tr><th>Upload</th><td>{{range .Files}}<code>{{.Name}}</code> {{end}}</td></tr>{{end}}
{{if .Request}}<tr><th>JSON body</th><td>{{typeLink .Request.Type}}</td></tr>{{end}}
<tr><th>Answers</th><td>
{{range .Responses}}<div><strong>{{.Status}}</strong> {{status .Status}}{{if .Redirect}} to {{range .Redirect}}<code>{{.}}</code> {{end}}{{end}}{{if .Media}} as {{join .Media ", "}}{{end}}{{if .Types}}: {{range $i, $t := .Types}}{{if $i}} or {{end}}{{typeLink $t}}{{end}}{{end}}{{if .Notes}} <span class="has-text-grey">({{join .Notes "; "}})</span>{{end}}</div>{{end}}
</td></tr>
<tr><th>Source</th><td><code>{{.Pos}}</code>{{if .Handler}} <code>{{.Handler}}</code>{{end}}{{range .Wrappers}} in <code>{{.}}</code>{{end}}{{if .Via}} via <code>{{.Via}}</code>{{end}}</td></tr>
{{if .Unknowns}}<tr><th>Unknown</th><td>{{join .Unknowns "; "}}</td></tr>{{end}}
</table>
</div>
</details>
</td></tr>
{{end}}
</tbody></table>
</div>
{{end}}
<div class="box">
<h3 class="title is-5">Types</h3>
<p class="is-size-7 has-text-grey">Every type the bodies above are made of, from the Go declarations and their comments.</p>
{{range .Types}}
<h4 class="title is-6 mt-4 mb-1" id="{{anchor .Name}}"><code>{{.Name}}</code> <span class="has-text-grey has-text-weight-normal">{{.Kind}}{{if .Format}} ({{.Format}}){{end}}</span></h4>
{{if .Doc}}<div class="content is-small">{{prose .Doc}}</div>{{end}}
{{if .Enum}}<p class="is-size-7">One of: {{range $i, $e := .Enum}}{{if $i}}, {{end}}<code>{{if $e}}{{$e}}{{else}}""{{end}}</code>{{end}}</p>{{end}}
{{if .Fields}}<table class="table is-narrow is-size-7"><thead><tr><th>Field</th><th>Type</th><th></th></tr></thead><tbody>
{{range .Fields}}<tr><td><code>{{.Name}}</code></td><td>{{typeLink .Type}}{{if .Nullable}} or null{{end}}{{if .Optional}} <span class="has-text-grey">optional</span>{{end}}</td><td>{{.Doc}}</td></tr>
{{end}}</tbody></table>{{end}}
{{if .Elem}}<p class="is-size-7">of {{typeLink .Elem}}</p>{{end}}
{{end}}
</div>
`))

// typeLink renders a type name with its named parts linked to their
// sections: "[]screen.Component" links Component.
func typeLink(name string) template.HTML {
	var prefix strings.Builder
	for {
		switch {
		case strings.HasPrefix(name, "[]"):
			prefix.WriteString("[]")
			name = name[2:]
			continue
		case strings.HasPrefix(name, "map[string]"):
			prefix.WriteString("map[string]")
			name = strings.TrimPrefix(name, "map[string]")
			continue
		}
		break
	}
	esc := template.HTMLEscapeString
	if strings.Contains(name, ".") {
		anchor := "type-" + strings.ReplaceAll(name, ".", "-")
		return template.HTML(esc(prefix.String()) + `<a href="#` + anchor + `"><code>` + esc(name) + `</code></a>`)
	}
	return template.HTML(`<code>` + esc(prefix.String()+name) + `</code>`)
}
