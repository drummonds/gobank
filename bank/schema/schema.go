// Package schema is the shape of a component's migrations. Each component
// owns its tables (ADR-0001) and the migrations that bring them up to date
// (ADR-0003), published as a Component for the wiring to apply at start.
package schema

// Migration is one version step: the statements that take a component's
// tables from the version before to this one, applied in one transaction.
type Migration struct {
	Version    int
	Statements []string
}

// Component is a component's migrations in version order, 1, 2, 3…
type Component struct {
	Name       string
	Migrations []Migration
}
