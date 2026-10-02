# Changelog

## [Unreleased]

## [0.3.54] - 2026-10-02

 - Core contracts package; the app logs in to the running demo

### Added
- ADR-0002 stage 1, story 1: the core's customer-side contracts
  (`core.CustomerQueries`, `core.Authenticator`) move from `bff` into a
  new root-module package `core`, with a contract test suite
  (`core/coretest`) that every implementation runs. The demo implements
  them through an adapter, and mounts the customer BFF on its own port
  under `/v1/`, so the Flutter app can log in to the running demo and see
  real customers: `GOBANK_APP_PASSWORD` is the one password every customer
  logs in with (unset = app login off). Transactions still come from the
  demo's in-memory log until stage 2.
- The Flutter app's `android/` and `ios/` platform folders are generated
  and committed; the Android debug manifest allows cleartext traffic so a
  debug build can reach the demo over plain HTTP (release builds cannot).

### Fixed
- `bff/stubbank` returned transactions grouped by account, oldest first,
  although `TransactionPage` promises newest first; caught by the new
  contract suite.

## [0.3.53] - 2026-10-01

 - DB explorer: FK links fixed; browse per component

### Added
- The DB explorer is divided into the bank's components (go-dbexplorer
  v0.3.0, catalog built from `components.go`): `/internal/explorer/c/payments`
  shows only the tables payments owns and the contract views it publishes,
  and every table in the full view is tagged with its owner, linked to that
  scope. Foreign keys into another component open that component's scope.
- The DB explorer applies the viewer's role (go-dbexplorer v0.4.0): the
  customers component, which holds PII, needs `view_pii`, so the read-only
  role no longer sees `cust_pii` or the other customer tables; foreign keys
  into it show the value and owner without a link.

### Fixed
- DB explorer foreign-key links now open the referenced rows instead of the
  whole table. The server route and the WASM bridge parsed the explorer's
  query string themselves and dropped `filter`/`value`; both now hand the
  link's URL to go-dbexplorer's `Render`.

## [0.3.52] - 2026-10-01

### Added
- `tp release` ends by asking the gobank-deploy appliance to fetch the new
  release's binaries into its store (`post_release: task cloud:fetch`), so
  the environments page offers it at once. Needs task-plus with
  `post_release` support and the LAN; a failure is only a warning.

## [0.3.51] - 2026-09-30

 - Release binaries fixed (force gitea token); customer chart on gogal v0.2.0

### Changed
- The customer chart renders through gogal v0.2.0; go-analyze/charts and
  the local fork's `replace` are gone from `cmd/demo`.

### Fixed
- Release binaries: v0.3.50's goreleaser run failed because `tp release`
  exports both `GITHUB_TOKEN` and `GITEA_TOKEN`; `.goreleaser.yaml` now sets
  `force_token: gitea`. v0.3.50 has no binaries attached — use v0.3.51.

## [0.3.50] - 2026-09-30

 - Releases carry the demo binaries: the build stage gobank-deploy fetches from

### Added
- `.goreleaser.yaml`: `tp release` builds `cmd/demo` for linux amd64 and
  arm64 (CGO off, version from the tag) and attaches `demo-linux-amd64`,
  `demo-linux-arm64` and `checksums.txt` to the Forgejo release. This is
  the build stage gobank-deploy's store fetches, so a new version reaches
  an environment without touching gobank-deploy or the appliance. Built on
  the laptop because `cmd/demo/go.mod` replaces the charts fork locally.

## [0.3.49] - 2026-09-29

 - Version update

### Changed
- The DB explorer uses the PostgreSQL catalog queries on both backends
  (go-dbexplorer v0.2.0, go-postgres v0.6.0), so the demo no longer tells it
  which backend it's on. On pglike, the schema section now shows PG type
  names, the `<table>_pkey` index and view columns.
- Daily accrual postings are written by one worker per CPU on PostgreSQL
  (one on pglike/WASM). Locally 8 workers post ~4.6x the movements/s of one.
- Customer creation holds the state lock only to decide the customer (record,
  funding, book); its database transaction runs unlocked. Adding customers
  uses one worker per CPU on PostgreSQL, and the day loop is no longer
  starved by a running batch add. A customer the database refuses is taken
  back off the books.

