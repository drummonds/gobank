package main

import (
	"context"
	"strings"

	dbexplorer "git.bytestone.uk/hum3/go-dbexplorer"
)

// explorer returns the DB explorer (extracted to go-dbexplorer) bound to
// the demo's database and mounted under /internal/explorer, divided into
// the bank's components so it can be scoped to one (/c/{component}).
// scope is the path the demo is mounted under; the explorer's links are
// absolute, so they carry it.
func (ds *DemoState) explorer(scope string) *dbexplorer.Explorer {
	return &dbexplorer.Explorer{
		DB:       ds.DB(),
		BasePath: scope + "internal/explorer",
		Catalog:  explorerCatalog,
		Annotate: contractBadge,
		Authoriser: dbexplorer.AuthoriserFunc(func(ctx context.Context, component string) bool {
			return roleFrom(ctx).CanViewComponent(component)
		}),
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
// sort, dir, trunc, filter, value). ctx carries the viewer's role (see
// withRole), which decides the components shown; rawURL is scope-relative
// as the handler saw it.
func (ds *DemoState) BuildExplorerPage(ctx context.Context, scope, rawURL string) string {
	return ds.explorer(scope).Render(ctx, scope+strings.TrimPrefix(rawURL, "/"))
}

// contractBadge marks a contract view (ADR-0001); the explorer itself tags
// each table and view with its owning component from the catalog.
func contractBadge(name string) string {
	if strings.HasPrefix(name, "contract_") {
		return `<span class="tag is-success is-light">contract view</span>`
	}
	return ""
}
