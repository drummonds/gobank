# Issue draft — hum3/gobank

**Title:** Cold review July 2026: demo bugs (money formatting, in-memory DB pooling, import) and doc contradictions

Full write-up: `docs/research/cold-review-2026-07.md` (this repo). Highlights:

**Bugs (verified with repro tests against the pinned deps):**

1. `fmtMoney` carry bug — `cmd/demo/format.go:38` prints `£1.100` for 1.999,
   `£0.100` for 0.995, `£999.100` for 999.999: `int64(frac*100+0.5)` can hit
   100 and `%02d` doesn't carry. Fix: round to integer pence first
   (`p := int64(math.Round(v*100)); p/100, p%100`).
2. In-memory DB vs connection pool — `cmd/demo/db.go:33` opens pglike with
   `file::memory:?_pragma=temp_store(2)` and no `SetMaxOpenConns(1)`.
   go-postgres only special-cases the exact `":memory:"` DSN, so a second
   pooled connection gets a **separate empty database** (verified: "no such
   table" on conn 2). Works today only while requests stay sequential.
3. `.goluca` import only writes the ledger — `main_wasm.go:49` / `export.go:51`
   import into `sim.Ledger`; in-memory customers/balances/txlog are untouched,
   so the UI still shows pre-import state. Export→Reset→Import doesn't
   round-trip.
4. Fresh checkout of cmd/demo doesn't build: `//go:embed diagrams/*.svg`
   (`models.go:8-12`) targets gitignored files that only exist after
   `task diagrams`.
5. Committed binaries: `cmd/demo/demo.test` (44 MB) and
   `cmd/chartcompare/chartcompare` (5.6 MB) are tracked
   (`git rm --cached` + push; .gitignore now covers demo.test).
6. `task test:wasm` / `task check` fail on Node ≥ 22 (verified on 22.17):
   `cmd/demo/wasm_test.js:12` assigns `globalThis.crypto = webcrypto`, but
   modern Node exposes `crypto` as a getter-only global. Guard the polyfill
   with `if (!globalThis.crypto)`.

**Doc contradictions to resolve (your call which side wins):**

- ROADMAP.md Phase 2/3 = Kubernetes+AlloyDB / CockroachDB, but
  `cmd/demo/project_data.json` non-goals say "no Kubernetes/CloudSQL".
- ROADMAP "Done" vs project_data `done:false` disagree on RBAC + accounting;
  ROADMAP "To Do" still lists savings accounts & customer model (implemented).
- project_data.json says deployment = "GitHub Pages" (actual: statichost) and
  links GitHub mirrors instead of canonical Codeberg.
- `cmd/demo/diagrams/{container,system_context}.d2` show BFF / mock-fps /
  reconciliation / persistent storage that don't exist; the app's Models page
  presents an architecture it doesn't implement — label as "target" or trim.

**Native vs wasm divergence:** no RBAC under wasm; gilt Buy form is dead in
the browser (no `goBuyGilt` export); Charts nav present only in wasm; phone
preview only native.
