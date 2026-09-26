package main

import (
	"bytes"
	"fmt"
	"html"
	"strings"

	"git.bytestone.uk/hum3/gobank/adr"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// The documentation page is generated from the component registry and the
// embedded ADRs, so it cannot drift from the code it describes.

// BuildDocsHTML renders the system documentation: components with their
// tables and contract views, the contract-view rule, the pre-rule debt and
// the architecture decision records.
func BuildDocsHTML() string {
	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">System Documentation</h2>`)
	s.WriteString(`<p class="subtitle is-6 has-text-grey">Generated from the component registry and the ADRs compiled into this build. `)
	s.WriteString(`Structure diagrams are on the <a href="/about/models">Models</a> page.</p>`)

	s.WriteString(`<div class="box"><h3 class="title is-5">Components</h3>`)
	s.WriteString(`<p class="has-text-grey mb-3">Each component owns its tables. Other code reads them only through the component's contract views or API (<a href="/about/docs/adr/contract-views">ADR-0001</a>).</p>`)
	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped">
<thead><tr><th>Component</th><th>Purpose</th><th>Internal tables</th><th>Contract views</th><th>Files</th><th>Library</th></tr></thead><tbody>`)
	for _, c := range components {
		s.WriteString(fmt.Sprintf(`<tr><td><strong>%s</strong></td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			html.EscapeString(c.Name), html.EscapeString(c.Purpose),
			codeList(c.Tables, "none yet"), codeList(c.Views, "none yet"),
			codeList(c.Files, "library only"), html.EscapeString(c.Library)))
	}
	s.WriteString(`</tbody></table></div></div>`)

	s.WriteString(`<div class="box"><h3 class="title is-5">Who may read what</h3>`)
	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-narrow">
<thead><tr><th>Reader</th><th>Own internal table</th><th>Another component's internal table</th><th>Contract view</th><th>Component API</th></tr></thead><tbody>
<tr><td>Component code</td><td>yes</td><td><span class="tag is-danger is-light">no</span> (test fails)</td><td>yes</td><td>yes</td></tr>
<tr><td>Cross-cutting code (reports, dashboard, book)</td><td>n/a</td><td><span class="tag is-danger is-light">no</span> (test fails)</td><td>yes</td><td>yes</td></tr>
<tr><td>DB explorer, ad hoc SQL</td><td><span class="tag is-warning is-light">at risk</span></td><td><span class="tag is-warning is-light">at risk</span></td><td>yes</td><td>n/a</td></tr>
</tbody></table></div></div>`)

	s.WriteString(`<div class="box"><h3 class="title is-5">Pre-rule debt</h3>`)
	if len(contractDebt) == 0 {
		s.WriteString(`<p class="has-text-grey">None. Every cross-component read goes through a contract view or API.</p>`)
	} else {
		s.WriteString(fmt.Sprintf(`<p class="has-text-grey mb-3">%d cross-reads that predate ADR-0001. Each is allowed until replaced; the list can only shrink.</p>`, len(contractDebt)))
		s.WriteString(`<table class="table is-narrow"><thead><tr><th>File</th><th>Reads</th><th>Owned by</th></tr></thead><tbody>`)
		for _, d := range contractDebt {
			s.WriteString(fmt.Sprintf(`<tr><td><code>%s</code></td><td><code>%s</code></td><td>%s</td></tr>`,
				html.EscapeString(d.File), html.EscapeString(d.Table), html.EscapeString(componentOf(d.Table))))
		}
		s.WriteString(`</tbody></table>`)
	}
	s.WriteString(`</div>`)

	s.WriteString(`<div class="box"><h3 class="title is-5">Architecture decision records</h3>`)
	s.WriteString(`<table class="table is-narrow is-fullwidth"><thead><tr><th>ADR</th><th>Title</th><th>Status</th></tr></thead><tbody>`)
	for _, r := range adr.All() {
		s.WriteString(fmt.Sprintf(`<tr><td>%04d</td><td><a href="/about/docs/adr/%s">%s</a></td><td>%s</td></tr>`,
			r.Number, html.EscapeString(r.Slug), html.EscapeString(r.Title), html.EscapeString(r.Status)))
	}
	s.WriteString(`</tbody></table></div>`)
	return s.String()
}

// BuildADRHTML renders one ADR from its markdown.
func BuildADRHTML(slug string) (string, bool) {
	r, ok := adr.BySlug(slug)
	if !ok {
		return `<div class="notification is-warning">ADR not found.</div>`, false
	}
	var body bytes.Buffer
	md := goldmark.New(goldmark.WithExtensions(extension.Table))
	if err := md.Convert([]byte(r.Markdown), &body); err != nil {
		return fmt.Sprintf(`<div class="notification is-danger">ADR %s: %s</div>`, html.EscapeString(slug), html.EscapeString(err.Error())), false
	}
	var s strings.Builder
	s.WriteString(`<p><a href="/about/docs">&larr; Documentation</a></p>`)
	s.WriteString(`<div class="box content">`)
	s.WriteString(body.String())
	s.WriteString(`</div>`)
	return s.String(), true
}

// codeList renders names as <code> items, or a grey placeholder.
func codeList(names []string, empty string) string {
	if len(names) == 0 {
		return `<span class="has-text-grey">` + empty + `</span>`
	}
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = "<code>" + html.EscapeString(n) + "</code>"
	}
	return strings.Join(parts, " ")
}
