package api

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// The surface as pages: the same walk the OpenAPI document is rendered
// from, as HTML for the staff web's About → API and the docs site. The
// index is a table per server, in sections by the function the routes
// are registered in; a route's detail page is what the handler reads and
// answers, with its source. Plain HTML and Bulma classes, no script: the
// lofigui first rung. The staff web serves the index and a page per
// route; the docs site writes them all into one page.

// SurfaceJSON is the walked surface, generated beside the OpenAPI
// document, for the pages to render from.
//
//go:embed surface.json
var SurfaceJSON []byte

// Load is the generated surface.
func Load() (Surface, error) {
	var s Surface
	err := json.Unmarshal(SurfaceJSON, &s)
	return s, err
}

// Options say where the pages link: a route's detail, and its source.
type Options struct {
	// DetailURL is the href of a route's detail page from the index; nil
	// links to the route's anchor on the same page.
	DetailURL func(Route) string
	// IndexURL is the href back to the index from a detail page.
	IndexURL string
	// SourceRef is the ref the source links point at in the module's
	// forge, as "tag/v0.43.0" or "branch/main"; "" makes no links.
	SourceRef string
	// Perf is what the running process has measured of a route, false
	// for one it has not served; nil shows no performance at all (a
	// static rendering).
	Perf func(Route) (Perf, bool)
}

// Key names a route for a URL: its method, or "any", then its path, as
// "get/v1/health" or "any/settings".
func Key(r Route) string {
	m := strings.ToLower(r.Method)
	if m == "" {
		m = "any"
	}
	return m + r.Path
}

// Find is the route a key names, as Key makes them.
func Find(s Surface, method, path string) (Route, bool) {
	if method == "any" {
		method = ""
	}
	method = strings.ToUpper(method)
	for _, r := range s.Routes {
		if r.Method == method && r.Path == path {
			return r, true
		}
	}
	return Route{}, false
}

// Index renders the table of routes, a box per server and a section per
// registrar, each route linking to its detail.
func Index(s Surface, o Options) template.HTML {
	return render(indexTmpl, indexView(s, o))
}

// Detail renders one route's page.
func Detail(s Surface, r Route, o Options) template.HTML {
	return render(detailTmpl, routeView(s, r, o))
}

// Page is the index and every route's detail on one page, with the
// types: the docs site's page.
func Page(s Surface, o Options) template.HTML {
	if o.DetailURL == nil {
		o.DetailURL = func(r Route) string { return "#route-" + anchorKey(r) }
	}
	if o.IndexURL == "" {
		o.IndexURL = "#"
	}
	v := pageView{Index: indexView(s, o)}
	for _, r := range s.Routes {
		v.Routes = append(v.Routes, routeView(s, r, o))
	}
	for _, t := range s.Types {
		if t.Package != "" {
			v.Types = append(v.Types, t)
		}
	}
	return render(pageTmpl, v)
}

// Types renders the types section alone: the staff web's index carries
// it after the routes.
func Types(s Surface) template.HTML {
	var named []Schema
	for _, t := range s.Types {
		if t.Package != "" {
			named = append(named, t)
		}
	}
	return render(typesTmpl, named)
}

func render(t *template.Template, data any) template.HTML {
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		return template.HTML("<p>api: " + template.HTMLEscapeString(err.Error()) + "</p>")
	}
	return template.HTML(b.String())
}

type pageView struct {
	Index  iview
	Routes []rview
	Types  []Schema
}

type iview struct {
	Packages []packageView
	Perf     bool // the requests and p95 columns are shown
}

type packageView struct {
	Package  Package
	Gate     Gate
	Sections []sectionView
}

type sectionView struct {
	Title  string // "" when the package has one registrar: no heading
	Doc    string
	Routes []rview
}

// rview is one route as the pages show it.
type rview struct {
	Route
	Package   Package
	Methods   []string
	Summary   string
	Rest      string // the doc after its first sentence
	Security  []string
	Responses []respView
	Form      []Param
	Query     []Param
	Files     []Param
	PathParam []string
	URL       string // the detail page
	IndexURL  string
	SourceURL string // the registration in the forge, "" when unlinked
	Anchor    string
	Perf      *perfView // nil when the pages show no performance
}

type respView struct {
	Status   int
	Media    []string
	Types    []string
	Notes    []string
	Redirect []string
}

var pathParamNames = regexp.MustCompile(`\{([^}]+)\}`)

