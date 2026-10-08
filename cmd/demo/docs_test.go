package main

import (
	"git.bytestone.uk/hum3/gobank/bff/staff"
	"html"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/adr"
)

// The documentation page is generated from the registry: every component,
// its tables and contract views, the pre-rule debt, and every ADR appear.
func TestDocsPageCoversRegistryAndADRs(t *testing.T) {
	page := staff.BuildDocsHTML(components, contractDebt)
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
		if !strings.Contains(page, r.Title) || !strings.Contains(page, `href="about/docs/adr/`+r.Slug+`"`) {
			t.Errorf("docs page missing ADR %d %q", r.Number, r.Title)
		}
	}
}

// An ADR renders as HTML from its markdown; an unknown one is reported.
func TestADRPage(t *testing.T) {
	page, ok := staff.BuildADRHTML("contract-views")
	if !ok {
		t.Fatal("contract-views not found")
	}
	if !strings.Contains(page, "<h1") || !strings.Contains(page, "Contract views") || !strings.Contains(page, "Accepted") {
		t.Errorf("ADR page not rendered: %.200s", page)
	}
	if _, ok := staff.BuildADRHTML("no-such-adr"); ok {
		t.Error("unknown ADR reported as found")
	}
}

// The DB explorer labels every table with its owning component, linking
// that component's scope, and marks contract views, so an at-risk query
// knows whose table it is reading.
func TestExplorerShowsOwnership(t *testing.T) {
	ds := NewDemoState()
	page := newSite(ds).BuildExplorerPage(t.Context(), "/internal/explorer")
	if !strings.Contains(page, `>customer_accounts</a> <a class="tag is-light" href="/internal/explorer/c/customers">customers</a>`) {
		t.Errorf("explorer index does not tag customer_accounts with its owner: %.300s", page)
	}
	if !strings.Contains(page, `>contract_payments</a> <a class="tag is-light" href="/internal/explorer/c/payments">payments</a> <span class="tag is-success is-light">contract view</span>`) {
		t.Error("explorer index does not tag contract_payments as payments' contract view")
	}
}

// Scoped to one component, the explorer shows only the tables it owns and
// the contract views it publishes (ADR-0001).
func TestExplorerComponentScope(t *testing.T) {
	ds := NewDemoState()
	page := newSite(ds).BuildExplorerPage(t.Context(), "/internal/explorer/c/payments")
	for _, want := range []string{`href="/internal/explorer/c/payments/payments"`, `href="/internal/explorer/c/payments/contract_payments"`} {
		if !strings.Contains(page, want) {
			t.Errorf("payments scope missing %q", want)
		}
	}
	if strings.Contains(page, ">customer_accounts</a>") {
		t.Error("payments scope lists the customers component's table")
	}
}
