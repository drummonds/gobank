# Changelog

## [Unreleased]

## [0.22.0] - 2026-10-08

 - Stage 6 story 1.6.3: the staff web is the package bff/staff, served by the BFF; the demo keeps the console

### Changed
- ADR-0002 stage 6 story 1.6.3, staff pages through the BFF. The staff
  web is the package `bff/staff`: the layout, the role switch and the
  PII authorisation, and the pages that read the bank through
  `core.StaffQueries` and act on it through `core.Commands` (accounting,
  products, customers, the payment detail, treasury including buying a
  gilt, reports, about, models and the documentation), with the
  dashboard's data sections and the money and chart formatting. The BFF
  serves it for every path outside `/v1/` (`bff.Config.Staff`), without
  the customer routes' security headers, so the BFF is the demo's
  handler. The console (the dashboard and its controls, the payments
  generator, settings, runtime, export and import, the explorer and
  `/about.json`) stays in `cmd/demo` and mounts its pages on the site
  (`Site.Handle`), rendering them in the layout through the site's
  services, until story 1.6.4 moves it in. A pure move: every route and
  page is as it was. `TestStaffWebKnowsOnlyTheCore` holds the package's
  imports to the core, the embedded ADRs and its UI libraries; the root
  module now requires lofigui, gogal and goldmark. The architecture
  diagrams, the favicon and the project data the pages embed live under
  `bff/staff/`.

## [0.21.0] - 2026-10-08

 - Stage 6 story 1.6.2: the customer web is the BFF's HTML; the demo's phone frame and open customer API are retired

### Changed
- ADR-0002 stage 6 story 1.6.2, customer web through the BFF. The demo's
  hand-rendered phone frame (`/app/`, `bankapp_*.go`, `LayoutBankApp`)
  and the open `/api/customers` and `/api/customer/` endpoints are gone:
  the Bank App link opens the BFF's own HTML at `/v1/screen/login`, so
  the browser and the app render from one screen tree and no customer
  data is served without a session. The BFF takes the scope it is mounted
  under (`bff.Config.Scope`, `/` on a server, the service worker's scope
  in the tab): its documents carry it as their `<base>`, every link and
  action is relative to it, and its redirects and session cookie carry
  it. In the tab the demo keeps the session cookie the service worker's
  response cannot set (`keepCookiesInTab`) and logs in with a fixed demo
  password that the login screen states (`bff.Config.LoginNote`). The
  API load benchmark reads the customer's screens through the BFF.

### Fixed
- The BFF's session store asks for its database on each use
  (`bff.Config.SessionDB` is now a function) rather than holding the
  handle it was built with. The demo closes and replaces its in-memory
  database on a reset or an import, after which every login failed with
  "database is closed"; the old run's sessions now go with it and the
  new run's customers can log in.

### Removed
- `bff.TxIcon`, which served the phone frame alone.

## [0.20.0] - 2026-10-08

 - Demo pages no longer queue behind one slow page; dashboard, P&L and balance sheet served from background readings

### Fixed
- The demo's pages no longer queue behind one another. Every staff page
  was rendered under one process-wide mutex that was held through the
  page's database reads, so a slow page (customers, with hundreds of
  per-account reads on a saturated database) stalled every other page
  for every visitor: on the Hetzner demo, pages waited minutes. The
  mutex only guarded lofigui's global buffer, which the pages never
  needed; each page now returns its HTML directly.
