# GoBank Roadmap

GoBank aims to be a realistic banking software core, validated by a simulation model.
The same code runs in the browser (WASM) and in production (Kubernetes),
with thin Flutter mobile apps talking to a Go backend-for-frontend.

![Roadmap](roadmap.svg)

The roadmap is a set of named epics, each a list of items, and every
item an open issue labelled `roadmap` on the Forgejo (task-plus
`tp roadmap`). The forge says what exists and what is done; this file
groups and describes it. An item leaves the file when its issue is
closed, landing in the changelog under the version that finished it,
where every shipped story lives (the WASM model bank that came first,
then the nine stages of ADR-0002 and what followed them). Epic order
is file order, not a sequence: epics interleave. A story that serves an
item says so with a `**Roadmap:** #N` line.

## Transition to the target architecture

[ADR-0002](adr/0002-target-architecture.md): one BFF for app and web, a
core library of components over a database that holds every fact,
accruals pipelined one day ahead by a start-of-day workflow, and a
simulation that differs from production only in event sources and the
clock. Stages 1 to 6 and the general ledger's first half shipped
(v0.4.0 to v0.26.0); users and access, products as versioned code and
speed are epics of their own below. What remains here is the
transition's own leftovers.

- #66 UUID columns across the family (go-luca #10) — every
  column that holds a GUID is typed `UUID`, not `TEXT` or `VARCHAR(36)`:
  16 bytes instead of 36 in every row and index (the movements indexes
  at 44M rows a year are the cost that matters), and the database
  rejects a non-UUID. Walk the dependency tree bottom up
  (`docs/research/gobank-family.d2`), one release per repo, pins bumped
  up the tree, one gobank release drilled on preprod: go-postgres
  (verify pglike translates `ALTER COLUMN … TYPE uuid USING …::uuid` and
  that UUID text compares case-insensitively as Postgres does;
  `UUID`→`TEXT` and `gen_random_uuid()` already exist); go-luca (`id`,
  `batch_id`, `*_account_id`, `customer_id`, `commodity_id` on every
  table, with FKs retyped together); gobank
  (`customer_accounts.ledger_account_id`, and sessions once they carry a
  ledger id). gobank-products stores nothing; gotreesitter, lofigui and
  gogal have no database. Out of scope: columns that hold a human
  reference (`customer_id` and payment `from_id`/`to_id` as
  `cust-000123`, `PAY-000042`): they become UUID identity plus a
  reference column in ADR-0004's identity story, which this is the first
  half of. Open: the Postgres retype rewrites each table under a lock
  (minutes at the Hetzner scale; ADR-0003's accepted downtime) versus
  expand-and-contract with a new column, which pglike handles but
  doubles the migration; and whether gobanks-customers' `cust_*` ids are
  references or GUIDs today (checked in the go-postgres step)
- #67 Simulation becomes tests (ADR-0002 stage 9) — the
  scenarios the simulation runs by hand become a test suite, each
  building a definition of its own (ADR-0007)
- #133 Full API documentation (OpenAPI) — the running system describes
  its whole HTTP surface in one OpenAPI 3 document, served at
  `/v1/openapi.json` and rendered into the docs site by `docs:build`
  (Swagger UI or Redoc over the same file) in place of the hand-written
  `docs/research/bff-api.html`: every `/v1/` route with the screen
  schema its responses carry, the console and settings routes the
  operator token reaches, `/about.json` and the health check, each with
  its auth (session, bearer, none) and its errors. A test walks the
  router and fails when a route is missing from the document or the
  document names a route that is not served, so the two cannot drift.
  Out of scope: client generation from the spec (the Flutter shell
  interprets screens, not endpoints), the staff web's HTML pages. Open:
  a hand-written spec checked against the router by the test
  (recommended: no new machinery) versus one generated from the Go types
  (swag or ogen)

## Products as versioned code

[ADR-0006](adr/0006-products-as-versioned-code.md): a product version
is a Go package with its own golden, immutable once adopted, answering
one fixed event set as pure rules; a rate is a declared parameter with
a scope and a source, settings are bitemporal, and every rule posting
records the version, event and values it resolved. With it
[ADR-0007](adr/0007-bank-definition-module.md) (the bank's definition
as a module of its own) and
[ADR-0008](adr/0008-ledger-is-the-banks-view.md) (the ledger is the
bank's view). The contract, the catalogue and runner, the risk-free
rate, the event clock, recorded cascades, signed movements, the Bank
tab and policy as code shipped (gobank-products v0.6.0 to v0.9.0,
gobank v0.29.0 to v0.37.0). Out of scope for the epic: a product
designer, products as data, bulk migration between versions, notice
periods, corrections as adjustment postings, negotiated per-account
rates.

- #68 The bank as a top-level module (ADR-0007 as amended
  2026-10-13) — `modelbank` leaves gobank for a module of its own, the
  Isle of Alba Bank, that imports gobank (the capabilities) and
  gobank-products (the contract) and adds its definition, policy and
  wiring, `cmd/demo` going with it as the wiring is; gobank's own tests
  build against test definitions of gobank's own under `bank/`, so no
  module depends back on a bank; a documentation structure for a bank
  definition (`modelbank/README.md` is the seed). Pre-1.0 the interface
  may still be restructured; from 1.0 normal Go module rules hold
- #69 A product of the bank's own — one product version moves
  from gobank-products into the bank module, the contract and the
  engine staying in the library, proving a bank defines its own
  products
- #70 The remaining events — pre-posting (term lock, ISA
  allowance, overdraft limit as rules the payments path asks), start-up,
  manual commands from the console, change of version per account from
  the staff account page. Done-when of the stage: an account moved from
  easy-access v1 to v2 on preprod, its v1 cycle closed by postings that
  record v1