### Fixed
- Lock-order deadlock between the day loop and customer creation on the
  pglike store: the day loop wrote to the database under the engine lock
  (lazy accrual-account creation, BoE interest) while a customer
  transaction held the write lock and waited for the engine lock, stalling
  both for SQLite's 10s busy timeout (a permanent hang on the WASM shared
  connection). Accrual accounts are now created with the ledger, and BoE
  interest is written after the locks are released.
- Data races between customer creation and the engine sweep: the tx-bound
  simulation copy and the simulation clock are now read and set under the
  engine lock.

## [0.3.48] - 2026-09-28

 - Runtime page shows the day in progress: phase, movements done / expected, rate and elapsed

## [0.3.47] - 2026-09-27

### Fixed
- The DB explorer listed no tables on PostgreSQL: the backend flag was
  dropped when the explorer moved to go-dbexplorer, so it queried the
  SQLite catalog. Covered by a test that runs against `GOBANK_PG_DSN` when
  set.

## [0.3.46] - 2026-09-27

### Changed
- The contract-view baseline is empty: customers publish
  `contract_customer_accounts` and the ledger `contract_ledger_movements`,
  which the book and the applied-interest figures now read; gilt and
  accrual table creation and the accrual persistence code moved into the
  files of the components that own those tables (`treasury.go`,
  `accrual.go`); reset clears the register and accrual state through their
  components' APIs.

## [0.3.45] - 2026-09-27

