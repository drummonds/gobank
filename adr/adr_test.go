package adr

import "testing"

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
