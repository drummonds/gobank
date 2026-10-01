package main

import (
	"strings"

	dbexplorer "git.bytestone.uk/hum3/go-dbexplorer"
)

// explorer returns the DB explorer (extracted to go-dbexplorer) bound to
// the demo's database and mounted under /internal/explorer, divided into
// the bank's components so it can be scoped to one (/c/{component}).
func (ds *DemoState) explorer() *dbexplorer.Explorer {
	return &dbexplorer.Explorer{
		DB:       ds.DB(),
		BasePath: "/internal/explorer",
		Catalog:  explorerCatalog,
		Annotate: contractBadge,
	}
}

// explorerCatalog is the component registry as the explorer reads it.
var explorerCatalog = func() dbexplorer.StaticCatalog {
	var c dbexplorer.StaticCatalog
	for _, comp := range components {
		c = append(c, dbexplorer.Component{Name: comp.Name, Tables: comp.Tables, Views: comp.Views})
	}
	return c
}()

// BuildExplorerPage renders the explorer page for an explorer URL — the
// index for /internal/explorer, a table for /internal/explorer/<table> —
// honouring every query parameter the explorer's own links carry (page,
// sort, dir, trunc, filter, value). Server and WASM both route through it.
func (ds *DemoState) BuildExplorerPage(rawURL string) string {
	return ds.explorer().Render(rawURL)
}

// contractBadge marks a contract view (ADR-0001); the explorer itself tags
// each table and view with its owning component from the catalog.
func contractBadge(name string) string {
	if strings.HasPrefix(name, "contract_") {
		return `<span class="tag is-success is-light">contract view</span>`
	}
	return ""
}
