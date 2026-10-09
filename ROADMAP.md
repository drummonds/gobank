# GoBank Roadmap

GoBank aims to be a realistic banking software core, validated by a simulation model.
The same code runs in the browser (WASM) and in production (Kubernetes),
with thin Flutter mobile apps talking to a Go backend-for-frontend.

![Roadmap](roadmap.svg)

## Phase 1 — WASM Model Bank

The browser-based model proves the banking core works end-to-end.

### Done

- WASM build and deploy (docs.bytestone.uk)
- lofigui web UI
- go-luca double-entry accounting integration
- RBAC infrastructure (admin role)
- Daily accrual cost no longer scales as a posting per account per day: accrual
  lives on the ledger position and the start-of-day pass runs at the start of
  the day with the whole day as headroom (ADR-0002 stage 3 story (e))

### To Do

- **Workflow engine** — a `workflow` component (own tables, ADR-0001):
  long-running processes made of steps, timers and human decisions,
  durable so an instance resumes after a restart, with timers on the
  bank clock (`core.Clock`) so a six-month wait runs inside a simulated
  afternoon. First consumers: KYC journeys and sign-up (Phase 2 #4),
  lending proposals, dormancy, licence promotion (below). Stage 1.12's
  single workflow runner becomes this engine's runner. WASM runs it in
  memory. Out of scope: a visual designer, BPMN, retries with backoff
  beyond "try again next day". Open: definitions as Go code (steps are
  named functions, the instance row records the step it is at) versus a
  declarative definition table; whether the start-of-day pass itself
  becomes an instance
- **Bank licence trajectory** — the bank is an institution that grows
  through tiers: a deposit-taker (savings products only, no lending, no
  regulatory capital), a small bank once it has raised investment or
  retained 5M, a regulated bank at 25M with capital rules. A tier is a set
  of permissions the rest of the bank reads as rules: lending headroom
  is nil at tier one, the reserve-ratio rule at tier two, capital-based
  at tier three. A `licence` component (own table) holds the current
  tier and the history of transitions — date, trigger (investment event,
  earnings threshold crossed), the figures at the time, who approved —
  and a staff page shows the trajectory: where the bank is, what the
  next step needs, how far along. Promotion is a workflow with a staff
  step; the simulation can inject an investment event. Thresholds are
  data, not code. Out of scope: real PRA/FCA regimes (mobilisation,
  Part 4A), ICAAP/ILAAP numbers (the risk register keeps those out too).
  Open: the thresholds (5M and 25M are the model's own; UK mobilisation
  is a £50k deposit cap) and whether a tier can be lost
- **Lending gateway** — a loan is a proposal, not a disbursement: opening
  a customer with a loan, or a later request, creates a lending proposal
  that a workflow decides — refused at the deposit-taker tier, approved
  within headroom at later tiers, with a staff approval step above a
  size. Replaces the open-time trim in `planCustomerLocked`; the proposal
  and its decision are records, so the staff view shows a queue and the
  refusals. The generator keeps asking for loans at tier one so the
  refusals are visible. Out of scope: credit scoring, affordability,
  repayment schedules (products)
- **Dormancy and the account-number lifecycle** — a savings account with
  no customer movements for six months (interest credits do not count)
  becomes dormant; dormant with a nil balance it is closed automatically,
  with a balance it stays dormant and is reported. Closing returns the
  account number to quarantine for a period before it can be reissued,
  so numbers have a lifecycle: issued → open → closed → quarantined →
  free. The sweep is a workflow timed from the last customer movement.
  Out of scope: the Dormant Assets Scheme (15 years), a reactivation
  journey. Open: the quarantine length; whether the number register is
  its own component or part of customers. Real UK banks do not reuse
  numbers; the model does so the register stays finite
- **Treasury: cash at BoE and net interest margin** — the treasury view
  gains the BoE balance over time (the reserve account's daily position
  from the ledger) and, for a reporting period, the net interest margin:
  interest earned (BoE reserves, gilts, loans) less interest paid
  (savings), as money and as a rate over average earning assets. Builds
  on period accounting reports
- **Feature flag component** — a `flags` component (own table, ADR-0001)
  holding named on/off switches, read through a core query and flipped from
  a staff page; WASM keeps them in memory. Needed for the ADR-0002
  transition, where an old and a new path run side by side (stage 2
  computed versus projected transactions, a new payment rail) and the demo
  must switch between them per deployment without a rebuild. A flag is
  short-lived: it is deleted once both paths converge. Out of scope:
  percentage rollouts, per-customer targeting, external flag services.
  Open: startup-only (read once from config; simple, no table, every process
  agrees) versus live (stored, flippable mid-run; lets the demo show a
  cutover or a kill switch without a restart)
- **Key and emerging risk register** — a register component (own tables,
  ADR-0001) listing the bank's key risks (credit, liquidity, interest-rate
  risk in the banking book, operational, conduct) with owner, inherent and
  residual rating and controls; each key risk carries KRIs computed from the
  simulation through contract views, with amber/red thresholds, so a
  simulated run moves the register. Emerging risks (horizon scanning: cyber,
  climate, payment-scheme change) are recorded without KRIs until one is
  measurable. Out of scope: capital modelling (ICAAP/ILAAP numbers).
  Open: which KRIs the simulation can already support versus which need new
  metrics (e.g. liquidity coverage needs a treasury cash view)
- **DB explorer on the pluggable go-dbexplorer** — fix FK filter links
  (route through `Explorer.Render`); then a `Catalog` adapter over
  `components.go`. The `Authoriser` is story 1.7.3