- The dashboard, P&L and balance sheet are served from readings taken in
  the background (`bank/reading.go`). The book totals and the interest to
  date aggregate every account through the ledger's views, which takes
  seconds on a large bank; pages now get the last reading at once, and a
  reading older than two seconds is taken again behind the page rather
  than in front of it. Only a reading never taken (the first page after
  startup) or one a command has invalidated (opening a customer, a
  payment, a day's close, a restart or import) makes a page wait, so
  what follows a command is still what the command did; a read in
  flight when that happens is discarded and taken again.

## [0.19.0] - 2026-10-07

 - Stage 6 story 1.6.1: the demo is one handler, served in the tab by a service worker

### Changed
- ADR-0002 stage 6 story 1.6.1, one handler in the tab. The demo's routes
  are one `http.Handler` built over its state (`newHandler`, cmd/demo/
  routes.go): the server listens on it and the WASM build serves the same
  handler from a service worker (lofigui's wasm-deploy bootstrap over
  go-wasm-http-server), replacing the forty `goRender*` exports, the page
  switch in `app.js` and the WASM-only `index.html`. The WASM demo now has
  the server's layout, HTMX polling, forms and links. Pages are
  scope-relative: every document carries the path it is served under as
  its `<base>`, links and form actions are relative to it and redirects
  carry it, so the demo runs unchanged at `/` on a server and at `/demo/`
  on the docs site; a form's redirect target is honoured only when it is
  such a path. The staff session in the tab is a fixed ID (a service
  worker's response cannot set a cookie). `test:wasm` and `bench:wasm`
  drive the binary over the same Request/Response protocol the worker
  uses (`wasm_harness.js`), under the `/demo/` scope.

## [0.18.0] - 2026-10-07

 - Stage 5 story 1.5.4: products, customers, payments and the bank itself are packages; the core is in packages and the demo is wiring, console and UI (ADR-0004 applied)

## [0.17.0] - 2026-10-06

 - Stage 5 stories 1.5.2 and 1.5.3: the daily snapshots and the books of account are the packages bank/history and bank/ledger

### Changed
- ADR-0002 stage 5 story 1.5.2, history. The daily snapshots are the
  package `bank/history`: it owns `daily_snapshots` and its migration
  (`history.Schema`), writes a day's snapshot once (`Save`), gives the
  latest day and the span of the record (`Latest`, `Span`) and the series
  the charts draw (`Series`, the newest `MaxPoints` days as
  `core.History`). The demo takes the snapshot (`recordHistory`) and the
  adapter's `History` reads the series from the package. No schema change.
- ADR-0002 stage 5 story 1.5.3, ledger. The books of account are the
  package `bank/ledger`: go-luca's ledger opened on the bank's database
  with the chart of accounts resolved (`Open`, `Chart`: equity, the
  interest P&L accounts, the BoE reserve's accounts; `Account` opens any
  other path on first use), the customer account paths under the savings
  and loans roots (`OpenCustomerAccount`), the striped account locks
  (`Lock`, shared by a ledger bound to a transaction with `WithTx`), the
  posting of an event with projections (`Post`) and the exact read of an
  account's live position from the contract view (`LivePosition`).
  go-luca still owns the tables and publishes the contract views. The
  demo keeps what is the products' business until 1.5.4: `accountDay`
  and `postEvent` run the day's rules on the accounts an event touched,
  over the package. The six resolved account IDs leave `DemoState` for
  the ledger's `Chart`. No schema change.
- The WASM integration test no longer runs a full year of one customer;
  the short run renders the customers and treasury capital pages too,
  so every page is still exercised. `task check` is ~25s quicker.
- Roadmap: stories 1.5.4 to 1.5.7 (products, customers, payments, the
  bank composite) are one story, released once.
- ADR-0002 stage 5 story 1.5.4, the rest of the core into packages; the
  stage's done-when. `bank/products` is the catalogue, the day's rules
  for one account (`AccountDay`), the posting of an event with the rules
  rerun on the accounts it touched (`PostEvent`) and the start-of-day
  pass (`RunPass`). `bank/customers` is the customer store, the account
  register and its contract view, the read model (balances and accrual
  from the ledger's live positions), opening a customer (`Plan`,
  `Persist`) and the statement lines. `bank/payments` is the payments
  table, its view and a payment's lifecycle. `bank/refs` is the one
  allocator the customers' and the payments' numbers come from
  (ADR-0004). `bank` is the bank: the composition root that implements
  `core.Commands` and `core.StaffQueries`, owns the business day and its
  start, the BoE reserve's accounting, the position, the profit and loss
  and the balance sheet, and reopens itself over a wiped database on a
  reset. The demo's `coreAdapter` is gone: `DemoState` embeds the bank
  and is wiring (the database, the simulation, the restart record, the
  console, the memory limit); the app's login is `appLogin` in the demo.
  The staff pages, the bank app and the runtime page read through the
  core. `internal/yield` moves to the root module. No schema change.
- ADR-0004 (identity, references and shared state across topologies) is
  applied: the dashboard's book and customer count are a read model over
  the ledger's and the customers' contract views, cached for two seconds
  and invalidated by every command, in place of running totals under a
  lock; one mutex guards the day-start transition and the BoE close; the
  ledger's account lock is a local courtesy. A figure on the dashboard
  can lag a command from elsewhere by up to two seconds.
### Added
- ADR-0004: identity, references and shared state across topologies.

## [0.16.0] - 2026-10-06

 - Stage 5 story 1.5.1: the treasury is the first bank component in a package of its own, bank/treasury

 - Stage 5 story 1.5.1: the treasury is the first bank component in a package of its own, `bank/treasury`

### Changed
- ADR-0002 stage 5 story 1.5.1, treasury. The gilt desk is the package
  `bank/treasury`: it owns `gilt_yields` and `gilt_holdings` and their
  migrations (`treasury.Schema`), quotes the yield curve, lists the
  holdings and books a purchase on the bank's business day, which is the
  only thing it reads of the rest of the bank. The demo's `coreAdapter`
  forwards `GiltYields`, `GiltHoldings` and `BuyGilt` to it; the treasury
  pages stay in the demo as UI (`treasury_pages.go`) reading through the
  core. A component may now be a package: the registry's `Package` field
  names it, the documentation page shows it, and the contract-view rule
  (ADR-0001) scans `bank/` as well as the demo. The bank packages are held
  to the core, each other and the domain libraries by
  `TestBankPackagesKnowOnlyTheCoreAndEachOther`: no UI, no simulation, no
  demo wiring. No schema change: the drill is the restart alone.
### Added
- `bank/schema`: the shape of a component's migrations (`Migration`,
  `Component`), so a package can publish its own schema for the wiring to
  apply. The demo's `componentSchema` and `migration` are aliases of it.

## [0.15.0] - 2026-10-06

 - Stage 4 story (c), the simulation as a package of its own (the stage is done), and the customer app as designed: generated wireframe, access states, BFF journey

 - Stage 4 story (c): the simulation is a package of its own, built on the core alone; the stage's done-when
 - The customer app as designed: a wireframe generated from the screen trees (served and proposed), the access states, one journey through the BFF, on a docs page

### Changed
- ADR-0002 stage 4 story (c), the split, and the stage's done-when. The
  simulation is `cmd/demo/sim`: the warped clock the bank reads, the
  replayed base rate, the generators for the population and its payments,
  the run loop and the console knobs. It imports nothing of the bank but
  `core` (and go-luca for the money type), so the compiler is the
  enforcement that no simulation code touches bank internals, and
  `TestSimulationImportsOnlyTheCore` checks the package's imports. The
  simulation drives the bank through `core.Commands` and reads it through
  `core.StaffQueries`; its own lock replaces its share of `DemoState.mu`;
  the run is recorded through a store the wiring implements over the
  simulation component's table. `DemoState` is now the demo application:
  the bank, wired to the simulation and the console, whose methods
  forward to the simulation (`console.go`). Reset is wiring: the
  simulation stops and its clock goes back to the opening day, then the
  bank's tables go. The memory limit is the process's: the simulation asks
  before each day whether to pause.
- The bank no longer writes the run row at the start of a day; its own
  record of having begun a day is the day's snapshot. The simulation
  records the run after the bank has begun the day.
### Added
- `app.md`, the customer app page of the docs, with three d2 views: the
  **wireframe**, one phone per screen with every action wired from the
  component that carries it to the screen or endpoint it opens, grouped
  by the customer's state (logged out, onboarding, logged in), generated
  from the screen trees by `screen.Journeys.Wireframe` and
  `go run ./cmd/bff -wireframe`; the **access states** (logged out,
  locked out, logged in, signed out, applying, checking) with a decision
  table of what Home shows in each; and **one journey through the BFF**
  as a sequence (log in, summary, pay, expiry, log out).
- `bff.Storyboard`: every screen the app has or is to have, as screen
  trees with sample data over the stub bank. Proposed, and drawn dashed:
  Home as a **summary** a logged-in customer lands on (net position,
  recent movements, accounts, pay), a four-tab nav (Home, Accounts,
  Activity, More) with log out on More, a signed-out screen that log out
  and an expired session land on, pay someone and payment sent, and the
  start of KYC onboarding (open an account, your details, identity check,
  checking). The storyboard is where a new screen is designed first; its
  builders become the served screens when their story is built.


## [0.14.0] - 2026-10-06

 - Stage 4 story (b): the bank reads the time and the base rate from injected sources; the simulation clock is warped by the day length and resumes mid-day

 - Stage 4 story (b): the bank reads the time and the base rate from injected sources; the simulation's clock is warped by the day length and resumes mid-day

### Changed
- ADR-0002 stage 4 story (b), the clock. The bank reads the time from
  `core.Clock` and the Bank of England base rate from
  `core.BaseRateSource`; every banking fact is stamped by the clock (a
  payment's creation and settlement, a customer's join date, a
  transaction's value date) and the day's rate is the source's for that
  day. Operational records (restarts, schema versions, the run row's
  write time) keep the wall clock, as does the ledger's knowledge time.
  `StartDay` now follows the clock: nothing happens while the clock is on
  the bank's day; a pass still owed is finished first; days the clock has
  moved on to are started one at a time until the bank is level. The
  bank's business day is the latest on its own record, the daily
  snapshots, not the simulation's run row.
- The simulation supplies a warped clock: a simulated day begins at the
  start of its slot and the day length sets how fast the day's hours pass
  (an hour into a two-hour day is noon); flat out, time passes at the
  wall's pace; the clock stays inside its day until the next begins. The
  slot's start is on the run row (simulation schema version 4), so a
  restart resumes the clock mid-day. The simulation steps its clock only
  when the bank is level with it and reports the day complete
  (`core.Position.DayComplete`), so a resumed pass finishes on its own
  day. Payments on the payments page now carry simulated times.
- The dashboard shows the wall clock and the sim clock side by side, with
  the warp between them (×12 at a two-hour day, flat out otherwise), so a
  watcher gets a feel for the rate simulated time passes at; `/about.json`
  `sim` gains `wall`, `clock` and `warp`.
- The base-rate series (`boe_rates.csv`) is simulation data
  (`sim_rates.go`), replayed through the source the bank reads; a real
  deployment wires a feed in its place.
- The contract suite's fixture may hand over a fake clock; with one,
  `StartDay` is checked to follow it.

## [0.13.0] - 2026-10-06

 - Stage 4 story (a): OpenCustomer and StartDay are core commands; the generators reach the bank through commands alone; accrual_state dropped

 - Stage 4 story (a): opening a customer and starting a day are core commands; the generators reach the bank through commands alone; accrual_state dropped

### Changed
- ADR-0002 stage 4 story (a), the entry points. `core.Commands` gains
  `OpenCustomer` (a customer with their KYC standing, PII and accounts,
  each with its opening money; the bank allocates the ID, dates the record
  on its business day, funds each account by a payment on record and keeps
  a loan within its lending headroom, down to nothing) and `StartDay` (the
  bank moves on a business day and runs the day's pass, resuming one cut
  short). The generators are simulation files of their own
  (`sim_customers.go`, `sim_payments.go`) that raise events on the bank
  through these commands and `Transfer`, and decide them from the staff
  queries; a test holds them to it until story (c) moves them into a
  package. The daily chance of a new customer leaves the bank's start of
  day for the simulation's run loop. The contract suite checks the two
  commands on every implementation. `core.CustomerID(n)` names the
  register's numbering.
- The `accrual_state` table, unwritten since v0.11.0, is dropped (products
  schema version 2); the products component owns no table.
- The upgrade drill runs on preprod, never on prod; prod is promoted once
  the drill has finished there (`upgrade-drill.md` step 9). ADR-0002 names
  market data (the base rate) an injected source alongside the clock.
- `benchmark.md` carries the first Hetzner run, small scale: 60,000
  customers reached at 142.1/s, 18,388,121 account days per 12h, the
  last day 4m41s over 119,794 accounts. The rows are now written by
  gobank-deploy's `perf` command into this checkout; `large` is a ccx33
  with dedicated cores.

## [0.12.0] - 2026-10-06

 - about.json carries the two performance rates; benchmark.md is the published performance doc

 - about.json carries the two performance rates; benchmark.md is the performance doc, published

### Changed
- The start-of-day pass is no longer paced over the day length: it runs
  at the start of the day at the system's capacity, whatever the length,
  and the rest of the day is idle. The length is headroom, as the night
  is for a bank's overnight run, not a load to even out (ADR-0002
  amended). A day length set on the console now applies to the day in
  progress: the loop's idle wait is recomputed, so setting zero starts the
  next day at once, and the dashboard's countdown follows.

### Added
- `/about.json` `sim` gains the two rates a performance run reads:
  `customers_per_sec` (the live rate while a batch add runs, else the
  last batch's), `account_days_per_12h` (the start-of-day pass's rate over
  the whole day, projected to 12h), with `adding_customers`,
  `last_day_duration` and `last_day_accounts`. gobank-deploy's perf
  workflow reads them on a Hetzner environment and the results go in
  `benchmark.md`, now the performance doc: a dated row per run and scale
  above the laptop pglike baseline, published by `docs:build` and linked
  from the docs landing page with the upgrade drill.

## [0.11.0] - 2026-10-05

 - Stage 3 story (e): start-of-day workflow paced over the day and resumable; products engine map gone

 - The day length set on the console is the run's and outlives a restart
 - Stage 3 story (e): the start-of-day workflow — the pass is paced over the day, resumes after a restart, and the products engine's account map is gone

### Changed
- ADR-0002 stage 3 story (e), the start-of-day workflow. The date advances
  at the start of the slot; then the pass visits every registered account
  once, under its lock, and writes its position for the day: yesterday's
  cycle-end application is booked first, at yesterday's last second, and
  the day's interest accrues on the balance as it stands. The pass is
  paced so the share of accounts done tracks the share of the day length
  elapsed (flat out at zero), and it resumes after a restart or a stop
  from the projections already written: an account with a position for
  the day is done, whoever wrote it, so the work left is exactly the
  accounts without one, and the date does not advance until none are
  left. A transfer or a funding runs the same day rules on the accounts
  it touches, so the projection is rewritten on the closing balance; a
  new account has a position from the day it opens. The runtime page's
  phase is "projecting positions" with the accounts done, and the
  dashboard tile is "Account days / 12h": whether the pass, at its
  measured rate, could do a day's accounts overnight. Measured on pglike:
  0.47 ms per account-day at 10,000 accounts, the whole day.
- The products engine (`gbp.Simulation`) and its account map are gone from
  the demo, with `simMu`: the demo runs `Product.Apply` and
  `Product.Accrue` (gobank-products v0.3.0) for one account at a time over
  the ledger's positions, and reads balances and accrued interest from the
  ledger's views alone. The daily whole-penny postings into the
  AccruedInterest holding accounts are gone with it: a customer account
  posts nothing daily, its accrual is on its position, and interest totals
  on an accrual basis read the applications from the P&L accounts and the
  accrual from the positions (truncated once per family rather than per
  account). The BoE reserve keeps its daily posting and month-end receipt,
  computed at the start of the slot for the day just closed.
- `accrual_state` is no longer written. The table stays one release so a
  rollback to v0.10 finds it; the next release drops it.

### Fixed
- A day length set on the settings page lasted only as long as the
  process: an upgrade restarted the demo at `GOBANK_DAY_LENGTH` (flat out
  on prod), so the upgrade drill's 2h day was gone the moment it was
  needed and the days raced past the drill's readings. The setting is now
  recorded with the run (`sim_run.day_length`, simulation schema version
  3) and a resumed run keeps it; `GOBANK_DAY_LENGTH` is where a run
  starts until the console sets one.

## [0.10.2] - 2026-10-05

 - Dashboard interest throughput is measured over the whole day, projection included

### Fixed
- The dashboard's "Interest movements / 12h" was quoted at the accrual
  phase's rate alone (engine plus postings), which since v0.10.0 is the
  cheap part of a day: projecting every account's position is most of it
  and ran after the sample was taken, so the figure was about ten times
  the rate the movements table grows at. The rate is now measured over
  the whole day, begin to finish, the span the runtime page reports as
  the last day's duration.

## [0.10.1] - 2026-10-05

 - Fix v0.10.0 deadlock: customer funding no longer projects; the day's pass writes a new account's first position

### Fixed
- v0.10.0 stopped within minutes on Hetzner: the day stuck in "products
  engine" and no customers added. An in-day event (a transfer, a funding)
  and the daily pass both rewrite an account's position inside a
  transaction that holds its rows, and the demo serialised events on the
  global engine mutex, held across the write: one customer creator's
  transaction held the shared equity account's position row while
  another, holding the mutex, waited for it, and the engine stopped
  behind them. Now each ledger account has its own lock (one of 4,096
  striped by a hash of the ID, so the set is a few kilobytes), held for as
  long as the work holds the account's rows: a transfer takes its two accounts,
  a customer creation takes the equity account and the new accounts until
  it commits, and the daily pass takes each account as it projects it. An
  event and the pass take turns on one account while every other account
  carries on; the engine mutex is held only for the cache update. That is
  the cost of projecting one day ahead: in-day events are heavier and
  end-of-day work is smeared across the day.

## [0.10.0] - 2026-10-05

 - Stage 3 (c) and (d): interest application inside the daily pass; positions are the truth, read from the ledger's contract views

 - ADR-0002 stage 3, story (c), gobank side: gobank-products v0.2.0,
   go-luca v0.3.0 and go-postgres v0.7.0. Interest application is inside
   the engine's daily pass (`Product.NextDay`, cycle a product parameter),
   so the day's update carries the applied interest and the demo reads it
   from there.
 - ADR-0002 stage 3, story (d): projections are the truth. After each
   day's sweep the demo projects every account's position into the ledger
   (`Project`: balance from the movements, the engine's accrual numerator
   on the row) and reads them back at start (`Positions`), the BoE reserves
   account's accrual among them. A transfer or funding is posted with
   projections, so both accounts' positions for the day move at once.
   Customer balances and accrued interest are read from the ledger's
   contract view `contract_ledger_live_positions` (money as NUMERIC,
   parsed exactly), not from the engine's cache: `core.Account` and the
   customer read model carry `AccruedE7` (7 decimal places of a pound)
   in place of the engine numerator. The demo's own
   `contract_ledger_movements` view is gone; go-luca publishes it.
   `accrual_state` is no longer read; it is still shadow-written so v0.10
   can be rolled back to with nothing lost, and story (e) drops it.

## [0.9.0] - 2026-10-04

 - Stage 2 complete: stored daily snapshots and database sessions; customer page PII and layout fixes, UTC payment times, product currency

 - ADR-0002 stage 2, story (e): the dashboard's daily series (book,
   customers, NIM, BoE base rate) are stored daily snapshots: the
   `history` component, one row a day in `daily_snapshots`, written when
   a day begins and never rewritten, so a restart on Postgres shows the
   same charts and the position's NIM is the latest snapshot's. The
   in-memory slices are gone; the charts draw the newest 7,300 days.
 - ADR-0002 stage 2, story (f): app sessions live in the database. The
   BFF's session store is a contract (`bff.Sessions`) with the memory
   store it had and `bff.SQLSessions` over a `sessions` table (token hash,
   customer, created and last-seen times); `Config.SessionDB` picks it.
   The demo passes its database and registers the table as the `sessions`
   component, so a restart or an upgrade keeps customers logged in.
   `cmd/bff` on the stub bank stays in memory.
 - With (e) and (f), stage 2 (stored truth) is complete: a restart on
   Postgres resumes with the same transactions and charts. Both
   migrations only add a table, so v0.8.0 runs on the same database and a
   rollback is possible.
 - Customer detail and account pages show the customer's name only with
   PII authorisation, falling back to the ID as every other page does
   (#23), and no longer carry the inline phone preview, so the record has
   the page (#24); the Bank App button opens the real app.
 - Payment Created and Settled times are full UTC datetimes
   (`2006-01-02 15:04:05Z`) on the detail page, its timeline, the
   payments list and the customer report (#22).
 - Currency is a property of the product (`Currency`, ISO 4217; GBP for
   every product today) and reaches `core.Account`, `core.Transaction`,
   `core.Product` and the screen tree (`currency` on row and tx
   components; additive, schema stays v1). `screen.Glyph` picks the
   savings glyph from it (pound, dollar, euro or yen banknote; a money bag
   otherwise) and the demo's phone frame renders through it and
   `bff.TxIcon`, so the dollar sign is gone and the icon tables exist once
   (#26).

## [0.8.0] - 2026-10-04

 - Stage 2 story (d): transactions as a ledger projection

 - ADR-0002 stage 2, story (d): a customer's transactions are a projection
   of the ledger. Every statement line is a movement on one of the
   customer's ledger accounts, read from `contract_ledger_movements` (which
   now carries both account paths and the knowledge time) with the balance
   it left; the in-memory `txLog` and its 100k cap are gone, so a restart
   shows the same transactions and no history is ever trimmed. A
   movement reads as Deposit, Loan, Transfer In, Transfer Out, Interest or
   Loan Interest by its code, counterparty and the account's family
   (`txTypeOf`). The funding movement of a new customer now carries its
   payment reference, as transfers do. `core.Transaction.ID` is the
   movement's ID (a string, was an int).

 - `GET /about.json`: the process for another program — version, schema
   version per component, console settings (day length), the run (running,
   time left in the day), the position and the restart record with
   downtime and whether the handover was intact. gobank-deploy's upgrade
   drill reads it before and after each redeploy; `upgrade-drill.md` says
   how that automated run maps onto the manual steps.
 - The about/runtime page's Data Store box shows the schema version each
   component's tables are at, as `/about.json` does.

## [0.7.0] - 2026-10-04

 - Adding restart component

 - ADR-0002 stage 2, story (c): the restart record. Every process start
   writes a row to `restarts` (simulation component, schema version 2:
   a new table only, so v0.6.0 runs on the same database and a rollback
   is possible) with its version, the run's day and customers, and the
   version and stop time of the process it follows; a clean stop (SIGTERM,
   after the day in progress and the HTTP drain) writes the stop time and
   the run as it leaves it. The settings page shows the record newest
   first: downtime per restart, day and customers at the previous stop
   against this start (red when they differ), and "unknown (unclean
   stop)" when the previous process was killed. A process from before the
   record (or a rollback to one) shows as unrecorded, its downtime
   measured from its last write of the run row.
 - `upgrade-drill.md`: the in-place upgrade and rollback drill on Hetzner,
   every later story's acceptance (ADR-0003).
 - `task-plus.yml` declares maturity code 1 (branch per story, pull
   request) and deploy 2 (upgrade in place, downtime recorded, rollback
   rehearsed).

## [0.6.0] - 2026-10-04

 - Stage 2 story (b): resume from the database, schema versions, static settings form, end-of-day countdown

 - Stage 2 story (b): the demo resumes its run from the database; schema versions and migrations; static settings form; end-of-day countdown

 - ADR-0002 stage 2, story (b): resume. The demo no longer drops its
   tables at start: on PostgreSQL it rebuilds the bank from the database
   (the engine adopts every registered account with its ledger balance,
   accrued interest and the book totals are read back, sequence numbers
   continue) and carries on from the recorded day. A run that was going
   when the process stopped starts again by itself. SIGTERM finishes the
   day in progress before the process exits, so an in-place upgrade
   (ADR-0003) leaves the database consistent; the daily histories and the
   transaction log restart from the resumed day until stories (d) and
   (e) store them. A database from before this release (which was never
   meant to outlive its process) is started fresh, once.
 - Schema versions: every component that owns tables declares its schema
   as numbered migrations, applied once and recorded in `schema_versions`
   per component (the ad hoc `ALTER TABLE` is gone). Two new components
   in the registry: `schema` and `simulation` (the run row, `sim_run`).
 - Reset on PostgreSQL stays on PostgreSQL: it drops the run's tables and
   migrates again, where before it silently switched to the in-memory
   store.
 - Settings page: only the status line polls (its own HTMX fragment); the
   form is static, so a value being typed is no longer wiped before Save.
 - Dashboard: with a day length set, the day tile counts down to the end
   of the day.
 - gobank-products: `Simulation.AdoptAccount` (pinned at a pre-release
   commit until v0.1.11 is tagged).

## [0.5.0] - 2026-10-03

 - Stage 2 story (a): simulated day length (GOBANK_DAY_LENGTH); ADR-0003 upgrade-in-place; feature flags and payment rails on the roadmap

 - ADR-0002 stage 2, story (a): simulated day length. `GOBANK_DAY_LENGTH`
   (a duration such as `2h`; unset or `0` runs flat out) sets how long a
   simulated day takes in wall-clock time; the settings page shows and
   changes it, the dashboard shows it when set, and it takes effect from
   the next day. The run loop waits out what remains of the day after the
   day's work instead of a fixed 200ms.
 - Console settings (customer ceiling, day length) are a value behind
   `Get` and `Update` with their own lock, so the run loop and the pages
   never wait on the state lock; Reset keeps them. The BoE rate and
   reserve ratio are bank state, no longer settings.
 - ADR-0003: from stage 2 every release is an in-place upgrade of the
   running demo with recorded downtime, blue-green at stage 8; roadmap
   ticks stage 1 (v0.4.0), breaks stage 2 into stories, adds feature
   flags and multiple payment rails

## [0.4.0] - 2026-10-03

 - ADR-0002 stage 1 complete: staff queries and commands as core contracts, every staff page through them; Flutter platform folders and emulator tasks

 - Stage 1 seams complete: staff queries and commands through `core`

### Added
- ADR-0002 stage 1, story (d): the staff side of the core. `core.StaffQueries`
  is one query interface per component — `BookQueries` (position, P&L,
  balance sheet, daily history), `CustomerRegister` (register pages and
  records without PII; name and PII as separate calls so a page fetches
  them only once authorised), `PaymentQueries`, `ProductQueries` and
  `TreasuryQueries` — and every staff page (dashboard, customers,
  accounting, products, payments, reports, treasury, settings) now renders
  from them rather than from `DemoState`. The simulation console's own
  state (running, adding, rates, memory) is `SimStatus`, not a core query.
- ADR-0002 stage 1, story (e): `core.Commands`. `Transfer` moves money
  between two customers' savings accounts and refuses what the bank
  cannot do (`ErrInvalidAmount`, `ErrSameCustomer`, `ErrNotFound`,
  `ErrInsufficientFunds`); the payments generator now picks the customers
  and amount and calls it. `BuyGilt` buys at today's yield and refuses an
  unknown tenor or a face value under `core.MinGiltPurchase`; the treasury
  page calls it.
- The contract suite (`core/coretest`) covers the staff queries and the
  commands: the register sums to the position, the products carry the
  book, the statements balance, a transfer moves exactly what the
  queries then show. The demo adapter runs all of it.
- `core.Account` carries `AccruedNumerator`, the exact accrued-but-unapplied
  interest, so the customer report's sub-penny "accruing" figure comes
  through the contract.
- The Flutter app's `android/` and `ios/` platform folders are generated
  and committed; the Android debug manifest allows cleartext traffic so a
  debug build can reach the demo over plain HTTP (release builds cannot).
- Taskfile tasks for the Android emulator: `app:emulator` starts the
  Android Studio AVD and waits for boot, `app:emulator:stop` kills it,
  `app:run` runs the app on it (`BFF_URL=` to point it at a demo).

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
