# Changelog

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