- **Working savings accounts** — full lifecycle: open, deposit, withdraw, accrue interest, close
- **Import/export via luca files** — load and save simulation state as plain-text accounting files
- **Export single savings account** — extract one account's history as a luca file
- **Period accounting reports, produced and stored** — a reporting
  period (month, year, tax year for BBSI) is a thing with a close: at
  close the P&L, balance sheet and the period's returns (BBSI at tax
  year end) are produced from the ledger as at that knowledge time and
  stored as records, so what was reported stays what was reported if a
  late posting changes the books; real time is a live view of the open
  period, never stored. The reports page lists periods and their stored
  reports; BBSI moves from a live build to the stored year-end report.
  Needs go-luca `knowledge_time` (1.3.2). Open: a posting after close —
  restate, or carry into the next period with a note
- **Customer model** — confidence score, maximum aggregate balance, realistic switching behaviour
- **Named customers behind customer wall** — PII-like data gated behind access control
- **mock-fps integration** — compiled-in Faster Payments simulator for payment processing
- **FPS stand-in mode** — simulate FPS outages, payment queuing, and recovery

## Telemetry — between Phase 1 and Phase 2

The demo tells us what it is doing through the runtime page and the day
length; from stage 3 of ADR-0002 onwards the cost of the design (events
and the daily pass taking turns per account, projections written through
the day) has to be measured, not inferred. Telemetry is its own item so
that every later story ships with its numbers.

