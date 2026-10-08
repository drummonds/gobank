package staff

import (
	"context"
	"strings"

	dbexplorer "git.bytestone.uk/hum3/go-dbexplorer"
)

// explorer is the DB explorer (go-dbexplorer) bound to the console's
// database and mounted under /internal/explorer, divided into the bank's
// components from the registry so it can be scoped to one (/c/{component}).
// The explorer's links are absolute, so they carry the scope.
func (s *Site) explorer() *dbexplorer.Explorer {
	return &dbexplorer.Explorer{
		DB:       s.cfg.Console.DB(),
		BasePath: s.cfg.Scope + "internal/explorer",
		Catalog:  s.catalog,
		Annotate: contractBadge,
		Authoriser: dbexplorer.AuthoriserFunc(func(ctx context.Context, component string) bool {
			return RoleFrom(ctx).CanViewComponent(component)
		}),
	}
}

// explorerCatalog is the component registry as the explorer reads it.
func explorerCatalog(components []Component) dbexplorer.StaticCatalog {
	var c dbexplorer.StaticCatalog
	for _, comp := range components {
		c = append(c, dbexplorer.Component{Name: comp.Name, Tables: comp.Tables, Views: comp.Views})
	}
	return c
}

// BuildExplorerPage renders the explorer page for an explorer URL — the
// index for /internal/explorer, a table for /internal/explorer/<table> —
// honouring every query parameter the explorer's own links carry (page,
// sort, dir, trunc, filter, value). ctx carries the viewer's role (see
// WithRole), which decides the components shown; rawURL is scope-relative
// as the handler saw it.
func (s *Site) BuildExplorerPage(ctx context.Context, rawURL string) string {
	return s.explorer().Render(ctx, s.cfg.Scope+strings.TrimPrefix(rawURL, "/"))
}

// contractBadge marks a contract view (ADR-0001); the explorer itself tags
// each table and view with its owning component from the catalog.
func contractBadge(name string) string {
	if strings.HasPrefix(name, "contract_") {
		return `<span class="tag is-success is-light">contract view</span>`
	}
	return ""
}
