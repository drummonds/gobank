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
		Postgres: ds.dbIsPostgres,
		BasePath: "/internal/explorer",
		Annotate: ownershipBadge,
	}
}

// BuildExplorerHTML renders the DB explorer overview: all tables with
// row/column counts and FK relationships.
func (ds *DemoState) BuildExplorerHTML() string {
	return ds.explorer().IndexHTML()
}

// BuildExplorerTableHTML renders the detail view for a single table:
// schema, FKs, indexes, and paginated data. trunc shortens ID/text cells
// and is carried through sort/pagination links as ?trunc=1.
func (ds *DemoState) BuildExplorerTableHTML(name string, page int, sort string, dir string, trunc bool) string {
	return ds.explorer().TableHTML(name, page, sort, dir, trunc)
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