- #71 History of parameters and products — the
  parameter page gains every setting decided for it in order (value,
  effective, decided, by, and the root event that caused it, from the
  events record of ADR-0011); a product page gains its lineage, not only
  its present: the versions it has had (`easyaccess` v1, v2), when each
  was adopted and withdrawn and on whose authority, the accounts still
  on each, and the settings each took, so a reader sees how the product
  came to be what it is. Open: whether lineage is a record (a version
  names its predecessor, written at adoption) or the family name alone;
  whether withdrawal is itself an event in `bank/events`

## Users and access

A `users` component (own tables, ADR-0001) in the bank's database: a
user is a login name, a password hash and the roles it holds, and names
the customer it is when it is one. Staff and customers are one table
told apart by role. Chosen (2026-10-09) over an identity server: the
tab needs its login in-process, and `OpenCustomer` creating a login is a
call in the same transaction; the later split, if a second bank or SSO
earns it, is Ory Kratos, and because the component sits behind its API
that split is a move, not a redesign. The component and the staff login
shipped (v0.28.0); the API token for the console (v0.37.0). Out of
scope: passkeys and WebAuthn (the mobile epic), MFA, a password-reset
journey, a separate identity service, the standalone RBAC module (a
second bank). Design facts that keep passkeys a later addition rather
than a rework: a user is representable without a password (the hash is
nullable; passkeys become a `user_passkeys` table of credential ID,
public key, sign count, transports, AAGUID, backup flags and RP ID); the
user's identity is its UUID v4 key, which is the WebAuthn user handle,
never the login name; `core.Authenticator` stays password-shaped and a
passkey login is a begin/finish ceremony behind an interface of its own
with the challenge held in the stored session; the session records how
it was authenticated and whether the user was verified, which step-up
asks; the relying-party ID is a domain per environment that
gobank-deploy sets beside the app password. Server library when they
come: `github.com/go-webauthn/webauthn`; browser side
`navigator.credentials` on the BFF login screen; app side Corbado's
Flutter `passkeys` package with the `.well-known` association files
served by the BFF. Settled (2026-10-09): argon2id at the OWASP minimum
(19 MiB, t=2, p=1), measured at 15 ms native and 52 ms in the tab
against bcrypt's 43 ms and 52 ms; a hash costs more than a whole
customer open, so the generator hashes the deployment password once per
process and gives every generated customer that same salt and hash.
Open: whether permissions become rows (role, action) or stay the code
table they are today.

- #72 Customers as users — `OpenCustomer` creates the
  customer's user; `core.Authenticator` reads the component; the
  generator opens its customers with the deployment's password so the
  sense-check logs in as today, and `GOBANK_APP_PASSWORD` is no longer
  read at login
- #73 Permissions with the users — `Role`, `Can` and the
  action list move out of `bff/staff` into the component, read through
  `core`; staff pages, PII gating and the DB explorer's `Authoriser`
  ask it, closing the gap where every role browses every table.
  Done-when of the stage
