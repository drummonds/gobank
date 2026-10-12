// Package adr holds the architecture decision records as embedded markdown,
// one numbered file per decision, with the diagrams each shows and their
// sources, so the running system can serve its own documentation whole.
package adr

import (
	"embed"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.md *.svg *.goluca
var files embed.FS

// Record is one architecture decision.
type Record struct {
	Number   int
	Slug     string // file name without number and extension
	Title    string // first heading, without the ADR-NNNN prefix
	Status   string // the "Status:" line
	Date     string // the "Date:" line's day, 2006-01-02
	Markdown string
}

var (
	fileName = regexp.MustCompile(`^(\d{4})-(.+)\.md$`)
	heading  = regexp.MustCompile(`(?m)^# ADR-\d{4}: (.+)$`)
	status   = regexp.MustCompile(`(?m)^Status: (.+)$`)
	date     = regexp.MustCompile(`(?m)^Date: (\d{4}-\d{2}-\d{2})`)
)

// assetTypes is what an ADR may show beside its text, by extension.
var assetTypes = map[string]string{".svg": "image/svg+xml", ".goluca": "text/plain; charset=utf-8"}

// Asset is a diagram or a diagram's source by file name, with its content
// type; false for anything that is not one.
func Asset(name string) ([]byte, string, bool) {
	kind, ok := assetTypes[path.Ext(name)]
	if !ok || name != path.Base(name) {
		return nil, "", false
	}
	body, err := files.ReadFile(name)
	if err != nil {
		return nil, "", false
	}
	return body, kind, true
}

// All returns every ADR in number order.
func All() []Record {
	entries, _ := files.ReadDir(".")
	var recs []Record
	for _, e := range entries {
		m := fileName.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		body, _ := files.ReadFile(e.Name())
		rec := Record{Number: n, Slug: m[2], Markdown: string(body)}
		if h := heading.FindStringSubmatch(rec.Markdown); h != nil {
			rec.Title = strings.TrimSpace(h[1])
		}
		if s := status.FindStringSubmatch(rec.Markdown); s != nil {
			rec.Status = strings.TrimSpace(s[1])
		}
		if d := date.FindStringSubmatch(rec.Markdown); d != nil {
			rec.Date = d[1]
		}
		recs = append(recs, rec)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Number < recs[j].Number })
	return recs
}

// ByNumber returns one ADR.
func ByNumber(n int) (Record, bool) {
	for _, r := range All() {
		if r.Number == n {
			return r, true
		}
	}
	return Record{}, false
}

// BySlug returns one ADR by its file slug, e.g. "contract-views".
func BySlug(slug string) (Record, bool) {
	for _, r := range All() {
		if r.Slug == slug {
			return r, true
		}
	}
	return Record{}, false
}
