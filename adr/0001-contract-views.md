# ADR-0001: Contract views

Status: Accepted
Date: 2026-09-26

## Context

The model bank is one database shared by several functions: the ledger
(go-luca), customers, products, treasury and, next, payments. Any code can
query any table, so a table's shape is frozen the moment a second function
reads it, and nothing records which reads are load-bearing. Ad hoc queries
in the DB explorer are useful and must stay possible.

## Decision

Every table belongs to exactly one **component**, declared in the component
registry (`cmd/demo/components.go`). A component's tables are **internal**.
It publishes a **contract view** (a SQL view named `contract_<component>`,
or `contract_<component>_<name>` where it needs several) as the surface
other code reads, and a Go **API** for what a view cannot answer: commands,
writes, and derived figures.

| Reader | Own internal table | Another component's internal table | Contract view | Component API |
|---|---|---|---|---|
| Component code | yes | no | yes | yes |
| Cross-cutting code (reports, dashboard, book) | n/a | no | yes | yes |
| DB explorer, ad hoc SQL | at risk | at risk | yes | n/a |

"No" is enforced by a test that scans every SQL string literal in the source
and fails on a reference to another component's internal table. "At risk"
means the query may run but nothing promises the table will look the same
tomorrow.

Cross-reads that existed before this decision are listed in a baseline in
the registry. They stay allowed until fixed, are shown on the documentation
page as debt, and the test fails if the list grows or if an entry no longer
occurs, so it only shrinks.

Until the banking core is extracted from `cmd/demo` into packages, a
component is a set of files in `package main`; ownership of a file is
declared in the registry. Once components are packages, Go's package
boundary carries the API rule and the test keeps checking SQL.

## Consequences

Each component can reshape its tables freely as long as its contract view
still holds. Adding a component means registering it, which is also what
puts it on the documentation page: the registry is the single source for
the docs, the rule and the explorer. Views cost nothing in-browser (pglike
supports them) and map to a `contract` schema on PostgreSQL later. The
baseline is visible technical debt with a named owner per entry.
