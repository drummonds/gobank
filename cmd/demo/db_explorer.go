package main

import (
	"strings"

	dbexplorer "git.bytestone.uk/hum3/go-dbexplorer"
)

// explorer returns the DB explorer (extracted to go-dbexplorer) bound to
// the demo's database and mounted under /internal/explorer.
func (ds *DemoState) explorer() *dbexplorer.Explorer {
	return &dbexplorer.Explorer{
		DB:       ds.DB(),
		BasePath: "/internal/explorer",
		Annotate: ownershipBadge,
	}
}

// BuildExplorerPage renders the explorer page for an explorer URL — the
// index for /internal/explorer, a table for /internal/explorer/<table> —
// honouring every query parameter the explorer's own links carry (page,
// sort, dir, trunc, filter, value). Server and WASM both route through it.
func (ds *DemoState) BuildExplorerPage(rawURL string) string {
	return ds.explorer().Render(rawURL)
}

// ownershipBadge labels a table with its owning component, or a view as a
// contract view (ADR-0001). Unowned names get no badge.
func ownershipBadge(name string) string {
	if strings.HasPrefix(name, "contract_") {
		return `<span class="tag is-success is-light">contract view</span>`
	}
	if owner := componentOf(name); owner != "" {
		return `<span class="tag is-light">` + owner + `</span>`
	}
	return ""
}
