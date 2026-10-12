package adr

import (
	"strings"
	"testing"
)

// Every ADR is a numbered markdown file whose first heading carries the
// number and title, followed by a status line. The list is in number order
// with no gaps, so a missing decision is a visible hole.
func TestRecordsParsedInOrder(t *testing.T) {
	recs := All()
	if len(recs) == 0 {
		t.Fatal("no ADRs found")
	}
	for i, r := range recs {
		if r.Number != i+1 {
			t.Errorf("ADR %d at position %d: numbers must be sequential from 1", r.Number, i)
		}
		if r.Title == "" || r.Status == "" || r.Slug == "" || r.Markdown == "" {
			t.Errorf("ADR %d incomplete: %+v", r.Number, r)
		}
	}
	first, ok := ByNumber(1)
	if !ok || first.Slug != "contract-views" || first.Title != "Contract views" {
		t.Errorf("ADR-0001 = %+v, want contract-views", first)
	}
	if _, ok := ByNumber(999); ok {
		t.Error("unknown ADR reported as found")
	}
}

// An ADR's diagrams and their sources are embedded beside its markdown,
// so the running system serves the record whole; a record carries the
// date of its "Date:" line. Nothing else in the directory is served.
func TestAssetsAndDates(t *testing.T) {
	svg, kind, ok := Asset("0008-decided.svg")
	if !ok || kind != "image/svg+xml" || !strings.Contains(string(svg), "<svg") {
		t.Errorf("Asset(0008-decided.svg) = %d bytes, %q, %v", len(svg), kind, ok)
	}
	if src, kind, ok := Asset("0008-decided.goluca"); !ok || kind != "text/plain; charset=utf-8" || len(src) == 0 {
		t.Errorf("Asset(0008-decided.goluca) = %d bytes, %q, %v", len(src), kind, ok)
	}
	for _, name := range []string{"adr.go", "0001-contract-views.md", "../go.mod", "", "no-such.svg"} {
		if _, _, ok := Asset(name); ok {
			t.Errorf("Asset(%q) served", name)
		}
	}
	first, _ := ByNumber(1)
	if first.Date != "2026-09-26" {
		t.Errorf("ADR-0001 date = %q, want 2026-09-26", first.Date)
	}
	for _, r := range All() {
		if r.Date == "" {
			t.Errorf("ADR-%04d has no date", r.Number)
		}
	}
}