- #136 Staff web icon: a staff held up by a web — one SVG, no text,
  a vertical walnut staff as the heavier element with a silver web spun
  between it and the frame's corner (straight radials, circular arcs),
  served through the staff web's own `/favicon.svg` handler so it tells
  the staff tab from the customer app's at a glance; the brief with
  the three directions and the palette is on the issue

## Speed

The demo is slow to use. Found by hand on 2026-10-08 at the Hetzner
scale: savings and lending 8 s (a movement sum per request), customers
9 s (not attributed), P&L and balance sheet never return (the interest
reading is slower than the day that invalidates it), the explorer 50 s;
background readings (v0.20.0) stopped pages queuing behind one another
but not the reads themselves. Three suspects, told apart before anything
is moved: a read slow in the database (a sum over every account, a
missing index), a read serialised in-process (`b.mu`, simMu, one
`sql.DB`) before any query is sent, and the simulation saturating the
box the database shares. The database on its own server is the
hypothesis to test, not the plan. Done when at the Hetzner scale every
staff page is under 1 s at p95 at concurrency 4, the P&L and balance
sheet return, and the rows are in `benchmark.md`.

- #74 Measure — every route records last, p50, p95 and max on
  `/about/runtime` and in `/about.json`, and every named read behind a
  page (the book, the interest, the product books, a customer page, the
  explorer's counts) records its duration and whether it was served from
  a reading (age, TTL) or live; a small Go loader in the repo hits the
  route list at concurrency 1, 4 and 16 and reports throughput and p95
  per route, so slow-but-parallel is told from serialised (more
  Postgres only helps the first); a `task bench:reads` runs the named
  reads against a database seeded at 10k, 100k and 1M customers and
  writes one table into `benchmark.md`
- #75 Reads move to the GL
  ([ADR-0005](adr/0005-general-ledger.md)) — the designed fix for sums
  over every account, taken after measure has measured the reads it
  replaces: the book, accrued interest, P&L and balance sheet, the
  history series and lending headroom, each labelled with its day; the
  snapshot taken at the close; the BoE reserve moves to the GL; customer
  reads stay on the sub-ledger. The GL opened and posted in shadow
  shipped in v0.26.0; this is the done-when of ADR-0005's stage. Out of
  scope: retiring old sub-ledger positions (data governance), which
  this makes possible
- #76 The known reads — the product books from the book
  reading (branch `products-from-the-book`), the customers page's
  queries, the explorer's counts, each with its before and after row
- #77 The database on its own server — gobank-deploy gives an
  environment a second server running Postgres on the private network
  and the demo's `GOBANK_PG_DSN` points at it; kept if measure's numbers
  say the shared box was the bottleneck, and in any case the first step
  of the many-process topology (blue-green deployment)

## Mobile apps

Thin iOS and Android apps with as little on the device as possible. All
banking logic stays on the server; the client renders, navigates, and
holds the keys. The BFF is hardened from the start: apart from login,
health and the public login screen it answers nothing without a session.
Built so far: the screen layer (`screen/`: schema v1, JSON and HTML
renderers), the hardened BFF (`bff/`, `cmd/bff`: sessions, credential
login, rate limiting, audit log, authenticated screen endpoints only;
runs against `bff/stubbank` standalone and inside the demo at `/v1/`),
the Flutter shell (`app/`: login, screen renderer, navigation; session
token in memory until the native plugin exists) and the customer web
client as the BFF's HTML rendering (v0.21.0). WASM stays the simulator:
the browser model bank runs the whole bank in-page with no server. Done
when you can open the app on a phone, log in with a biometric-bound
passkey, see live balances served by the BFF, and make a deposit or
withdrawal signed with the device key; the browser app runs against the
same BFF, and nothing in either client decides anything.

- #78 Native security plugin — biometric-bound keys (Secure
  Enclave, StrongBox), passkey registration and login, App Attest and
  Play Integrity token fetching, certificate pinning, screenshot blocking
  and app-switcher blanking, jailbreak, root and overlay detection; the
  BFF checks attestation before issuing tokens. Swift and Kotlin written
  by us rather than assembled from pub.dev packages: this is the code a
  later native rewrite would need, so it carries over unchanged
- #79 Device-bound signing — request signing for
  transactions, step-up authentication (PSD2 SCA)
- #80 Passkeys on the web client — WebAuthn in the browser on
  the BFF's login screen, the customer web client being the HTML
  rendering of the same endpoints (shipped in One BFF)
- #81 App-shielding SDK evaluation — Promon, Guardsquare,
  Appdome, Zimperium; chosen and integrated before any external pilot
- #82 Native checkpoint — after the first external
  pilot, a written list of what Flutter cannot do; move to SwiftUI and
  Jetpack Compose only if the list is non-empty
- #83 Document rendering on the device (scaling option)
  — where a document is rendered is a cost question, not a feature.
  Terms and conditions are a function of the product version alone: the
  text is a product parameter (a text view on the parameters pages), so
  the rendered document is cached per version and costs O(p) in product
  versions, nothing per customer. Only per-customer documents scale with
  the book: statements (json2pdf) and movement diagrams (pta2svg). For
  those the BFF serves the data and the client renders. The web client
  loads the TinyGo WASM builds both renderers already ship; the Flutter
  app cannot run them (no JavaScript host on Android or iOS), so on the
  device the same pure-Go renderers are bound natively with gomobile (an
  .aar and an .xcframework behind a platform channel), the mobile
  counterpart of WASM in the browser. Until then the BFF renders
  everything (the thin-client rule), which is enough for the demo; taken
  when a perf run (speed) shows document rendering on the server's bill.
  Out of scope: a WASM runtime inside the app (both renderers would need
  WASI rebuilds and an FFI layer), a WebView onto a BFF page. Open: the
  statement rendered on the device from JSON versus a PDF from the BFF
  opened in the device's viewer; whether pta2svg draws arrowheads without
  SVG markers (flutter_svg has none) or the diagram travels as PNG

## Payment rails

Done when every payment records the rail it travelled, each rail is a
scheme adapter with its own timing and outages, and one real outbound
payment has left the demo bank over a live sandbox API.

- #84 Multiple payment rails, and one real rail — every payment
  records the rail it travelled (internal book transfer, FPS, Bacs,
  CHAPS) and each rail is a scheme adapter behind one interface with its
  own settlement timing, cut-offs, limits and outage behaviour; a routing
  rule picks the rail from amount and urgency. The payments component
  stays the one bank-level view (one lifecycle, one list, filterable by
  rail) and gains a per-rail view: volumes, queue depth, settlement
  position and scheme status. The mock-fps and FPS stand-in items are
  the FPS rail's, and the adapter boundary is what the unbundling
  unbundles. Out of scope: real scheme messaging (ISO 20022), cards.
  Open: rail as a column on the one `payments` table (the coherent view
  comes free) versus a table per rail with a bank-level union view (each
  rail's data differs — Bacs has a three-day cycle, CHAPS is same-day,
  FPS instant). The rails become real with a real rail: one outbound
  payment from the demo bank to a real account through a live bank API,
  credentials per environment via gobank-deploy, sandbox first. Out of
  scope: inbound, bulk, cards. Open: Starling (developer sandbox,
  personal access token, plain REST) versus Barclays (Open Banking PIS,
  needs TPP registration and eIDAS certificates, likely out of reach for
  a hobby project) — Starling sandbox first
- #85 Payee management — a customer's saved payees (name, sort
  code and account number, reference), created, edited and deleted in
  the app and the customer web and offered by send money; a component
  with its own table (ADR-0001) behind session-only BFF endpoints.
  Builds on device-bound signing. Out of scope: international payees, a
  real Confirmation of Payee call (stubbed behind an interface). Open:
  new-payee limits and cooling-off
- #86 mock-fps integration — compiled-in Faster Payments
  simulator for payment processing
- #87 FPS stand-in mode — simulate FPS outages, payment
  queuing, and recovery
- #88 Unbundle mock-fps as a separate service (HTTP API)

## Workflows and lifecycles

Done when long-running processes (onboarding, a lending proposal,
dormancy) run as durable workflow instances on the bank clock, with a
staff queue for the human steps.

- #89 Workflow engine — a `workflow` component (own
  tables, ADR-0001): long-running processes made of steps, timers and
  human decisions, durable so an instance resumes after a restart, with
  timers on the bank clock (`core.Clock`) so a six-month wait runs inside
  a simulated afternoon. First consumers: KYC journeys and sign-up,
  lending proposals, dormancy, licence promotion (Isle of Alba). The
  many-process topology's single workflow runner becomes this engine's
  runner. WASM runs it in memory. Out of scope: a visual designer, BPMN,
  retries with backoff beyond "try again next day". Open: definitions as
  Go code (steps are named functions, the instance row records the step
  it is at) versus a declarative definition table; whether the
  start-of-day pass itself becomes an instance
- #90 KYC process — onboarding as a lifecycle (pending → verified →
  rejected → in review): identity capture, document and liveness checks
  as pluggable verifiers with a stub provider in the demo, running as a
  journey on the workflow engine, a risk rating, and the record of how
  and when a customer was verified and of later name and address changes
  (#25); an unverified customer transacts only up to a cap. Out of
  scope: a real IDV provider (same interface as the stub), AML
  transaction monitoring (belongs with the risk register). Open: does
  the customer generator produce KYC outcomes, so the staff view has a
  queue to work
- #91 Lending gateway — a loan is a proposal, not a
  disbursement: opening a customer with a loan, or a later request,
  creates a lending proposal that a workflow decides — refused at the
  deposit-taker tier, approved within headroom at later tiers, with a
  staff approval step above a size. Replaces the open-time trim in
  `planCustomerLocked`; the proposal and its decision are records, so
  the staff view shows a queue and the refusals. The generator keeps
  asking for loans at tier one so the refusals are visible. Out of
  scope: credit scoring, affordability, repayment schedules (products)
- #92 Working savings accounts — full lifecycle: open,
  deposit, withdraw, accrue interest, close
- #93 Dormancy and the account-number lifecycle — a savings
  account with no customer movements for six months (interest credits
  do not count) becomes dormant; dormant with a nil balance it is closed
  automatically, with a balance it stays dormant and is reported.
  Closing returns the account number to quarantine for a period before
  it can be reissued, so numbers have a lifecycle: issued → open →
  closed → quarantined → free. The sweep is a workflow timed from the
  last customer movement. Out of scope: the Dormant Assets Scheme (15
  years), a reactivation journey. Open: the quarantine length; whether
  the number register is its own component or part of customers. Real
  UK banks do not reuse numbers; the model does so the register stays
  finite

## Customers

Done when a customer is a realistic model (confidence, balances,
switching) and names and addresses sit behind an access-controlled
wall.

- #94 Customer model — confidence score, maximum aggregate
  balance, realistic switching behaviour
- #95 Named customers behind the customer wall — PII-like
  data gated behind access control

## Finance and regulatory reporting

Done when a period closes to stored reports that stay as reported, the
returns (BBSI, AEOI, SCV) come from them, and the treasury and risk
views read the same ledger.

- #96 Period accounting reports, produced and stored — a
  reporting period (month, year, tax year for BBSI) is a thing with a
  close: at close the P&L, balance sheet and the period's returns (BBSI
  at tax year end) are produced from the ledger as at that knowledge
  time and stored as records, so what was reported stays what was
  reported if a late posting changes the books; real time is a live view
  of the open period, never stored. The reports page lists periods and
  their stored reports; BBSI moves from a live build to the stored
  year-end report. Needs go-luca `knowledge_time` (shipped, go-luca
  v0.3.0). Open: a posting after close — restate, or carry into the next
  period with a note
- #97 AEOI return prototype — the bank produces the Automatic
  Exchange of Information return (CRS, and FATCA for US persons) that a
  UK reporting financial institution files with HMRC each year: for
  every reportable account holder, the account's year-end balance and
  the interest paid in the year, keyed to the holder's tax residence and
  TIN. The return is a stored period report like the BBSI, produced from
  the ledger as at the year end and kept as reported. The customer model
  grows to carry what the return needs and KYC collects today in words
  only: tax residences, TINs, date and place of birth, a
  self-certification on record, and for entities the controlling
  persons; the generator gives a share of customers a foreign residence
  so the return has rows. Out of scope: the XML schema and HMRC's
  gateway (the prototype produces the rows and a CSV), due-diligence
  thresholds, undocumented-account handling. Open: whether residence and
  TIN live with the PII behind the customer wall or as a
  reportable-person record of their own; whether a reportable account is
  marked at open or found at year end