### Removed
- `deploy/hetzner/` scripts and the `cloud:up` / `cloud:down` / `cloud:status`
  / `cloud:ssh` tasks. Hetzner deployment now lives in the separate
  [gobank-deploy](https://git.bytestone.uk/hum3/gobank-deploy) repository
  (`up`, `down`, `status` per named environment, plus the environments
  console); `task cloud:ui` still opens that console from here.

### Added
- Contract views (ADR-0001): every table belongs to one component, declared
  in a registry (`cmd/demo/components.go`); other code reads a component's
  data only through its `contract_*` views or its API. A source-scanning
  test enforces the rule, with the pre-rule cross-reads held in a baseline
  that can only shrink. The About menu gains a Documentation page generated
  from the registry: components, tables, views, the rule, the debt list and
  the architecture decision records, which live as markdown in `adr/` and
  are compiled into the build. The DB explorer badges each table with its
  owning component and lists contract views.
- `GOBANK_MEMORY_LIMIT` sets the demo's auto-stop threshold (e.g. `6GB`), replacing the fixed 800MB; the GC advisory limit follows it with 12.5% headroom on both server and WASM builds. Unset keeps 800MB, the browser-tab size. The runtime page shows the configured value.
- Dashboard throughput readouts: interest movements per 12h (the engine's measured accrual rate against an overnight batch window, averaged over the last 10 simulated days) and customers added per second during and after a batch add.
 - Phase 2 groundwork for the mobile apps, all inside this repository:
   `screen/` (versioned server-driven screen tree with JSON and HTML
   renderers), `bff/` and `cmd/bff` (hardened backend-for-frontend: session
   tokens, credential login with rate limiting and lockout, audit log, and
   screen endpoints that answer nothing without a session; runs on the
   in-memory `bff/stubbank` until the banking core is extracted from
   `cmd/demo`), and `app/` (Flutter thin shell that renders BFF screens).
   ROADMAP Phase 2 reordered so authentication comes first.

### Changed
- Payments are read from the database, not from a list held in memory: a
  `payments` table owned by the payments component, written in the same
  transaction as the customer a funding payment belongs to, with status
  transitions recorded on the row. Other code reads them through the
  `contract_payments` view or the payments API (`paymentByID`,
  `paymentPage`, `paymentsOf`, `paymentCount`), the first component built
  under ADR-0001.
- Customers are read from the database, not from a list held in memory.
  Identity and KYC come from gobanks-customers, the accounts a customer
  holds from a new `customer_accounts` register (product, sort code,
  account number, ledger account), balances and accrual from the products
  engine, applied interest from the ledger's application movements. Bank-wide
  savings/lending totals, per-product totals and the interest P&L are derived
  from the ledger (`book.go`) instead of summed over every customer on each
  page render. The customer-facing transaction log keys entries by ledger
  account rather than account index. On PostgreSQL this takes the customer
  mirror (~590 bytes and 6 heap objects per customer, ~175 MiB at 300k
  customers) off the Go heap; the products engine's in-memory account
  registry (~1.5 KiB per customer) and the payments list remain and are the
  next targets.
- The bank app's customer list endpoint returns the first page of customers
  (50) rather than every customer.

## [0.3.44] - 2026-08-27

 - Bulk database writes now stream in small transactions targeting ~10ms
   each (adaptive chunk sizing), designed for an ultimate load of one real
   day per simulated day: writes trickle continuously instead of arriving
   as monolithic bursts that hold locks and demand oversized hardware.
   Daily accrual postings are collected in memory under the sim lock and
   written to the ledger lock-free (collectAccrualMovements +
   writeMovementsChunked); accrual_state persistence commits in the same
   ~10ms chunks.
 - This removes the 26–37s dashboard stalls seen on PostgreSQL while the
   simulation ran (a lock convoy: dashboard → state lock → customer adder
   → sim lock → day sweep doing one fsync'd transaction per account).
   Measured with the sim running and a 30,000-customer add concurrently:
   dashboard polls hold 51–68ms with no stalls, and customers add at ~190/s
   while simulated days tick ~10s/day at 30k customers.
 - Known remaining spike: month-end interest application movements are
   still written per account inside the products engine under the sim
   lock; streaming those needs a gobank-products change.

## [0.3.43] - 2026-08-27

 - Hetzner cloud deployment for performance testing: `deploy/hetzner/` plus
   Taskfile targets `cloud:up` / `cloud:down` / `cloud:status` / `cloud:ssh`
   (run via `tp secrets`). `cloud:up SCALE=small|medium|large|xl` (or any
   hcloud server type) provisions a firewalled server via cloud-init with
   PostgreSQL sized to the machine's RAM, cross-compiles the demo (arm64
   for cax types), and deploys it as a systemd service on port 1347.
   Deleting the server (`cloud:down`) is what stops billing.
 - The demo server honours `GOBANK_PG_DSN` (real PostgreSQL backend via
   pgx) and `GOBANK_ADDR` (fixed listen address). A configured DSN is
   pinged at startup so an unreachable database fails fast, and in
   Postgres mode startup drops all public tables — in-memory simulation
   state is authoritative and stale rows from a previous run would
   collide; export .goluca to keep a run's ledger.
 - The About → Runtime page reports the actual data store (previously
   hardcoded "In-memory (pglike/SQLite)") with the DSN password redacted.
 - The DB explorer now works on real PostgreSQL: catalog access goes
   through backend-aware helpers (sqlite_master/PRAGMA on pglike,
   pg_tables/information_schema/pg_indexes on PostgreSQL).
 - Customer creation is transactional: each customer's writes (customer +
   PII rows, ledger accounts, funding movements) share one transaction via
   go-luca v0.2.32 `SQLLedger.WithTx` and gobanks-customers v0.2.1
   `SQLCustomerStore.WithTx` — 333 customers/s vs 178 on a cx33 against
   PostgreSQL. A customer whose persist fails is rolled back and skipped
   instead of surviving in memory only.
 - The 1ms per-customer yield in batch adds now only runs on WASM builds
   (`runtime.GOOS == "js"`); server builds add customers at full speed.

## [0.3.42] - 2026-08-26

 - The database now carries complete interest state: every simulated day
   the per-account accrual numerators (and the BoE accumulator) persist
   to a new accrual_state table, written off the state locks. An
   accrued_pounds_e7 column models each accrual as 7dp pounds (integer
   fixed point, rounded from the exact numerator; the pence conversion
   truncates, matching engine application). refreshFromLedger rehydrates
   engine numerators, account mirrors and BoE state after an import.
 - Daily accruals are visible in the ledger: newly accrued whole pence
   post daily (code LDAS:FTDP:ACRU) from Expense:Interest /
   Income:Interest into AccruedInterest holding accounts, reversed on
   month-end application so net P&L stays exactly the engine's.
 - BoE reserve interest is modelled in accounts: Income:Interest:BoE
   accrues daily into Asset:AccruedInterest:BoE and is received into
   Asset:BoEReserves at month end; reports show applied plus accrued.
 - Customer report shows accrued-but-unapplied interest at 7dp pounds
   alongside applied interest.
 - DB explorer: per-table option to truncate ID/text cells to 10 chars
   with the full value as hover tooltip (?trunc=1), persisted across
   sort and pagination links.
 - Bank app front door no longer lists every customer: /app/ is a mock
   login screen (customer ID plus any password) and unknown IDs are
   rejected; customer names no longer appear pre-login.

## [0.3.41] - 2026-08-26

 - Interest now runs on the gobank-products v0.1.10 engine: exact integer
   daily accrual in memory (numerator over 10,000 × 365, remainder carried
   — sub-penny interest is never lost) with one application movement per
   account at month end. Balances step up monthly; reports include
   accrued-but-unapplied interest. The demo's own float64 interest loop
   and pending-movement flusher are gone.
 - Blanket prohibition on float64 for money storage: all money is integer
   minor units (luca.Amount) across accounts, payments, tx log, histories,
   dashboards, treasury/gilts, P&L and the bank-app JSON (now *_minor
   fields). fmtMoney takes pence, fixing its rounding-carry bug.
   TestNoFloatMoneyStorage guards against regressions.
 - Performance: a simulated day at 10k accounts drops from ~3.1s to ~5ms
   (end-of-day sweep is query-free); WASM full-year integration run drops
   19.3s → 2.0s. Month-end application sweeps are paced on WASM so the
   page stays responsive; daily interest ledger writes no longer hold the
   state lock (worst-case dashboard lock wait ~2.7s → under 2ms).
 - Customer Count chart: integer y-axis labels; x-axis with pinned
   YYYY-mm-dd end labels, calendar-boundary mid labels and day/week/month
   minor tick marks (via a local go-analyze/charts fork adding
   XAxisOption.CustomTicks); collision handling so date labels never
   overlap, regression-scanned across 1500 spans.

## [0.3.40] - 2026-08-25

 - Fix daily interest postings never reaching the ledger database: demo
   only looked up Expense:Interest / Income:Interest (relying on a
   gobank-products EnsureInterestAccounts that does not exist), so the
   cached account IDs stayed empty and every posting was silently skipped.
   initLedger now creates the accounts when missing.
 - Flush pending interest movements to SQL at each end-of-day finalize
   (one batch per day) instead of only before export, so they show as
   normal transactions in the DB explorer.
 - Skip zero-pence interest movements; add regression tests.
 - Bump gobank-products pin v0.1.8 → v0.1.9 (v0.1.8 tag pins an
   unresolvable go-luca).
 - Fix wasm crash when adding 100 customers: the demo DSN
   `file::memory:` was only pool-safe for a single connection — go-postgres
   special-cased just `:memory:`, so a second pool connection (opened when
   dashboard polling overlaps a batch add) saw its own empty database.
   Fixed in go-postgres v0.5.7 (all in-memory DSN spellings now share one
   database); demo pin bumped v0.5.5 → v0.5.7, regression test added.
 - Fix native (server-mode) startup: layout templates still used pongo2
   syntax after lofigui's switch to html/template; results now passed as
   template.HTML.
 - Fix "database is locked" errors under concurrent load (seen natively
   once pool connections genuinely shared one database): go-postgres
   v0.5.8 opens the shared temp file with immediate transactions, WAL and
   a busy timeout, so read-then-write transactions no longer hit SQLite's
   handler-bypassing SQLITE_BUSY deadlock-avoidance path. Pin bumped
   v0.5.7 → v0.5.8.
 - Fix the browser (WASM) crash on adding customers: go-postgres's
   single-shared-connection fallback only locked per driver call, so
   concurrent pool "connections" interleaved statements on the one SQLite
   connection and corrupted it (panic inside SQLite). go-postgres v0.5.9
   holds the lock across whole transactions and open result sets; pin
   bumped v0.5.8 → v0.5.9.
 - Drop the deprecated ncruces/go-sqlite3/embed blank import that printed
   "you're unnecessarily importing ... embed" at startup.
 - Fix a WASM deadlock ("all goroutines are asleep") when the DB explorer
   queried while another query's rows were open: go-postgres v0.5.10 now
   materialises query results instead of holding the shared-connection
   lock until rows close. Pin bumped v0.5.9 → v0.5.10.
 - Fix `task test:wasm` on modern Node: wasm_test.js/wasm_bench.js no
   longer assign the getter-only globalThis.crypto.
 - Fix the browser page freezing and eventually being killed by Chrome
   (no console error) once ~100 customers were added: the simulation used
   a fixed-rate 200ms ticker, and in WASM once advanceDay takes longer
   than the interval the next tick is always due, the Go scheduler never
   goes idle, and control never returns to the JS event loop — UI frozen,
   days advancing flat-out, memory growing until the tab dies. The sim
   (and payments) loops now self-pace: wait 200ms after each day
   completes. Verified by driving the built wasm in headless Chrome via
   CDP: page stays responsive with 130+ customers and stable memory.

## [0.3.39] - 2026-03-26

 - Add RC deploy site and fix tp check warnings

## [0.3.38] - 2026-03-25

 - adding extra files

## [0.3.37] - 2026-03-20

 - Adding memory management

## [0.3.36] - 2026-03-19

 - Adding app functionality to wasm

## [0.3.35] - 2026-03-19

 - New DB and updating customer view

## [0.3.34] - 2026-03-18

 - (no changes recorded)

## [0.3.33] - 2026-03-18

 - adding benchmark

## [0.3.32] - 2026-03-18

 - fixing db

## [0.3.31] - 2026-03-18

 - Adding customer app preview

## [0.3.30] - 2026-03-17

 - Working on documenation and interest

## [0.3.29] - 2026-03-16

 - moving simulation and products to gobank-products

## [0.3.28] - 2026-03-16

 - tidying

## [0.3.27] - 2026-03-16

 - Adding roles and movement codes

## [0.3.26] - 2026-03-16

 - unifying DB

## [0.3.25] - 2026-03-16

 - fixing menu and db explorer  in WASM

## [0.3.24] - 2026-03-16

 - db explorer

## [0.3.23] - 2026-03-16

 - fix import export buttons

## [0.3.22] - 2026-03-16

 - tweak check

## [0.3.21] - 2026-03-14

 - Removing .task from vc

## [0.3.20] - 2026-03-14

 - Updating to goluca

## [0.3.19] - 2026-03-07

 - Adding software hierarchy to docs

## [0.3.18] - 2026-03-07

 - Adding version

## [0.3.17] - 2026-03-07

 - Concept of blue green dB releases and unfying metatdate documentation

## [0.3.16] - 2026-03-06

 - Adding project info page

## [0.3.15] - 2026-03-05

 - Improving treasury display

## [0.3.14] - 2026-03-04

 - Adding github pages

## [0.3.13] - 2026-03-04

 - Updating check to include docs build

## [0.3.12] - 2026-03-04

 - Updating gotreesitter and docs

## [0.3.11] - 2026-03-04

 - Release prep

## [0.3.10] - 2026-03-02

 - Starting to get form

## [0.3.9] - 2026-03-02

 - Adding HTMX to  be more dynamic

## [0.3.8] - 2026-03-02

 - Release prep

## [0.3.7] - 2026-03-02

 - Adding graphs getting better

## [0.3.6] - 2026-03-02

 - Adding benchmarks

## [0.3.5] - 2026-03-01

 - Changing money format

## [0.3.4] - 2026-03-01

 - Update WASM

## [0.3.3] - 2026-03-01

 - Changed build version number for demo

## [0.3.2] - 2026-03-01

 - Updating BOE display and version

## [0.3.1] - 2026-03-01

 - Adding about box

## v0.3.0 2026-02-28

- Encrypting at rest for customer data

## v0.2.1 2026-02-28

- A bit more complexity

## v0.1.4 2026-02-27

- Multi page

## v0.1.3 (unreleased)

- fleshing out mock payments

## v0.1.0 (unreleased)

- Initial scaffold: simulation engine, account behaviors, daily updates
- Clock abstraction (WallClock, SimClock) for testable time
- AccountBehavior interface with optional hooks (MovementHook, ParameterHook, PendingClosureHook)
- ParameterStore for time-varying per-account values
- SavingsAccountBehavior with daily interest accrual via go-luca
- DailyUpdate mechanism delivering account state changes after each processed day
- Benchmarks: 1,000 accounts x 3 days in ~4.8s on SQLite :memory:
