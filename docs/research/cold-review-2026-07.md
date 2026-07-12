# Cold Review — gobank family, July 2026

A cold review of gobank (v0.3.39) and its wasm Model Bank, plus the first-party
component repos: gobank-products, go-luca, go-postgres, gotreesitter,
gobanks-customers, gobank-db and lofigui. Component repos were reviewed at
their latest tags; behavioural findings were verified against the **exact
versions pinned by `cmd/demo`** (go-luca v0.2.25, go-postgres v0.5.2,
gobank-products v0.1.5) using standalone repro tests.

**Verified** = reproduced by a test against the pinned versions.
**Confirmed by reading** = unambiguous in source but not executed.
**Suspected** = plausible from source; did not reproduce under test.

## Summary

| # | Finding | Where | Severity | Status |
|---|---------|-------|----------|--------|
| 1 | Small accounts earn zero interest forever; all accounts under-accrue (daily sub-penny truncation, accumulator unused) | go-luca `interest.go` (v0.2.25 and, post-refactor, gobank-products `interest.go`) | High | Verified |
| 2 | `fmtMoney` prints `£1.100` for values ≈ x.995+ (pence field overflows without carry) | cmd/demo/format.go:38 | High (display) | Verified |
| 3 | Demo DSN `file::memory:` + connection pool → each pooled connection can get a separate empty database | cmd/demo/db.go:33 + go-postgres driver.go:36 | High (latent) | Verified |
| 4 | Dual source of truth: float64 balances in memory vs integer pence in ledger; `.goluca` import updates ledger only | cmd/demo (customers.go, demo_state.go, export.go) | Medium | Confirmed by reading |
| 5 | `nextval()` implemented as UPDATE-then-SELECT with name spliced into SQL string | go-postgres driver.go:298-325 | Medium | Suspected (race did not reproduce; splice confirmed) |
| 6 | go-luca `RecordMovement` doesn't persist `knowledge_time`; returned value uses a different clock | go-luca db.go:266 | Medium | Confirmed by reading |
| 7 | `DemoState.DB()` handle used outside the lock while `Reset()` closes/reopens it | cmd/demo treasury.go / demo_state.go | Medium | Confirmed by reading |
| 8 | Unsalted SHA-256 of NI numbers in an indexed plaintext column | gobanks-customers crypto.go:57, schema.go:26 | Medium (privacy) | Confirmed by reading |
| 9 | No key-length validation; README example key is 31 bytes and fails at runtime | gobanks-customers key_provider.go, README.md:57 | Medium | Confirmed by reading |
| 10 | Fresh checkout of cmd/demo cannot `go build` — `//go:embed` targets gitignored SVGs | cmd/demo/models.go:8-12 | Medium (DX) | Verified |
| 11 | Demo-grade auth: everyone is Admin, guessable session IDs, hardcoded PII key, no RBAC at all under wasm | cmd/demo auth.go, main.go, db.go | Documented risk | Confirmed by reading |
| 12 | Native/wasm feature divergence (gilt Buy dead in browser, Charts nav, phone preview) | cmd/demo | Low | Confirmed by reading |
| 13 | ROADMAP.md contradicts project_data.json (Kubernetes both planned and a non-goal; done/not-done disagree) | gobank docs | Docs | Confirmed |
| 14 | Widespread doc drift: broken links, malformed CHANGELOG, wrong module path in gotreesitter README, overclaiming gobank-db README, version pins lagging tags | all repos | Docs | Confirmed |

Repro tests live outside the repo (session scratchpad, `repro_test.go`); results
are quoted inline below so the report is self-contained. Ready-to-paste issue
drafts, one per affected repo, are in `docs/research/issues/` (no Codeberg API
token was available on this machine to file them directly).

---

## 1. Financial correctness

### 1.1 Daily interest truncation — small accounts earn nothing (High, Verified)

`go-luca v0.2.25` `defaultInterest` (interest.go:10-18) computes
`balance × rate / 365` in decimal, then truncates **toward zero** to the
account exponent via `DecimalToInt` (decimal.go:24-27). The comment says it
plainly: *"No accumulator is used."* The `Account.InterestAccumulator` field
(luca.go:128, *"sub-unit fractions at extended precision"*) is plumbed through
schema and scans but is never written by the default path.