- #98 Treasury: cash at BoE and net interest margin — the
  treasury view gains the BoE balance over time (the reserve account's
  daily position from the ledger) and, for a reporting period, the net
  interest margin: interest earned (BoE reserves, gilts, loans) less
  interest paid (savings), as money and as a rate over average earning
  assets. Builds on period accounting reports
- #99 Key and emerging risk register — a register component
  (own tables, ADR-0001) listing the bank's key risks (credit,
  liquidity, interest-rate risk in the banking book, operational,
  conduct) with owner, inherent and residual rating and controls; each
  key risk carries KRIs computed from the simulation through contract
  views, with amber/red thresholds, so a simulated run moves the
  register. Emerging risks (horizon scanning: cyber, climate,
  payment-scheme change) are recorded without KRIs until one is
  measurable. Out of scope: capital modelling (ICAAP/ILAAP numbers).
  Open: which KRIs the simulation can already support versus which need
  new metrics (e.g. liquidity coverage needs a treasury cash view)
- #100 SCV regulatory report at scale (100K+ customers)

## Performance and observability

The demo tells us what it is doing through the runtime page and the day
length; the cost of the design (events and the daily pass taking turns
per account, projections written through the day) has to be measured,
not inferred. Telemetry is its own epic so that every later story ships
with its numbers. The performance run on Hetzner, small and large,
shipped (v0.12.0, gobank-deploy `perf`). Done when a Hetzner day can be
read as a timeline of its phases and lock waits in the viewer, the
drill records its downtime from the same source, and a slower release
fails before it ships.

- #101 Observability with OpenTelemetry — the demo emits metrics and
  traces through the OpenTelemetry SDK, OTLP to a collector per
  environment that gobank-deploy runs alongside the demo, starting with
  what the runtime page shows today: day phase and duration, movements
  per second, database connections, memory. Out of scope: a logs
  pipeline, alerting. Open: OTLP push versus a Prometheus pull endpoint;
  where the collector and viewer live on a one-server environment
- #102 Account locks in use — once the above is in place:
  how many of the per-account lock stripes are held, how often a holder
  waits and for how long, with the equity account's stripe named, so
  the cost of an event and the daily pass taking turns on an account
  (ADR-0002 stage 3) is measured rather than inferred from the day
  length
- #103 Load tests in CI — the `bench:reads` thresholds of the
  pipeline gates extended to a load profile (concurrent customers on the
  BFF, the generator at the small scale) run per release on the Hetzner
  perf box, its row in `benchmark.md`. Out of scope: a load test per
  commit
- #135 RapiDoc viewer for the API in the staff web — the one
  web component (go:embed, about 0.8 MB, no CDN so it works in the tab)
  under About beside the server-rendered API page, over the served
  `/v1/openapi.json`, Bulma colours passed as attributes; gives try-it
  with bearer auth for the customer routes and the operator token for
  the console, which the server-rendered page cannot. Out of scope:
  Swagger UI, Redoc and Scalar (framework applications shipped as
  blobs, the lofigui ceiling). Open: the weight it adds to the WASM
  download, and whether it replaces the server-rendered page or sits
  beside it

## Resilience

The bank is killed, restarted and lied to on purpose, and the books
must come out the same. The resume and the drill (v0.6.0, v0.7.0)
proved one restart each; this epic makes the restart a test that runs.
Done when a fuzzer can stop the bank at any point of any day and replay
it to the same books, every command posts once however often it is
retried, and a day's invariants are checked on every run.

- #104 Restart the day at any point — the start-of-day
  pass and the event cascade killed and resumed at random points, the
  books compared with an uninterrupted run of the same seed; the
  comparison is the test, run in `task check` at the small scale and by
  the drill on preprod
- #105 Idempotent commands — every command carries a key so a
  retry after a lost reply posts once; the BFF, the generator and the
  rails retry on it, fuzzed by replaying and reordering requests. Open:
  the key as a column on the event versus a separate dedupe table
- #106 Fuzzing the inputs and the day — Go's native fuzzing on the
  parsers and the command boundary (amounts, dates, references), and a
  chaos mode in the simulation (a dropped rail, a slow database, clock
  jumps) with the day's invariants asserted at close: the trial balance,
  GL equal to sub-ledger, positions equal to movements

## Delivery pipeline