func indexView(s Surface, o Options) iview {
	v := iview{Perf: o.Perf != nil}
	gates := map[string]Gate{}
	for _, g := range s.Gates {
		gates[g.Package] = g
	}
	for _, p := range s.Packages {
		pv := packageView{Package: p, Gate: gates[p.Path]}
		var regs []Registrar
		for _, reg := range s.Registrars {
			if reg.Package == p.Path {
				regs = append(regs, reg)
			}
		}
		for _, reg := range regs {
			sec := sectionView{}
			if len(regs) > 1 {
				sec.Title, sec.Doc = sectionTitle(reg)
			}
			for _, r := range s.Routes {
				if r.Package == p.Path && r.Registrar == reg.Name {
					sec.Routes = append(sec.Routes, routeView(s, r, o))
				}
			}
			if len(sec.Routes) > 0 {
				pv.Sections = append(pv.Sections, sec)
			}
		}
		var loose sectionView // registered outside any declared function
		for _, r := range s.Routes {
			if r.Package == p.Path && r.Registrar == "" {
				loose.Routes = append(loose.Routes, routeView(s, r, o))
			}
		}
		if len(loose.Routes) > 0 {
			pv.Sections = append(pv.Sections, loose)
		}
		v.Packages = append(v.Packages, pv)
	}
	return v
}

// sectionTitle makes a heading of a registrar's doc: what it mounts, up
// to the colon or full stop, and the rest as the section's note.
func sectionTitle(reg Registrar) (title, doc string) {
	d := strings.Join(strings.Fields(reg.Doc), " ")
	if d == "" {
		return reg.Name, ""
	}
	for _, verb := range []string{"mounts ", "serves ", "builds ", "registers "} {
		if len(d) > len(verb) && strings.EqualFold(d[:len(verb)], verb) {
			d = d[len(verb):]
			break
		}
	}
	title = d
	if i := strings.IndexAny(d, ":."); i > 0 {
		title, doc = d[:i], strings.TrimSpace(strings.TrimLeft(d[i+1:], ":. "))
	}
	return strings.ToUpper(title[:1]) + title[1:], doc
}

