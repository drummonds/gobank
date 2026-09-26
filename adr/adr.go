// Package adr holds the architecture decision records as embedded markdown,
// one numbered file per decision, so the running system can serve its own
// documentation.
package adr

import (
	"embed"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.md
var files embed.FS

// Record is one architecture decision.
type Record struct {
	Number   int
	Slug     string // file name without number and extension
	Title    string // first heading, without the ADR-NNNN prefix
	Status   string // the "Status:" line
	Markdown string
}

var (
	fileName = regexp.MustCompile(`^(\d{4})-(.+)\.md$`)
	heading  = regexp.MustCompile(`(?m)^# ADR-\d{4}: (.+)$`)
	status   = regexp.MustCompile(`(?m)^Status: (.+)$`)
)

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