Verified against the pinned versions:

```
£100 @ 3%:    daily interest posted = 0p   (exact: 0.8219p/day, £3.00/yr)
£10,000 @ 3%: daily interest posted = 82p  (exact: 82.19p)
InterestAccumulator after accrual = 0      (the 0.19p is discarded)
```

Consequences at 3% gross:
- Any account below ~£121.67 accrues **zero** interest, permanently.
- Larger accounts silently lose up to 1p/day (~70p/yr on £10k, non-compounding
  loss grows with account count — 100k accounts ≈ up to £365k/yr under-accrual).

The post-refactor code (gobank-products v0.1.6 `interest.go:46-52` +
go-luca v0.2.30) has the same truncation and still never uses the accumulator,
so upgrading the pins does not fix this.

**Suggested fix:** accumulate the sub-unit remainder in `InterestAccumulator`
(the field designed for it) and post when it reaches a whole unit; or hold
accruals at extended precision (exponent −5) and settle monthly, which is also
how real deposit products behave.

### 1.2 `fmtMoney` carry bug (High for display, Verified)

`cmd/demo/format.go:38`:

```go
return fmt.Sprintf("%s%s.%02d", prefix, s, int64(frac*100+0.5))
```

When the fractional part rounds up to 100 pence there is no carry into the
whole part — `%02d` happily prints three digits:

```
fmtMoney(1.999)       = "£1.100"        (should be £2.00)
fmtMoney(0.995)       = "£0.100"        (should be £1.00)
fmtMoney(999.999)     = "£999.100"      (should be £1,000.00)
fmtMoney(1234567.997) = "£1,234,567.100"
```

Because balances are float64 and compound daily (§1.3), values ending ≈ .995+
occur routinely; every money cell in the UI goes through this function.