func routeView(s Surface, r Route, o Options) rview {
	v := rview{Route: r, Methods: methods(r), Summary: summary(r), Anchor: anchorKey(r), IndexURL: o.IndexURL}
	for _, p := range s.Packages {
		if p.Path == r.Package {
			v.Package = p
		}
	}
	if o.DetailURL != nil {
		v.URL = o.DetailURL(r)
	} else {
		v.URL = "#route-" + v.Anchor
	}
	if o.SourceRef != "" && s.Module != "" {
		if file, line, ok := strings.Cut(r.Pos, ":"); ok {
			v.SourceURL = fmt.Sprintf("https://%s/src/%s/%s#L%s", s.Module, o.SourceRef, file, line)
		}
	}
	if o.Perf != nil {
		p, _ := o.Perf(r)
		v.Perf = viewPerf(p)
	}
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
	var gate Gate
	for _, g := range s.Gates {
		if g.Package == r.Package {
			gate = g
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

var nonAnchor = regexp.MustCompile(`[^a-z0-9]+`)

// anchorKey is a route's key as an HTML id.
func anchorKey(r Route) string {
	return strings.Trim(nonAnchor.ReplaceAllString(strings.ToLower(Key(r)), "-"), "-")
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

var funcs = template.FuncMap{
	"join":   strings.Join,
	"prose":  prose,
	"status": statusText,
	"anchor": func(name string) string {
		return "type-" + strings.NewReplacer(".", "-", "[", "", "]", "").Replace(name)
	},
	"typeLink": typeLink,
}

const intro = `
<h2 class="title is-4">API</h2>
<p class="subtitle is-6 has-text-grey">Read off the code at build time: every route the servers register and what each handler reads and answers. Branches are not followed, so a response listed is one the handler can write on some path through it. The same walk as <a href="v1/openapi.json"><code>/v1/openapi.json</code></a>.</p>
`

const indexBody = `
{{$perf := .Perf}}{{range .Packages}}
<div class="box">
<h3 class="title is-5"><code>{{.Package.Path}}</code></h3>
{{if .Package.Doc}}<div class="content is-small">{{prose .Package.Doc}}</div>{{end}}
{{if .Gate.Wrapper}}<p class="is-size-7 has-text-grey mb-3"><strong>Gate</strong> <code>{{.Gate.Wrapper}}</code>: every route but {{range $i, $p := .Gate.Open}}{{if $i}}, {{end}}<code>{{$p}}</code>{{end}} needs {{range $i, $c := .Gate.Reads.Cookies}}{{if $i}} or {{end}}the <code>{{$c}}</code> cookie{{end}}{{range .Gate.Reads.Headers}}{{if eq . "Authorization"}} or a bearer token{{end}}{{end}}.</p>{{if .Gate.Doc}}<div class="content is-small">{{prose .Gate.Doc}}</div>{{end}}{{end}}
{{range .Sections}}
{{if .Title}}<h4 class="title is-6 mt-4 mb-1">{{.Title}}</h4>{{if .Doc}}<p class="is-size-7 has-text-grey mb-2">{{.Doc}}</p>{{end}}{{end}}
<table class="table is-fullwidth is-narrow is-hoverable">
<thead><tr><th style="width:7em">Method</th><th style="width:22em">Path</th><th>Route</th>{{if $perf}}<th style="width:6em">Requests</th><th style="width:5em">p95</th>{{end}}</tr></thead>
<tbody>
{{range .Routes}}
<tr><td><code>{{join .Methods " "}}</code></td><td><a href="{{.URL}}"><code>{{.Path}}</code></a></td><td>{{.Summary}}{{if .Action}} <span class="tag is-light is-warning is-small">{{.Action}}</span>{{end}}{{if .Unknowns}} <span class="tag is-light is-danger is-small">unknown</span>{{end}}</td>{{if .Perf}}{{if .Perf.Count}}<td>{{.Perf.Count}}</td><td>{{.Perf.P95}}</td>{{else}}<td class="has-text-grey">0</td><td></td>{{end}}{{end}}</tr>
{{end}}
</tbody></table>
{{end}}
</div>
{{end}}
`

const detailBody = `
<div class="content is-small mb-2"><a href="{{.IndexURL}}">API</a> › <code>{{.Package.Name}}</code></div>
<h2 class="title is-4"><code>{{join .Methods " "}}</code> <code>{{.Path}}</code></h2>
<p class="subtitle is-6">{{.Summary}}</p>
<div class="box">
<div class="content is-small">
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
<tr><th>Source</th><td>{{if .SourceURL}}<a href="{{.SourceURL}}"><code>{{.Pos}}</code></a>{{else}}<code>{{.Pos}}</code>{{end}}{{if .Handler}} <code>{{.Handler}}</code>{{end}}{{range .Wrappers}} in <code>{{.}}</code>{{end}}{{if .Via}} via <code>{{.Via}}</code>{{end}}{{if .Registrar}}, registered in <code>{{.Registrar}}</code>{{end}}</td></tr>
{{if .Unknowns}}<tr><th>Unknown</th><td>{{join .Unknowns "; "}}</td></tr>{{end}}
</table>
{{if .Perf}}<h3 class="title is-6 mt-4 mb-1">Performance</h3>
{{if .Perf.Count}}<p>{{.Perf.Count}} requests since the process started: p50 {{.Perf.P50}}, p95 {{.Perf.P95}}, max {{.Perf.Max}}; {{.Perf.PeakInFlight}} in flight at the peak; {{.Perf.PerSecond}} a second over the last minute.</p>
<p>At the peak in flight and the p95 this route completes about {{.Perf.Capacity}} requests a second: with a client acting once every {{.Perf.ThinkSeconds}} seconds, about {{.Perf.Clients}} clients. The peak is what it has been asked for, not a limit: load it harder to find one.</p>
{{else}}<p class="has-text-grey">This route has not been served since the process started.</p>{{end}}{{end}}
</div>
</div>
`

const typesBody = `
<div class="box">
<h3 class="title is-5">Types</h3>
<p class="is-size-7 has-text-grey">Every type the bodies above are made of, from the Go declarations and their comments.</p>
{{range .}}
<h4 class="title is-6 mt-4 mb-1" id="{{anchor .Name}}"><code>{{.Name}}</code> <span class="has-text-grey has-text-weight-normal">{{.Kind}}{{if .Format}} ({{.Format}}){{end}}</span></h4>
{{if .Doc}}<div class="content is-small">{{prose .Doc}}</div>{{end}}
{{if .Enum}}<p class="is-size-7">One of: {{range $i, $e := .Enum}}{{if $i}}, {{end}}<code>{{if $e}}{{$e}}{{else}}""{{end}}</code>{{end}}</p>{{end}}
{{if .Fields}}<table class="table is-narrow is-size-7"><thead><tr><th>Field</th><th>Type</th><th></th></tr></thead><tbody>
{{range .Fields}}<tr><td><code>{{.Name}}</code></td><td>{{typeLink .Type}}{{if .Nullable}} or null{{end}}{{if .Optional}} <span class="has-text-grey">optional</span>{{end}}</td><td>{{.Doc}}</td></tr>
{{end}}</tbody></table>{{end}}
{{if .Elem}}<p class="is-size-7">of {{typeLink .Elem}}</p>{{end}}
{{end}}
</div>
`

var (
	indexTmpl  = template.Must(template.New("index").Funcs(funcs).Parse(intro + indexBody))
	detailTmpl = template.Must(template.New("detail").Funcs(funcs).Parse(detailBody))
	typesTmpl  = template.Must(template.New("types").Funcs(funcs).Parse(typesBody))
	pageTmpl   = template.Must(template.New("page").Funcs(funcs).Parse(intro + `{{with .Index}}` + indexBody + `{{end}}` +
		`<h3 class="title is-5 mt-5">Routes</h3>{{range .Routes}}<div id="route-{{.Anchor}}">` + detailBody + `</div>{{end}}{{with .Types}}` + typesBody + `{{end}}`))
)

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
