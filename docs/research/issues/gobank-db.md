# Issue draft — hum3/gobank-db

**Title:** README overclaims vs the 2-function implementation; Migrate splits on ';'

Found in a July 2026 cold review of the gobank family (see
gobank `docs/research/cold-review-2026-07.md`).

The package is two functions (`Open`, `Migrate`, ~40 lines) but the docs claim
considerably more:

- README.md:9-15: "Shared Schema — common database schema used across gobank
  services" (no schema in the package; the only schema is test-only per the
  changelog), "Connection Management — pooled connections" (`Open` is a thin
  `sql.Open` wrapper, no pool config), "Migration Support — schema versioning"
  (`Migrate` has no versioning).
- index.md:3 repeats "connection management, migration orchestration,
  dual-backend support"; ROADMAP.md lists connection factory / migration
  runner / namespace isolation as v0.1 — none implemented.

Either trim the docs to what exists or treat the README as the roadmap and
say so.

Also:
- `Migrate` splits the schema on `;` (db.go:28) — breaks on any semicolon
  inside string literals, `CREATE FUNCTION` bodies or dollar-quoted blocks.
  Fine for simple DDL; worth documenting the limitation.
- Docs-site link uses `h3-gobank-db.statichost.eu` (README.md:40, index.md:15)
  while every sibling repo uses `.page` — check which is intended.