- [x] (v0.12.0) **Performance run on Hetzner, small and large** — a gobank-deploy
  `perf` workflow (beside the drill) creates an environment at a scale
  (`small` cx23, `large` cx53), waits for the demo, sets the day length to
  zero, batch-adds customers for a fixed wall-clock span, lets days run
  for another, reads the two rates off `/about.json` — customers added per
  second and account days per 12h (the pass's rate over the whole day) —
  with the customers and days reached, then downs the box and keeps the
  observations with the version and server type. gobank's share: the two
  rates join `/about.json`, and `benchmark.md` becomes the performance
  doc — published by `docs:build`, each run a dated row per scale,
  the laptop pglike tables kept as the baseline. Out of scope: profiling
  on the box, OpenTelemetry (below), cost per customer. Open: the spans
  (10 minutes each?); whether a run should also report the day's
  wall-clock at a fixed customer count, for comparison with the
  2026-10-01 ten-minute days
- **Observability with OpenTelemetry** — the demo emits metrics and traces
  through the OpenTelemetry SDK, OTLP to a collector per environment that
  gobank-deploy runs alongside the demo, starting with what the runtime
  page shows today: day phase and duration, movements per second,
  database connections, memory. Out of scope: a logs pipeline, alerting.
  Open: OTLP push versus a Prometheus pull endpoint; where the collector
  and viewer live on a one-server environment
- **Account locks in use** — once the above is in place: how many of the
  per-account lock stripes are held, how often a holder waits and for how
  long, with the equity account's stripe named, so the cost of an event
  and the daily pass taking turns on an account (ADR-0002 stage 3) is
  measured rather than inferred from the day length

Done when a Hetzner day can be read as a timeline of its phases and lock
waits in the viewer, and the drill records its downtime from the same
source.

## Phase 2 — Mobile apps: Go BFF + Flutter thin client

Thin iOS and Android apps with as little on the device as possible. All banking
logic stays on the server; the client renders, navigates, and holds the keys.
The BFF is hardened from the start: apart from login, health and the public
login screen it answers nothing without a session, so authentication is built
first and every other feature sits behind it.

### Architecture

- **Backend-for-frontend (BFF)** — a Go service that speaks only to the clients
  and returns screen-ready JSON. Its own binary (`cmd/bff`), separate from the
  admin simulator, with a narrow internal API (`bff.Bank`) to the banking core.
  Customer identity comes from the session, never from the URL.
- **Screen layer** — a versioned screen tree (`screen` package) built
  server-side with every presentation decision made in Go: money formatting,
  sign and colour, icons, date grouping, paging, navigation. Rendered to JSON
  for the apps and to HTML for the browser and the demo phone frame.
- **Flutter thin shell** — one Dart codebase for iOS and Android (`app/`). A
  JSON-to-widget interpreter plus login and navigation; no business rules in
  Dart, so it stays cheap to replace. Unknown component types render a fallback.
- **In-house native security plugin** — Swift and Kotlin, written by us rather
  than assembled from pub.dev packages. This is the code a later native rewrite
  would need, so it carries over unchanged.
- **WASM stays the simulator** — the browser model bank runs the whole bank
  in-page with no server, so it cannot authenticate against a BFF. It remains
  the admin and simulation tool; the customer web client is served by the BFF.

Everything lives in this repository: `screen/` and `bff/` are packages of the
root module, `cmd/bff` is the service binary, and `app/` is the Flutter shell.

### Started

- **Screen layer** — `screen/`: schema v1, JSON and HTML renderers
- **Hardened BFF** — `bff/` and `cmd/bff`: sessions, credential login, login
  rate limiting, audit log, authenticated screen endpoints only; runs against
  `bff/stubbank` standalone, and inside the demo over the `core` adapter
  (ADR-0002 stage 1) at `/v1/` on the demo's port
- **Flutter shell** — `app/`: login, screen renderer, navigation; session token
  held in memory until the native plugin exists

### To Do (in order)

- **1 Transition to the target architecture** — [ADR-0002](adr/0002-target-architecture.md):
  one BFF for app and web, a core library of components over a database
  that holds every fact, accruals pipelined one day ahead by a start-of-day
  workflow, and a simulation that differs from production only in event
  sources and the clock. Nine stages, each leaving the demo running in
  WASM, on a server and on Hetzner. The list below is the ADR's nine
  stages with the stages added since (1.6a, 1.7 to 1.10) in the order
  they are worked; a shipped story keeps its number, so an insertion
  before one takes a letter (1.6a) and only unshipped stages are
  renumbered. Every story ends with a release deployed to the Hetzner
  demo and sense-checked there before the next starts:
- **1.1** [x] (v0.4.0) Seams — core commands/queries as interfaces; `DemoState` adapts to
  them; the BFF runs in the demo process and the app shows real data.
  Stories:
- **1.1.1** [x] (v0.3.54) customer contracts in `core`, contract suite,
  demo adapter, BFF at `/v1/`
- **1.1.2** [x] (gobank-deploy v0.3.0) gobank-deploy sets `GOBANK_APP_PASSWORD` per environment
  and shows it (its story 1g, gobank-deploy v0.3.0)
- **1.1.3** [x] (v0.4.0) Android debug build — `app/android` and `app/ios`
  generated and committed, cleartext traffic allowed in the debug
  manifest only — then (a) sense-checked from a phone against
  Hetzner (2026-10-03)
- **1.1.4** [x] (v0.4.0) staff queries — `core.StaffQueries`, one
  interface per component (book, customer register, payments,
  products, treasury), every staff page rendering from them
- **1.1.5** [x] (v0.4.0) commands — `core.Commands`: `Transfer` (the
  payments generator calls it) and `BuyGilt` (the treasury page
  calls it). Opening a customer stays inside the generator until
  stage 4 splits generators from the bank
- **1.2** [x] (v0.9.0) Stored truth — transactions as a ledger projection, stored chart
  snapshots, sessions in the database. From here every story is an
  upgrade of the running Hetzner demo, downtime accepted and recorded
  until stage 8 ([ADR-0003](adr/0003-upgrade-in-place-until-stage-8.md);
  deployment level 2 on the manual's
  [maturity ladder](https://man.bytestone.uk/maturity.html)), and
  development moves to a branch per story merged by pull request on
  the Forgejo (code level 1). Stories:
- **1.2.1** [x] (v0.5.0) simulated day length — a setting (`GOBANK_DAY_LENGTH`,
  zero means flat out; shown and set on the simulation page) so a
  day can take two hours and an upgrade lands mid-day
- **1.2.2** [x] (v0.6.0) resume — the demo stops dropping its tables at start and
  rebuilds its state from the database, with a schema version table
  and versioned migrations replacing the ad hoc `ALTER TABLE`. Also,
  from (a)'s sense-check: the settings form is static under the page
  poll (an HTMX status section polls, the form does not, as the
  dashboard already does) so a value being typed is not wiped before
  Save; and the dashboard's day tile counts down the seconds to the
  end of the day
- **1.2.3** [x] (v0.7.0) upgrade drill — the demo keeps a restart
  record (`restarts`, an expand-only migration of the simulation
  component): each process start against the stop before it, with
  the downtime and the run's day and customers on both sides, shown
  on the settings page; `upgrade-drill.md` is the drill — redeploy
  N to N+1 on Hetzner mid-day, read the downtime off the record,
  roll back to N from the release store and forward again, confirm
  nothing is lost — and every later story's acceptance. Needs
  gobank-deploy story 1i (stop timeout, fetch a named tag)
- **1.2.4** [x] (v0.8.0) about/runtime shows the schema version per
  component, as `/about.json` already does
- **1.2.5** [x] (v0.8.0) transactions as a ledger projection replacing `txLog`
- **1.2.6** [x] (v0.9.0) chart histories as stored daily snapshots —
  the `history` component, one row a day in `daily_snapshots`
- **1.2.7** [x] (v0.9.0) sessions in the database — `bff.Sessions`,
  with a SQL store over the demo's database (the `sessions`
  component)
- **1.3** [x] (v0.11.0) Pipelined accruals — start-of-day workflow writes next-day
  projections; interest application is product code inside it. A
  projection is an account's position at the start of a day: its
  balance and its accrued-but-unapplied interest. go-luca publishes
  the position as contract views and decides how it is kept (stored
  projections, sums over ranges); gobank-products holds the rules,
  one account at a time, and stores nothing; the demo's products
  component owns no table once `accrual_state` is retired. Numbers
  cross the contract as NUMERIC, so how go-luca stores them
  (integers, numerator and denominator) stays inside it. Stories:
- **1.3.1** [x] (go-postgres v0.7.0) enabler, go-postgres: NUMERIC in pglike — a `::numeric`
  cast and arithmetic, rounding and comparison on it give exact
  decimals with PostgreSQL's scale rules, so a contract view can
  publish a NUMERIC column computed from integers on both drivers
- **1.3.2** [x] (go-luca v0.3.0) go-luca: two contract views — `contract_ledger_eod_positions`
  (account, day, balance, accrued; the cheap one, from stored
  projections written for both accounts of a movement, incrementally)
  and `contract_ledger_live_positions` (today's row plus today's
  movements; dearer, and said so) — with `contract_ledger_movements`
  moving in from the demo, a projection-only write for the daily
  pass, and `knowledge_time` stored on every write path (cold review
  go-luca #6). Done with the cost measured: rows per account per
  day at the Hetzner scale, and the two views benchmarked. Measured
  on pglike at 1,000 accounts: 0.55 ms per account for the daily
  pass, 0.34 ms for a one-account live read, 11 ms for a day of
  every account from the end-of-day view; one row per account per
  day. Found and filed go-postgres #21 (INTERVAL arithmetic on a
  qualified column)
- **1.3.3** [x] (gobank-products v0.2.0, gobank v0.10.0) gobank-products: `Product.NextDay` — a pure function from
  an account's projection and the day's balance to the next day's
  projection and the ledger postings it calls for, with the
  application cycle (daily, monthly, annual) a product parameter
  and month-end application gone as a separate pass; the existing
  sweep loops over it so the goldens hold
- **1.3.4** [x] (v0.10.0) projections are the truth — the demo writes each day's
  pass as go-luca projections and posts customer movements with
  projections; balance and accrued reads come from the views;
  `accrual_state` goes (the BoE reserve's accrual becomes a
  projection on its account). Shipped with `accrual_state` still
  shadow-written so the drill's rollback loses nothing; (e) drops
  it. Needed go-luca v0.3.1 (`Positions`, constant-cost movement
  projection, accrual carried forward on a movement's new day)
- **1.3.5** [x] (v0.11.0) the start-of-day workflow — the date advances at the
  start of the slot; the pass over every account runs at the start
  of the day at the system's capacity (the day length is headroom,
  the rest of the day idle) and resumes after a restart from the
  projections already written; a transfer or funding rewrites
  the touched account's next-day projection; the engine's account
  map and the end-of-day sweep go. Done-when of the stage; drilled
  with a restart mid-pass
- **1.4** [x] (v0.15.0) Events and clock — bank and simulation split; generators and an
  injected clock. The simulation differs from the bank in where its
  events come from and in the clock and market data it reads, nothing
  else; the stage ends with the simulation a package of its own that
  knows the bank only through `core`. Stories:
- **1.4.1** [x] (v0.13.0) entry points — `core.CustomerCommands.OpenCustomer` (record,
  PII, opening deposits and loan disbursements, with the lending
  headroom a bank rule that trims or refuses the loan) and
  `core.DayCommands.StartDay`; the generators and the console reach
  the bank through these and `Transfer` only, and the daily
  new-customer roll leaves `startDay` for the run loop. Drops
  `accrual_state`, retired since v0.11.0. Drilled on preprod 2026-10-06
- **1.4.2** [x] (v0.14.0) the clock — `core.Clock` injected into the bank, which takes
  its business date and every banking timestamp (payments, value
  times, join dates) from it; the simulation supplies a warped clock
  (day D begins at slot start, the day length sets the warp, flat out
  steps a day at a time) and the run row keeps day and slot start so
  a resume rebuilds it. Operational records (restarts, schema
  versions, session expiry) stay on the wall clock. The base-rate
  series becomes a second injected source, market data the bank
  reads and the simulation replays. `StartDay` follows the clock
  (nothing to do while it stands still; catches up a day at a time)
  and the position says whether the day's pass is complete, which is
  what lets the simulation step its clock. The dashboard shows the
  wall clock and the sim clock with the warp between them
- **1.4.3** [x] (v0.15.0) the split — the simulation moves into `cmd/demo/sim`, built
  on `core` alone: run loop, generators, settings, rate series and
  console status; it drives the bank through `core.Commands` and
  reads it through `core.StaffQueries`. Reset becomes wiring (a fresh
  bank over a wiped database); separate locks replace the shared
  `ds.mu`. Done-when of the stage: the compiler is the enforcement
  and a test checks the package's imports
- **1.5** [x] (v0.18.0) Core into packages — one component at a time, into `bank/<component>`
  packages of the root module, each owning its tables and migrations behind
  its API, with `bank` the composition root that implements `core.Commands`
  and `core.StaffQueries`. Smallest coupling first so the pattern (the
  registry names a package, the contract-view test scans `bank/`, an
  import test is the boundary) is proven before the entangled components
  move. Every story is a pure move, no schema change. Done-when of the
  stage: `cmd/demo` holds only wiring, the console and the UI. Stories:
- **1.5.1** [x] (v0.16.0) treasury — `bank/treasury`: gilt yields and holdings,
  `Buy`; reads only the bank's business day
- **1.5.2** [x] (v0.17.0) history — `bank/history`: the daily snapshots
- **1.5.3** [x] (v0.17.0) ledger — `bank/ledger`: the go-luca wrapper, the chart of
  accounts, the account locks and `postEvent`
- **1.5.4** [x] (v0.18.0) products, customers, payments and the bank, as one story:
  `bank/products` (the catalogue, the pass and the accrual over the
  ledger), `bank/customers` (the store, the register, opening and
  transactions, over ledger and products), `bank/payments` over all of
  the above, and `bank` the composite that replaces `DemoState` and
  dissolves `coreAdapter`; book and about read through `core`. Stories
  1.5.4 to 1.5.7 of the original breakdown, taken together and released
  once; the stage's done-when
- **1.6** [x] (v0.24.0) One BFF — the staff UI and the customer web render through the
  BFF, which becomes the demo's one handler, and the WASM build serves that
  handler in the tab (ADR-0002 stage 6; absorbs item 5 and the HTML half of
  item 8, passkeys excluded). The BFF keeps no state of its own: the console
  it serves is an interface the demo implements. Out of scope: a staff
  login (the role switch stays; 1.7 adds it), staff sessions in the
  database (1.12 needs them), the audience split, and the screens app.md draws (its stories
  1 to 4). Done-when of the stage: `cmd/demo` holds no HTML and no handler,
  only wiring, the simulation and the console. Stories:
- **1.6.1** [x] (v0.19.0) one handler, in the tab — the demo's routes become one
  `http.Handler` built over the state (no default mux, no closures in
  `main`); the server listens on it and the WASM build serves the same
  handler in the tab through lofigui's service worker (`wasmhttp.Serve`,
  lofigui examples 06 and 07), replacing the forty `goRender*` exports, the
  page switch in `app.js` and the WASM-only `index.html` with the layout,
  HTMX polling and links the server already has. A pure move for the
  server. Settled: the staff session in the tab is a fixed ID (the
  browser drops Set-Cookie on a service worker's response); pages are
  scope-relative against a `<base>` so the demo runs at `/` on a server
  and at `/demo/` on the docs site, with redirects carrying the scope.
  Done: `main_wasm.go` registers the handler and nothing else; `test:wasm`
  drives the binary over the worker's own Request/Response protocol under
  `/demo/`; the docs demo serves the server's pages
- **1.6.2** [x] (v0.21.0) customer web through the BFF — the hand-rendered phone frame
  (`/app/`, `bankapp_*.go`, `LayoutBankApp`) and the open `/api/customers`
  and `/api/customer/` endpoints are retired; the demo's Bank App link
  opens the BFF's HTML at `/v1/screen/login`, whose phone frame is the
  demo's customer web; the WASM build logs in with a fixed demo password
  the login screen states. Item 5, app.md story 5, the HTML half of item 8.
  Done-when: no customer data is served without a session; app and browser
  render from one screen tree. Done: the BFF takes the scope it is mounted
  under (`bff.Config.Scope`) and its pages, redirects and cookie carry it;
  the tab keeps the session cookie the service worker cannot set
  (`keepCookiesInTab`); the login note is the deployment's
- **1.6.3** [x] (v0.22.0) staff pages through the BFF — the layout, the role switch, PII
  authorisation and the pages that read the core (dashboard data,
  accounting, products, customers, payments, treasury, reports, about and
  docs) move to `bff/staff`, mounted by `bff.Server`; the BFF is the
  demo's handler. Pure move over `core.StaffQueries` and `core.Commands`.
  Done: `staff.New(staff.Config)` is the staff web, served by the BFF for
  every path outside `/v1/` (`bff.Config.Staff`); the console mounts its
  own pages on it (`Site.Handle`) and renders them in the layout
  (`Page`, `StaticPage`, `Fragment`, `Role`, `PII`, `Require`,
  `Redirect`) until 1.6.4; `TestStaffWebKnowsOnlyTheCore` holds the
  package to the core and its UI libraries
- **1.6.4** [x] (v0.24.0) the console through the BFF — the dashboard controls, settings,
  runtime, restarts, export and import and the DB explorer move to
  `bff/staff` over a `Console` interface (start, stop, advance, reset, add
  customers, the payments generator's send, run and stop, settings,
  restarts, export, import, explorer, the status `/about.json` reports)
  that `DemoState` implements, replacing `Site.Handle`. Done-when of the
  stage. Done: `staff.Config.Console`; the site serves the console's
  pages only when given one; `internal/daylength` parses the day length
  for the simulation and the settings page; `cmd/demo` holds no HTML and
  no handler
- **UUID columns across the family** (go-luca #10) — every column that holds a GUID is
  typed `UUID`, not `TEXT` or `VARCHAR(36)`: 16 bytes instead of 36 in
  every row and index (the movements indexes at 44M rows a year are the
  cost that matters), and the database rejects a non-UUID. Walk the
  dependency tree bottom up (`docs/research/gobank-family.d2`), one
  release per repo, pins bumped up the tree, one gobank release drilled
  on preprod: go-postgres (verify pglike translates `ALTER COLUMN … TYPE
  uuid USING …::uuid` and that UUID text compares case-insensitively as
  Postgres does; `UUID`→`TEXT` and `gen_random_uuid()` already exist);
  go-luca (`id`, `batch_id`, `*_account_id`, `customer_id`,
  `commodity_id` on every table, with FKs retyped together); gobank
  (`customer_accounts.ledger_account_id`, and sessions once they carry a
  ledger id). gobank-products stores nothing; gotreesitter, lofigui and
  gogal have no database. Before 1.6a.1, so the GL's tables are born with
  UUID columns and only one ledger is rewritten. Out of scope: columns
  that hold a human reference (`customer_id` and payment `from_id`/`to_id`
  as `cust-000123`, `PAY-000042`): they become UUID identity plus a
  reference column in ADR-0004's identity story, which this is the first
  half of. Open: the Postgres retype rewrites each table under a lock
  (minutes at the Hetzner scale; ADR-0003's accepted downtime) versus
  expand-and-contract with a new column, which pglike handles but
  doubles the migration; and whether gobanks-customers' `cust_*` ids are
  references or GUIDs today (checked in the go-postgres step)
- **1.6a** General ledger and sub-ledger (ADR-0005) — the one ledger grows
  as accounts × days while the bank's questions are per product, so the
  P&L never returns and the book is a sum over every account. Two go-luca
  ledgers in one database: today's ledger becomes the customer sub-ledger,
  unchanged; a small general ledger holds a control account per product
  and the bank's own accounts, is posted per event (control movements,
  inserts only) and by a journal at the GL close that follows the
  start-of-day pass, and is reconciled to the sub-ledger daily. The GL
  lags by the length of the pass and every GL read carries its day.
  Out of scope: retiring old sub-ledger positions (ADR-0002 data
  management), which this makes possible. Stories:
- **1.6a.1** [x] (go-luca v0.4.0) go-luca enablers (go-luca #8, #9) — a second ledger in one database (a table
  and view prefix on `NewSQLLedger`), and indexes for day-bounded reads
  (movements by value time, positions by day) so the journal and the
  reconciliation scan a day, not the table
- **1.6a.2** [x] (v0.26.0) the GL opened and posted in shadow — the chart, per-event
  control movements in the event's transaction, the journal and the GL
  close at pass completion (idempotent per day, product, code), the
  posted-through day, the reconciliation reported on the dashboard;
  reads unchanged. Done when preprod closes days with no break
- **1.6a.3** reads move to the GL — the book, accrued interest, P&L and
  balance sheet, the history series and lending headroom, each labelled
  with its day; the snapshot taken at the close; the BoE reserve moves to
  the GL; customer reads stay on the sub-ledger. Done-when of the stage.
  Worked inside 1.9, after 1.9.1 has measured the reads it replaces
- **1.7** Users and access — a `users` component (own tables, ADR-0001)
  in the bank's database: a user is a login name, a password hash and
  the roles it holds, and names the customer it is when it is one. Staff
  and customers are one table told apart by role. Chosen (2026-10-09)
  over an identity server: the tab needs its login in-process, and
  `OpenCustomer` creating a login is a call in the same transaction.
  Authelia, Authentik and Pocket ID were looked at and are staff-only
  front doors with no customer API; the hosted services do not run in
  the tab. The later split, if a second bank or SSO earns it, is Ory
  Kratos (Go, headless, admin API, CockroachDB-native, Apache 2.0,
  imports argon2id hashes); because the component sits behind its API
  that split is a move, not a redesign. The staff web gets a login and
  the role comes from the session, replacing the role switch; the app's
  one password per environment (`GOBANK_APP_PASSWORD`, `demo` in the
  tab) becomes each customer's own. Sessions are already stored (1.2.7).
  Out of scope: passkeys and WebAuthn (items 6 and 8), MFA, a
  password-reset journey, a separate identity service, the standalone
  RBAC module (item 10, which waits for a second consumer). Design facts
  that keep passkeys a later addition rather than a rework: a user is
  representable without a password (the hash is nullable; passkeys
  become a `user_passkeys` table of credential ID, public key, sign
  count, transports, AAGUID, backup flags and RP ID); the user's
  identity is its UUID v4 key, which is the WebAuthn user handle, never
  the login name; `core.Authenticator` stays password-shaped and a
  passkey login is a begin/finish ceremony behind an interface of its
  own with the challenge held in the stored session; the session records
  how it was authenticated and whether the user was verified, which
  step-up (item 7) asks; the relying-party ID is a domain per
  environment that gobank-deploy sets beside the app password, so an
  environment reached by IP cannot drill passkeys. Server library when
  they come: `github.com/go-webauthn/webauthn` (pinned, still v0);
  browser side is `navigator.credentials` on the BFF login screen; app
  side is Corbado's Flutter `passkeys` package with the `.well-known`
  association files served by the BFF. Settled
  (2026-10-09): argon2id at the OWASP minimum (19 MiB, t=2, p=1) from
  `golang.org/x/crypto/argon2`, measured against `x/crypto/bcrypt` native
  and under GOOS=js: 15 ms against 43 ms (bcrypt cost 10) on the laptop,
  52 ms against 52 ms in the tab, so the tab is indifferent and the
  server gets the memory-hard hash at a third of the cost; p stays 1
  because the tab has one thread. A hash costs more than a whole
  customer open does today (7 ms on Hetzner), so the generator (1.7.2)
  hashes the deployment password once per process and gives every
  generated customer that same salt and hash; only a customer who sets
  a password of their own pays a hash in the open path. Open: whether
  permissions become rows (role, action) or stay the code table they are
  today. Stories:
- **1.7.1** the component and staff login — the `users` table and
  migration, password hashes, roles; a login page on the staff web; the
  first admin's password a per-environment secret gobank-deploy sets and
  shows, as it does the app password today; the session carries the
  user and the role switch goes. Done when preprod is logged into with
  a staff password after the drill
- **1.7.2** customers as users — `OpenCustomer` creates the customer's
  user; `core.Authenticator` reads the component; the generator opens
  its customers with the deployment's password so the sense-check logs
  in as today, and `GOBANK_APP_PASSWORD` is no longer read at login
- **1.7.3** permissions with the users — `Role`, `Can` and the action
  list move out of `bff/staff` into the component, read through `core`;
  staff pages, PII gating and the DB explorer's `Authoriser` (the Phase 1
  explorer item) ask it, closing the gap where every role browses every
  table. Done-when of the stage
- **1.8** Products as versioned code, with bitemporal parameters
  (ADR-0006, proposed 2026-10-09; taken before 1.7.2 and 1.7.3) — today
  a product is a gobank-products value (id, name, family, a feature
  list and a string map of defaults) wrapped by the bank with a
  `float64` rate and a blurb, an account records only a product id, the
  feature and event framework in gobank-products is a second mechanism
  the bank does not run (no term lock, ISA allowance or overdraft limit
  is enforced), nothing says which rules an account was on when a
  posting was made, and the base rate is an injected function connected
  to no product. A product version becomes a Go package in
  gobank-products (`easyaccess/v1`, `easyaccess/v2`) with its own golden,
  immutable once adopted, answering one fixed event set (start-up, open,
  pre-posting, post-posting, day, parameter change, manual command,
  change of version, close) as pure rules: facts read through an
  interface, intents (positions, postings, refusals) returned, the bank's
  runner carrying them out under the account's lock. A rate is a
  parameter the version declares with a scope (bank, product version,
  account) and a source: a stored setting, or a derivation from another
  parameter in code (base rate − 15 bps, floor 0, possibly negative).
  Settings are bitemporal (effective-from, decided-at, append-only), so a
  rate change is decided today for a future day and takes effect with
  nothing to do; a rule change is a new version, a value change a
  setting. The bank stores adoption (`products`), the account's version,
  the settings (`bank/parameters`) and on every rule posting the
  version, event and resolved values (`product_postings`). The base rate
  becomes a bank parameter the simulation writes and the treasury reads;
  `core.BaseRateSource` goes. Settled from the earlier opens: the version
  is a per-product integer in code with the module version recorded on
  adoption; the posting's version is a bank table, not go-luca
  metadata. The build carries only the versions the bank has (on sale
  or with accounts on them); each version ships its own up (adoption)
  and down (withdrawal) migration, both additive, applied by the bank's
  migration and the drill's rollback like any component's; a retired
  version's package goes, and its postings and their records stay,
  carrying the inputs so they verify by arithmetic without the code; an
  older build leaves accounts on versions it lacks unprojected and
  counted. Out of scope: a product designer, products as data, bulk
  migration between versions (the per-account event is in), notice
  periods, corrections as adjustment postings, negotiated per-account
  rates. Stories:
- **1.8.1** gobank-products: the contract and v1 of every product —
  `Version`, `Facts`, events and intents, parameter declarations with
  scope and derivation; six `<product>/v1` packages reproducing today's
  behaviour exactly with the bank's rates moved in as published basis
  points; goldens re-pinned per package; the feature framework,
  `SimContext`, `Simulation` and `ParameterStore` retired. A library
  release; gobank stays on v0.3.0 until 1.8.2
- **1.8.2** the catalogue and the runner — `products` and
  `product_postings`, `product_version` on accounts (the migration
  adopts the six and stamps every account 1), `bank/products`
  dispatching open, post-posting, day and close to the account's
  version; the `float64` rate and the defaults map gone from the bank;
  the products page showing versions and the accounts on each
- **1.8.3** parameters — `bank/parameters` with settings and
  resolution; the base rate as a bank parameter the simulation writes;
  a staff page setting a product rate effective on a future day;
  `easyaccess/v2` as a tracker adopted on preprod mid-run, new accounts
  opening on it, its accrual seen moving when the base rate does
- **1.8.4** the remaining events — pre-posting (term lock, ISA
  allowance, overdraft limit as rules the payments path asks),
  parameter change, start-up, manual commands from the console, change
  of version per account from the staff account page. Done-when of the
  stage: an account moved from easy-access v1 to v2 on preprod, its v1
  cycle closed by postings that record v1
- **1.9** Speed — the demo is slow to use. Found by hand on 2026-10-08
  at the Hetzner scale: savings and lending 8 s (a movement sum per
  request), customers 9 s (not attributed), P&L and balance sheet never
  return (the interest reading is slower than the day that invalidates
  it), the explorer 50 s; background readings (v0.20.0) stopped pages
  queuing behind one another but not the reads themselves. Three
  suspects, told apart before anything is moved: a read slow in the
  database (a sum over every account, a missing index), a read
  serialised in-process (`b.mu`, simMu, one `sql.DB`) before any query
  is sent, and the simulation saturating the box the database shares.
  The database on its own server is the hypothesis to test, not the
  plan. Stories:
- **1.9.1** measure — every route records last, p50, p95 and max on
  `/about/runtime` and in `/about.json`, and every named read behind a
  page (the book, the interest, the product books, a customer page, the
  explorer's counts) records its duration and whether it was served from
  a reading (age, TTL) or live; a small Go loader in the repo hits the
  route list at concurrency 1, 4 and 16 and reports throughput and p95
  per route, so slow-but-parallel is told from serialised (more
  Postgres only helps the first); a `task bench:reads` runs the named
  reads against a database seeded at 10k, 100k and 1M customers and
  writes one table into `benchmark.md`
- then **1.6a.3** — the book, P&L and balance sheet from the GL: the
  designed fix for sums over every account
- **1.9.2** the known reads — the product books from the book reading
  (branch `products-from-the-book`), the customers page's queries, the
  explorer's counts, each with its before and after row
- **1.9.3** the database on its own server — gobank-deploy gives an
  environment a second server running Postgres on the private network
  and the demo's `GOBANK_PG_DSN` points at it; kept if 1.9.1's numbers
  say the shared box was the bottleneck, and in any case the first step
  of 1.12's topology. Done-when of the stage: at the Hetzner scale every
  staff page under 1 s at p95 at concurrency 4, the P&L and balance
  sheet return, and the rows are in `benchmark.md`
- **1.10** Performance and API documentation in the pipeline — the
  speed won in 1.9 is held by the pipeline, and the BFF's contract is
  documented from the code rather than by hand. Performance: `task
  check` (which `tp release` runs) runs `bench:reads` at the small scale
  against the thresholds 1.9 settled, so a slower read fails the release;
  the Hetzner perf run stays a per-release command (it costs a server)
  and its row joins the release notes. API documentation: an OpenAPI
  document for `/v1/` and the screen schema, served at
  `/v1/openapi.json`, rendered by `docs:build` into the docs site in
  place of the hand-written `docs/research/bff-api.html`, with a test
  that fails when a route is missing from it. Out of scope: client
  generation from the spec (the Flutter shell interprets screens, not
  endpoints), a perf run per commit, load tests in CI. Open: a
  hand-written spec checked against the router by a test (recommended:
  no new machinery) versus a spec generated from the Go types (swag or
  ogen)
- The last three ADR-0002 stages are deferred: they are not next after
  1.10
- **1.11** Read/write split — separate read and write handles (ADR-0002
  stage 7)
- **1.12** Many processes — several BFFs, a generator and one workflow
  runner; deploys go blue-green (deployment level 3), ending the downtime
  accepted since stage 2 (ADR-0002 stage 8)
- **1.13** Simulation becomes tests (ADR-0002 stage 9)
- **2 Multiple payment rails** — every payment records the rail it travelled
  (internal book transfer, FPS, Bacs, CHAPS) and each rail is a scheme
  adapter behind one interface with its own settlement timing, cut-offs,
  limits and outage behaviour; a routing rule picks the rail from amount and
  urgency. The payments component stays the one bank-level view (one
  lifecycle, one list, filterable by rail) and gains a per-rail view:
  volumes, queue depth, settlement position and scheme status. The existing
  mock-fps and FPS stand-in items in Phase 1 become the FPS rail's stories,
  and the adapter boundary is what Phase 3 unbundles. Out of scope: real scheme
  messaging (ISO 20022), cards. Open: rail as a column on the one
  `payments` table (the coherent view comes free) versus a table per rail
  with a bank-level union view (each rail's data differs — Bacs has a
  three-day cycle, CHAPS is same-day, FPS instant). The rails become real
  with a **real rail**: one outbound payment from the demo bank to a real account
  through a live bank API, credentials per environment via gobank-deploy,
  sandbox first. Out of scope: inbound, bulk, cards. Open: Starling
  (developer sandbox, personal access token, plain REST) versus Barclays
  (Open Banking PIS, needs TPP registration and eIDAS certificates, likely
  out of reach for a hobby project) — Starling sandbox first
- **3 Payee management** — a customer's saved payees (name, sort code and
  account number, reference), created, edited and deleted in the app and
  the customer web and offered by send money; a component with its own
  table (ADR-0001) behind session-only BFF endpoints. Builds on #7.
  Out of scope: international payees, a real Confirmation of Payee call
  (stubbed behind an interface). Open: new-payee limits and cooling-off
- **4 KYC process** — onboarding as a lifecycle (pending → verified →
  rejected → in review): identity capture, document and liveness checks
  as pluggable verifiers with a stub provider in the demo, running as
  a journey on the workflow engine (Phase 1), a risk rating, and the
  record of how and when a customer was verified and of later
  name and address changes (#25); an unverified customer transacts only
  up to a cap. Out of scope: a real IDV provider (same interface as the
  stub), AML transaction monitoring (belongs with the risk register).
  Open: does the customer generator produce KYC outcomes, so the staff
  view has a queue to work
- **5** [x] (v0.21.0) **Demo phone frame onto the screen layer** (ADR-0002 stage 6,
  story 1.6.2) — the demo's phone frame is the BFF's HTML (`screen.HTML`),
  so browser and app show identical screens from one source, and the open
  `/api/customer/` endpoints are retired
- **6 Native security plugin** — biometric-bound keys (Secure Enclave, StrongBox),
  passkey registration and login, App Attest and Play Integrity token fetching,
  certificate pinning, screenshot blocking and app-switcher blanking, jailbreak,
  root and overlay detection; the BFF checks attestation before issuing tokens
- **7 Device-bound signing** — request signing for transactions, step-up
  authentication (PSD2 SCA)
- **8 Web client on the BFF** — the HTML rendering of the same endpoints is
  the customer web client (story 1.6.2); passkeys via WebAuthn in the browser
- **9 App-shielding SDK evaluation** — Promon, Guardsquare, Appdome, Zimperium;
  chosen and integrated before any external pilot
- **10 Standalone RBAC module** — extract the users component's roles and
  permissions (1.7.3) into its own repo (not gobank-db) once a second
  bank is the second consumer; until then it lives in the bank's database
- **11 Native checkpoint** — after the first external pilot, a written list of what
  Flutter cannot do; move to SwiftUI and Jetpack Compose only if the list is
  non-empty

## Phase 3 — Kubernetes + AlloyDB

The ADR-0002 stage 8 topology (several BFFs, a generator, one workflow
runner, primary and replica) deployed to a Kubernetes cluster with a real
database.

- Deploy to Kubernetes cluster
- AlloyDB (Postgres-compatible) backend
- Unbundle mock-fps as a separate service (HTTP API)
- Full HTTP API for all GUI actions (automation/scenario testing)
- SCV regulatory report at scale (100K+ customers)

## Phase 4 — CockroachDB + Scale

The same stage 8 topology across regions on distributed SQL.

- CockroachDB backend
- Multi-region deployment
- Blue-green schema migrations (zero-downtime data swaps)

## What Success Looks Like

**Phase 1 complete** means: you can open the WASM demo, run a simulation, see accurate financials (P&L, balance sheet), export any account as a luca file, trigger an FPS outage, and watch the bank handle it. All actions available via API for automated testing.

**Phase 2 complete** means: you can open the app on a phone, log in with a biometric-bound passkey, see live balances served by the BFF, and make a deposit or withdrawal signed with the device key. The browser app runs against the same BFF. Nothing in either client decides anything.

**Phase 3 complete** means: the same code runs in Kubernetes with AlloyDB, mock-fps is a separate service, and SCV reports generate correctly at scale.

**Phase 4 complete** means: the bank runs across regions on CockroachDB with zero-downtime migrations.