**Suggested fix:** round to integer pence *first*, then split:
`p := int64(math.Round(v*100)); whole, frac := p/100, p%100`. (Or format from
the ledger's integer pence and delete the float path entirely — see §1.3.)

### 1.3 Two sources of truth for balances (Medium, Confirmed by reading)

In-memory state is float64 (`CustomerAccount.Balance/Rate/Interest`
customers.go:72-74, `Payment.Amount` payments.go:94) and compounds interest as
`a.Balance += a.Balance * a.Rate / 365.0` (demo_state.go:307-309) — no
truncation. The go-luca ledger holds exact integer pence and truncates (§1.1).
The two are written independently (`poundsToPence` at the boundary,
payments.go:14) and **will** diverge over a long simulation; only the ledger is
authoritative, but the UI renders the floats.

`.goluca` import makes this visible: `goImport`/`handleImport` write into
`sim.Ledger` only (main_wasm.go:49, export.go:51). `ds.customers`, balances and
the tx log are untouched, so after an import the dashboard still shows the
pre-import state. Export→Reset→Import does not round-trip the visible app.

**Suggested fix:** treat the ledger as the single source of truth: derive
displayed balances from `Balance()`/`DailyBalances()` (or keep integer-pence
mirrors), and rebuild in-memory state from the ledger after import.

### 1.4 Interest rates are float64 end-to-end

`Account.GrossInterestRate float64` (go-luca luca.go), rate-change events carry
`OldRate/NewRate float64` (gobank-products event.go:88-89), and the rate enters
decimal math via `decimal.NewFromFloat` (interest paths). go-luca's README
claims "no floating-point in core accounting" — amounts honour that
(`Amount int64`, good), rates do not. Low practical impact, but the README
overclaims and basis-point-precise rates would be safer as scaled integers.

---

## 2. Concurrency & data integrity

### 2.1 `file::memory:` DSN vs connection pool (High latent, Verified)

`cmd/demo/db.go:33` opens `pglike` with `file::memory:?_pragma=temp_store(2)`
and never calls `SetMaxOpenConns(1)`. go-postgres special-cases only the exact
string `":memory:"` to share one database across pooled connections
(driver.go:36); the `file::memory:` form does not match, so each pooled
connection gets **its own private empty database**. Verified:

```
conn1: CREATE TABLE pool_probe; INSERT → ok
conn2 (same *sql.DB): SELECT COUNT(*) FROM pool_probe
       → sqlite3: SQL logic error: no such table: pool_probe
control ":memory:": second connection sees the table — special case works
```

The demo works today because `database/sql` reuses the single idle connection
while requests are sequential — but any concurrent access (payment goroutines
payments.go:184,266 + HTTP handlers + `AddCustomersBatch`) can open a second
connection and find *no tables at all*. `cmd/demo/db_test.go:58`
(`TestLedgerSurvivesConnectionPool`) exists precisely because this class of bug
bit before, but it tests the customer store path, not this DSN.

**Suggested fix:** use `":memory:"`, or `db.SetMaxOpenConns(1)`, or make
go-postgres detect all in-memory DSN forms (`:memory:`, `file::memory:`,
`mode=memory`).

### 2.2 `nextval()` is non-atomic and splices names into SQL (Medium, Suspected)

go-postgres v0.5.2 `driver.go:298-302` implements `nextval` as an `UPDATE`
followed by a separate `SELECT`, with the sequence name formatted into the SQL
text as `'%s'` (also translate_sequence.go:110,149 for CREATE/DROP). Two
independent issues:

- **Atomicity:** UPDATE-then-SELECT can interleave between goroutines. A
  stress test (8×50 concurrent `nextval` on a shared file DB, pinned v0.5.2)
  produced 400 distinct values and no duplicates — the driver's execution
  appears serialized today — so this is *suspected*, not reproduced. The
  pattern is still fragile; `UPDATE … RETURNING` closes it for free.
- **Injection surface:** sequence names are attacker-influencable in any
  application that derives identifier names from input; `'%s'` with no quote
  escaping is an injection primitive. Parameter binding (or at minimum
  identifier validation + quote doubling) is warranted even in a demo-grade
  driver.

### 2.3 `knowledge_time` not persisted on the single-movement path (Medium)

go-luca `RecordMovement` (db.go:266-269) omits `knowledge_time` from the
INSERT, so the stored value comes from the schema default (`NOW()` — the
*database's* clock), while the returned `Movement.KnowledgeTime` is
`time.Now()` in Go. Two clocks, one bitemporal field; the value callers see is
never the value stored. `RecordLinkedMovements` (db.go:320) writes it
explicitly — the two paths disagree. For a ledger that advertises bitemporal
querying, `RecordMovement` should bind the same timestamp it returns.

### 2.4 DB handle escapes the lock across `Reset()` (Medium)

`DemoState.DB()` returns `ds.db` under `ds.mu`, but callers then use the
handle unlocked (`getGiltYields`/`getGiltHoldings`/`BuyGilt`,
treasury.go:88-142) while `Reset()` (demo_state.go:606) closes and re-opens
`ds.db` under the lock. A request racing a reset queries a closed handle;
because those helpers return `nil` silently on error (treasury.go:88,109), the
symptom is an empty treasury page and a lost write, not an error.

### 2.5 lofigui global buffer under wasm

lofigui's `Context` has an internal mutex per call (lofigui.go:33,87,245), but
a whole render (`Reset → writes → Buffer`) is not atomic. The native server
serializes renders with `renderMu` (main.go:22-32); the wasm build calls
`lofigui.Reset()/HTML()/Buffer()` with no lock (e.g. main_wasm.go:62-66). Safe
today only because GOOS=js is single-threaded — but background goroutines
(`Start`, `StartPayments`, `AddCustomersBatch`) do interleave at yield points,
and any future move to wasm threads breaks the assumption. Cheap fix: use the
same `renderAndCapture` wrapper in both builds.

Related upstream nit: package-level `lofigui.Print/HTML/...` always write to
the package-global `defaultContext` even when a `Controller` is configured
with a custom `Context` (controller.go:75,124) — two apps in one process
interleave output. Documented only in guides, not in godoc.

---

## 3. Security & privacy (demo-grade, but worth stating)

The demo is explicitly a model bank, so these are documented-risk items rather
than vulnerabilities-in-production — but the docs should say so explicitly.

- **Everyone is admin.** Unknown sessions get `RoleAdmin` (auth.go:85,132);
  `/role` lets any client switch roles (main.go:219); `/auth/authorize` grants
  PII access with no verification (main.go:573).
- **Sessions are guessable:** `fmt.Sprintf("sess-%d", time.Now().UnixNano())`
  (main.go:51), cookie without `Secure`/`HttpOnly`/`SameSite` (main.go:52-56).
- **Hardcoded PII key** in both server and wasm builds (db.go:15-17,
  `FixedKeyProvider`). The comment says the server "could" use
  `EnvKeyProvider`; it doesn't. In the wasm build any key ships to the client
  anyway — worth a note in the docs that browser-side PII encryption is
  cosmetic.
- **No RBAC under wasm at all:** every `goX` export is unguarded
  (main_wasm.go:436-513); PII gating is a single `piiAuthorized` bool with no
  roles or TTL (main_wasm.go:358-377), unlike the server's role matrix +
  expiry.
- **gobanks-customers, NI hash:** `hashNI` is unsalted SHA-256 into an indexed
  plaintext column (crypto.go:57-60, schema.go:26,30). NI numbers have a tiny
  keyspace — the column is brute-forceable offline and links equal NIs across
  rows, undermining the (otherwise sound) AES-GCM design. Use HMAC-SHA-256
  with a key from the KeyProvider.
- **gobanks-customers, key validation:** no provider validates key length
  (key_provider.go); a bad key surfaces only as a generic `aes.NewCipher`
  error at first use. The README's own example key is **31 bytes** and fails
  at runtime (README.md:57). Also `RotateKeys` re-encrypts row-by-row outside
  a transaction (sql_store.go:359-363) — a mid-run failure leaves a mixed-key
  table (recoverable via `key_version`, but not atomic).
- Positives worth recording: AES-GCM with fresh 12-byte `crypto/rand` nonce
  per encryption, authenticated decrypt errors handled, all queries
  parameterized, no PII in error strings (verified by reading gobanks-customers
  crypto.go/sql_store.go).

---

## 4. Native vs wasm divergence

| Feature | Native (main.go) | WASM (main_wasm.go) |
|---|---|---|
| RBAC | 4 roles + permission matrix + PII TTL | none; single `piiAuthorized` bool |
| Gilt purchase | `/treasury/gilts/buy` works | Buy form renders but no `goBuyGilt` export and app.js never intercepts the POST — **dead button** |
| Charts report | route exists, **not in navbar** (layout.go:53-57) | in navbar (index.html:92-96) |
| Phone preview | wired (customers_http.go:8) | nil — silently absent |
| Import | ledger only (see §1.3) | ledger only (same) |
| Memory limit | none | `debug.SetMemoryLimit(900MB)` (main_wasm.go:16) |

None of these are individually serious; collectively they mean the wasm demo
and the native demo are different products, and nothing documents which is
canonical. A small feature matrix in the docs (or a shared route/action table
consumed by both builds) would stop the drift.

Also verified: **a fresh checkout of cmd/demo does not build** —
`//go:embed diagrams/container.svg` (models.go:8-12) targets files that are
gitignored (`cmd/demo/diagrams/*.svg`) and only exist after `task diagrams`.
`go build`/`go vet`/gopls all fail until then. Consider committing the
rendered SVGs (they're small and change rarely), or embedding the `.d2` source
and rendering at build time into a non-ignored path.

---

## 5. Documentation review

### 5.1 Direct contradictions

- **Kubernetes:** ROADMAP.md Phase 2 is "Kubernetes + AlloyDB", Phase 3
  "CockroachDB + Scale". `cmd/demo/project_data.json` non-goals:
  *"Kubernetes / CloudSQL — No planned deployment"* and *"no RBAC↔GCP IAM"*.
  These cannot both be true; the About page and the roadmap disagree about the
  project's future. **Decide which is current and update the other.**
- **Done vs not done:** ROADMAP.md "Done" lists RBAC infrastructure and
  go-luca integration; project_data.json marks RBAC and Accounting
  `done:false`. Meanwhile ROADMAP.md "To Do" still lists "Working savings
  accounts" and "Customer model", both long implemented (savings are the
  most-exercised feature in the codebase).
- **Deployment:** project_data.json says "WASM (GitHub Pages)" `done:true`;
  actual deployment is statichost (task-plus.yml `site: h3-gobank`,
  README, ROADMAP). docs/research/project-hierarchy.html still carries
  `drummonds.github.io` GitHub-Pages links alongside the statichost table.
- **Architecture diagrams:** `cmd/demo/diagrams/container.d2` and
  `system_context.d2` show a BFF gateway, reconciliation, a mock-fps Faster
  Payments integration and persistent storage. None exist in code, and the
  running app renders these SVGs on its Models page — the app presents an
  architecture it does not implement. Either label them "target architecture"
  or trim to as-built.
- **project_data.json RBAC blurb** says "Admin role currently" / future
  "multiple user roles" — auth.go already has four roles with a permission
  matrix.

### 5.2 Broken / stale links & references

- README.md:12 → `codeberg.org/hum3/gobank-docs` — **404**, repo doesn't
  exist (checked Codeberg HTTPS + SSH and the NAS Forgejo). *(fixed in this
  review — link removed)*
- gendocs footer + 6 files in docs/research → `h3-lofigui.statichost.page/research.html`
  — **404 live**; the page is `research-charts.html` (the "Chart Renderer
  Comparison" content; lofigui's index is `RESEARCH.html`, capitalised).
  *(fixed in this review)*
- project_data.json `repo`/`goluca_repo` point at the GitHub mirrors, not the
  canonical Codeberg repos the README declares.
- cmd/demo/db_test.go:60 referenced "the go-postgres v0.4.1 fix" (pins are
  v0.5.2). *(comment updated in this review)*
- cmd/boefetch/main.go:17 claimed fallback data through 2025-12-18; last row
  is 07/Aug/2025. *(comment updated in this review)*

### 5.3 CHANGELOG.md malformed

Every second entry header was glued to the previous line
(`- Updating BOE display and version## [0.3.4] - 2026-03-01`), 0.3.34 lost its
description entirely to a double-glue, and the file ran 0.3.2→0.3.38 ascending
then 0.3.37→0.1.0 descending. Likely a release script inserting at a fixed
offset without a trailing newline. *(reformatted, newest-first, in this
review — worth also checking the release tooling so it doesn't regress)*

### 5.4 Component repo docs

- **gotreesitter README quick-start points at a different repo**:
  `go get github.com/odvcencio/gotreesitter` and matching imports
  (README.md:6,27-28) — the module is `codeberg.org/hum3/gotreesitter`
  (go.mod; the README's own Links row has it right). Anyone following the
  README installs an unrelated fork. Also "206 grammars" vs 210 actually
  registered.
- **gobank-db README overclaims**: "Shared Schema", "Connection Management —
  pooled connections", "Migration Support — schema versioning". The package
  is two functions (`Open`, `Migrate`) in ~40 lines; no schema, no pool
  config, no versioning. Its `Migrate` splits on `;`, which breaks on
  functions/triggers/string literals containing semicolons. Docs-site link
  uses `statichost.eu` while every sibling uses `.page`.
- **go-luca README** "no floating-point in core accounting" vs float64
  interest rates (§1.4).
- **gobanks-customers**: the plural name (`gobanks-customers` vs every other
  `gobank-*`) is never explained anywhere — if intentional, one README line
  would stop people (and tooling) "correcting" it; README omits
  `EnvKeyProvider` (exists in code + CHANGELOG) and its example key is
  invalid (§3).
- **lofigui**: README badge "go-1.21+" vs go.mod `go 1.22` (and the ServeMux
  patterns require 1.22); "No JavaScript" claim vs the `RunWASM` JS-export
  path.

### 5.5 Version drift matrix

| Repo | gobank pins | Latest tag | Notes |
|---|---|---|---|
| gobank-products | v0.1.5 | v0.1.6 | v0.1.6 *still* pins go-luca v0.2.25 — pre-refactor, despite its own changelog describing the refactor |
| go-luca | v0.2.25 | v0.2.30 | v0.2.26+ moved interest to gobank-products and split out gobank-db |
| go-postgres | v0.5.2 | v0.5.4 | go-luca v0.2.30 wants v0.5.3; gobanks-customers wants v0.5.2; gobank-db wants v0.5.3 |
| gotreesitter | v0.6.6 | v0.6.7 | |
| gobanks-customers | v0.1.0 | v0.1.0 | current |
| gobank-db | — (not in graph) | v0.1.1 | enters the graph the moment go-luca is upgraded past v0.2.25 |
| lofigui | v0.17.22 | v0.17.39 | 17 patch releases behind |
| ncruces sqlite-wasm | v1 (via go-postgres v0.5.2) | — | go-luca v0.2.30 uses `/v2`, latest go-postgres line uses v1 pseudo-version, tip resolves `/v3` — three major versions of the same dep across the family |

The family currently ships a **coherent but frozen** dependency set. The trap:
`go get -u` anywhere pulls go-luca ≥0.2.26 and changes where interest is
computed (and adds gobank-db) — an upgrade that looks routine but moves
financial logic between repos. Recommend upgrading deliberately, in one
change, with the interest fix (§1.1) landed first, and adding a CI check that
sibling pins match a published compatibility row.

Small internal inconsistencies: benchmark.md said Go 1.25.3 while the modules
are on 1.26.0 (*clarifying note added in this review* — the results are a
historical record, so the version was kept and flagged rather than rewritten);
cmd/chartcompare/go.mod still declares `go 1.25.3`.

---

## 6. Repo hygiene

- **Committed binaries:** `cmd/demo/demo.test` (44 MB) and
  `cmd/chartcompare/chartcompare` (5.6 MB) are tracked. `.gitignore` covered
  `demo` and `main.wasm` but not `demo.test` *(pattern added in this review;
  the already-tracked files need `git rm --cached` — left for you)*.
- pgx (`jackc/pgx/v5/stdlib`) is imported unconditionally (db.go:9) and so is
  compiled into `main.wasm`, where a `postgres://` DSN is unreachable — dead
  weight in the shipped binary. A `//go:build !(js && wasm)` driver-registration
  file would trim it.
- Root module is an empty shell: `gobank.go` only blank-imports
  gobank-products. Harmless, but `import codeberg.org/hum3/gobank` gives a
  consumer nothing — consider either re-exporting the intended public surface
  or documenting that the repo is cmd/ + docs only.
- boefetch quietly ignores the parse error on its own fallback data
  (`entries, _ := parseAPIResponse(fallbackData)`, main.go:157).
- **`task test:wasm` (and therefore `task check`) fails on Node ≥ 22**
  (verified on Node 22.17): `cmd/demo/wasm_test.js:12` assigns
  `globalThis.crypto = webcrypto`, but modern Node exposes `crypto` as a
  getter-only global. The polyfill is only needed on old Node — guard it:
  `if (!globalThis.crypto) { … }` (`wasm_bench.js:11` has the same line).
  `docs:build` itself succeeds; only the Node harness breaks.

---

## 7. What's solid (credit where due)

- go-luca's movement model is balanced **by construction** — one row, one
  amount, debit and credit always equal; `validateSameExponent` blocks
  cross-exponent movements. Simple and correct.
- Amounts are integer smallest-unit everywhere in the ledger; no float money
  in go-luca core.
- gobanks-customers crypto shape is right: AES-GCM, fresh random nonces,
  authenticated decrypts, parameterized SQL, no PII in errors.
- The wasm test/bench harnesses (`wasm_test.js`, `wasm_bench.js`, day-scaling
  ratio gate) are a genuinely good idea and catch real regressions.
- Docs infrastructure (Taskfile → d2/markdown → statichost, RC site) is
  coherent; the issues above are content drift, not tooling.

---

*Review conducted 2026-07-10/11 (Claude Code). Behavioural findings verified
against the pinned dependency set with standalone tests; component repos
reviewed at latest tags via fresh clones.*