Every story ends with a release deployed and drilled on preprod before
prod is promoted. The release is prepared on the branch by `tp release`
and tagged on main (v0.7.0); ADRs are published whole (v0.36.0); the
console takes an API token so gobank-deploy may drive it (v0.37.0). The
deploy and the drill are still run by hand after the merge; the
pipeline is the time between a merge and a sense-checked deployment.
Done when a merge to main tags, deploys to preprod, drills and promotes
prod without a hand on the keyboard, and the docs site follows.

- #107 Post-merge automation — tag, deploy to preprod, the
  upgrade drill and the prod promotion run from the merge, in
  gobank-deploy or Woodpecker CI, so testing overlaps writing the next
  story. Open: gobank-deploy's own runner versus Woodpecker on the CI
  server
- #108 Performance gate in the pipeline — the speed won by the speed
  epic is held by the pipeline: `task check` (which `tp release` runs)
  runs `bench:reads` at the small scale against the thresholds speed
  settled, so a slower read fails the release; the Hetzner perf run
  stays a per-release command (it costs a server) and its row joins the
  release notes. Out of scope: a perf run per commit, load tests in CI
  (performance and observability)

## Blue-green deployment

ADR-0003 accepted downtime per upgrade until the many-process topology;
this epic ends it. ADR-0002 stages 7 and 8 live here. Done when a
release reaches prod with no downtime: two versions run side by side,
traffic moves between them and the schema migrates under both.

- #109 Feature flag component — a `flags` component (own table,
  ADR-0001) holding named on/off switches, read through a core query
  and flipped from a staff page; WASM keeps them in memory. Needed where
  an old and a new path run side by side (a new payment rail) and the
  demo must switch between them per deployment without a rebuild. A
  flag is short-lived: it is deleted once both paths converge. Out of
  scope: percentage rollouts, per-customer targeting, external flag
  services. Open: startup-only (read once from config; simple, no
  table, every process agrees) versus live (stored, flippable mid-run;
  lets the demo show a cutover or a kill switch without a restart)
- #110 Read/write split — separate read and write handles
  (ADR-0002 stage 7)
- #111 Many processes — several BFFs, a generator and one
  workflow runner; deploys go blue-green (deployment level 3), ending
  the downtime accepted since stage 2 (ADR-0002 stage 8)
- #112 Blue-green schema migrations — zero-downtime data
  swaps

## Kubernetes and AlloyDB

The many-process topology (several BFFs, a generator, one workflow
runner, primary and replica) deployed to a Kubernetes cluster with a
real database. Done when the same code runs in Kubernetes with AlloyDB,
mock-fps is a separate service, and every GUI action has an API for
scenario tests.

- #113 Deploy to a Kubernetes cluster
- #114 AlloyDB (Postgres-compatible) backend
- #115 Full HTTP API for all GUI actions — automation and
  scenario testing

## Data governance

What the bank keeps, for how long, and who may see it is policy, held
per bank in its definition module (ADR-0007), not an accident of what
the tables contain. Done when every table has a stated lifetime, the
stores are kept within it by routine maintenance, and an erasure
request is answered from policy.

- #116 Data lifetimes as policy — retention per record kind (the
  six-year regulatory minimum for accounts and movements, shorter for
  sessions, audit and telemetry), what survives an account's closure,
  and an erasure request that removes what policy allows and records
  what it must keep. The lifetimes are definition data (ADR-0007).
  Open: whether the ledger's history is immutable and only the PII
  beside it is erasable
- #117 Archive and maintenance — closed accounts and old movements
  moved out of the hot tables on their lifetime (the movements indexes
  at 44M rows a year are the cost), partitioning by day, routine vacuum
  and index maintenance in or beside the start-of-day pass, with the
  drill proving an archive restores. Open: an archive store (object
  storage, luca files as the single-account export writes them) versus
  cold partitions in the same database
- #118 DB explorer on the pluggable go-dbexplorer — fix FK
  filter links (route through `Explorer.Render`); then a `Catalog`
  adapter over `components.go`. The `Authoriser` is the permissions
  item under users and access

## Bank migration

Another bank moves onto this architecture by importing its book,
running the two in parallel, and switching accounts across one at a
time over a payment rail both sides speak, so no customer sees a
cutover day. Done when a book imported from luca files runs beside its
source and accounts switch over one by one with the customer's balance
and history intact.

