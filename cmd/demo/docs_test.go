package main

import (
	"html"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/adr"
)

// The documentation page is generated from the registry: every component,
// its tables and contract views, the pre-rule debt, and every ADR appear.
func TestDocsPageCoversRegistryAndADRs(t *testing.T) {
	page := BuildDocsHTML()
	for _, c := range components {
		if !strings.Contains(page, c.Name) || !strings.Contains(page, html.EscapeString(c.Purpose)) {
			t.Errorf("docs page missing component %s", c.Name)
		}
		for _, tbl := range c.Tables {
			if !strings.Contains(page, tbl) {
				t.Errorf("docs page missing table %s of %s", tbl, c.Name)
			}
		}
		for _, v := range c.Views {
			if !strings.Contains(page, v) {
				t.Errorf("docs page missing contract view %s of %s", v, c.Name)
			}
		}
	}
	for _, d := range contractDebt {
		if !strings.Contains(page, d.File) || !strings.Contains(page, d.Table) {
			t.Errorf("docs page missing debt entry %s/%s", d.File, d.Table)
		}
	}
	for _, r := range adr.All() {
		if !strings.Contains(page, r.Title) || !strings.Contains(page, "/about/docs/adr/"+r.Slug) {
			t.Errorf("docs page missing ADR %d %q", r.Number, r.Title)
		}
	}
}

// An ADR renders as HTML from its markdown; an unknown one is reported.
func TestADRPage(t *testing.T) {
	page, ok := BuildADRHTML("contract-views")
	if !ok {
		t.Fatal("contract-views not found")
	}
	if !strings.Contains(page, "<h1") || !strings.Contains(page, "Contract views") || !strings.Contains(page, "Accepted") {
		t.Errorf("ADR page not rendered: %.200s", page)
	}
	if _, ok := BuildADRHTML("no-such-adr"); ok {
		t.Error("unknown ADR reported as found")
	}
}

// The DB explorer labels every table with its owning component and marks
// contract views, so an at-risk query knows whose table it is reading.
func TestExplorerShowsOwnership(t *testing.T) {
	ds := NewDemoState()
	page := ds.BuildExplorerPage("/internal/explorer")
	if !strings.Contains(page, `>customer_accounts</a> <span class="tag is-light">customers</span>`) {
		t.Errorf("explorer index does not badge customer_accounts with its owner: %.300s", page)
	}
}
