# ADR-0003: Upgrade in place, with recorded downtime, until stage 8

Status: Accepted
Date: 2026-10-03
Stage: ADR-0002 stage 2 onwards

## Context

Every redeploy of the demo is a fresh run: `initDB` drops every public
table at start because the in-memory simulation state is authoritative
(`cmd/demo/db.go`). ADR-0002 stage 2 makes the database hold every fact,
and its done-when is that a restart resumes the run. From that point a
release is an upgrade of a live system with state to protect, and we want
practice at that long before the bank has real customers.

Zero-downtime deployment needs two versions of the program serving the
same database at once. The demo is one process holding its state behind
two mutexes; two processes against one database is ADR-0002 stage 8. So
the question is what deployment strategy to use for stages 2 to 7.

## Options

| Option | Complexity | Testability | Domain clarity | Operational risk |
|---|---|---|---|---|
| A. In place: stop, replace, start on the same host; downtime measured and recorded per upgrade; rollback rehearsed | low — the existing gobank-deploy redeploy | high — resume, downtime and rollback are each observable on Hetzner | high — a bank's change window, named as such | low — downtime is bounded and known |
| B. Blue-green now | high — pulls the stage 8 split (bank/simulation, many processes) forward past stages 2–7 | medium — needs the multi-process topology before it can be tested | low — the demo's single-process shape is not the thing being tested | medium — a big reorder of the ADR-0002 sequence |
| C. Keep fresh-run deploys until stage 8 | none | none — nothing about upgrades is tested | low — the demo never behaves like a system with a past | high — six stages of schema change with no migration discipline, met all at once at stage 8 |

## Decision

A. From stage 2 story (b) onward, every story is deployed as an upgrade of
the running Hetzner demo: the run resumes, the downtime is recorded, and
rolling back to the previous release from gobank-deploy's release store is
rehearsed. Schema changes are versioned and written expand/contract so a
rollback is always possible. Downtime is accepted and bounded, not hidden:
this is deployment level 2 in the manual's
[maturity ladder](https://man.bytestone.uk/maturity.html), where the
level is defined by its reliability metrics (downtime per upgrade, no
loss of committed facts, recovery by restart or rollback). Stage 8 moves
to level 3, blue-green, and the recorded downtime is the number it drives
to zero.

## Consequences

Easier: stage 2 gets a concrete first story (resume) and a standing
acceptance test (the upgrade drill) that every later story reuses;
migrations become a habit six stages before they are unavoidable.

Harder: the simulation must be slow enough for an upgrade to land inside
a day, so a simulated day length setting comes first; the demo can no
longer treat a leftover database as garbage, so every table needs an
owner for its migration (ADR-0001's component rule extends to schema
versions).

Given up: nothing is served during the restart. That is deliberate and
ends at stage 8.