- #119 Import and export via luca files — load and save simulation
  state as plain-text accounting files
- #120 Export a single savings account — extract one
  account's history as a luca file
- #121 Import another bank's book and run in parallel — a
  second bank's accounts and history loaded from luca files into a
  definition module of its own (ADR-0007, a second bank), both banks
  running on one deployment with their own ledgers and GL; the
  simulation drives both. Out of scope: a live feed from the source bank
- #122 Switch accounts over one at a time — an account moves
  from the source bank to the target on a day of its own: balance and
  history transferred, the source closed with a redirect, payments in
  flight re-routed over the common rail (payment rails), the customer's
  app pointed at the new BFF; the staff view shows the migration's
  progress and the stragglers. Open: the account number kept (the
  switch-service model) versus a new number with forwarding

## A second bank

ADR-0007 decided that a bank is a definition module over gobank's
capabilities; this epic proves it by running a second one. Done when
two definition modules run on one deployment, each with its own name,
catalogue, policy and look, and nothing in gobank names either.

- #123 Second definition module — a second bank as its
  own Go module (ADR-0007): name, catalogue, blurb, policy, base-rate
  series; the demo opened with either, and gobank-deploy deploying each
  as an environment. Out of scope: two banks in one process
- #124 Per-bank presentation — what the BFF and the
  apps show that is the bank's: name, colours and icons, the catalogue
  order, the terms text; the screen layer takes them from the
  definition, and the Flutter shell and the web client render whatever
  comes
- #125 Standalone RBAC module — extract the users component's
  roles and permissions into its own repo (not gobank-db) once a second
  bank is the second consumer; until then it lives in the bank's
  database

## Isle of Alba

The model bank operates somewhere. The Isle of Alba is that somewhere:
a fictional jurisdiction with its own regulator, licence tiers, returns
and calendar, held as data the way ADR-0007 holds the bank, so the
rules a bank runs under are a thing the model can vary. Done when the
regime a bank operates under is data: its licence tiers, its returns
and their calendar, its currency and bank holidays, and the licence
trajectory runs against them.

- #126 Bank licence trajectory — the bank is an institution that
  grows through tiers: a deposit-taker (savings products only, no
  lending, no regulatory capital), a small bank once it has raised
  investment or retained 5M, a regulated bank at 25M with capital rules.
  A tier is a set of permissions the rest of the bank reads as rules:
  lending headroom is nil at tier one, the reserve-ratio rule at tier
  two, capital-based at tier three. A `licence` component (own table)
  holds the current tier and the history of transitions — date, trigger
  (investment event, earnings threshold crossed), the figures at the
  time, who approved — and a staff page shows the trajectory: where the
  bank is, what the next step needs, how far along. Promotion is a
  workflow with a staff step; the simulation can inject an investment
  event. Thresholds are data, not code. Out of scope: real PRA/FCA
  regimes (mobilisation, Part 4A), ICAAP/ILAAP numbers (the risk
  register keeps those out too). Open: the thresholds (5M and 25M are
  the model's own; UK mobilisation is a £50k deposit cap) and whether a
  tier can be lost
- #127 The regime as data — a jurisdiction definition beside the
  bank's (ADR-0007): regulator, licence tiers and thresholds (the
  licence trajectory reads them), the returns calendar (BBSI, AEOI, SCV
  from finance and regulatory reporting), currency, bank holidays for
  the business-day calendar (go-luca). Open: whether a jurisdiction is a
  module like a bank or a table the bank's definition fills

## Local process on the device

Done when a customer's screens render on the phone from a local copy of
their own data, ten times faster than a round trip, with the same BFF
and screen layer and writes still going to the server.

- #128 The read side on the device — the core's read side
  bound with gomobile (document rendering on the device is the first
  binding): the customer's contracts, ledger projection and statements
  held on the device, synced from the server by events, the screen layer
  running locally so a screen is built without a request; the session,
  every command and the first sync go to the server. Same architecture,
  the BFF's read half moved to the phone. Out of scope: offline writes,
  other customers' data. Open: sync by polling the BFF versus a push
  channel; what the device keeps after logout (data lifetimes)

## Distributed scale

The many-process topology across regions on distributed SQL. Done when
the bank runs across regions on CockroachDB with zero-downtime
migrations.

- #129 CockroachDB backend
- #130 Multi-region deployment
