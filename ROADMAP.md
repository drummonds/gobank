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

### To Do

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
- **Multiple payment rails** — every payment records the rail it travelled
  (internal book transfer, FPS, Bacs, CHAPS) and each rail is a scheme
  adapter behind one interface with its own settlement timing, cut-offs,
  limits and outage behaviour; a routing rule picks the rail from amount and
  urgency. The payments component stays the one bank-level view (one
  lifecycle, one list, filterable by rail) and gains a per-rail view:
  volumes, queue depth, settlement position and scheme status. The existing
  mock-fps and FPS stand-in items below become the FPS rail's stories, and
  the adapter boundary is what Phase 3 unbundles. Out of scope: real scheme
  messaging (ISO 20022), cards. Open: rail as a column on the one
  `payments` table (the coherent view comes free) versus a table per rail
  with a bank-level union view (each rail's data differs — Bacs has a
  three-day cycle, CHAPS is same-day, FPS instant)
- **Daily accrual cost scales with every account** — each account gets a
  ledger posting every day (`collectAccrualMovements`: a "Daily interest
  accrual" movement whenever a new whole penny has accrued, plus a reversal
  per account at month end), and `persistAccrualState` upserts one
  `accrual_state` row per account per day, one statement each. On the
  Hetzner demo (2026-10-01: 300k customers, ~477k postings a day at
  ~1.2k/s) a simulated day takes over ten minutes, and the progress label
  stays on "bookkeeping" through the off-lock write, so the run looks hung.
  Options: post accruals in aggregate (one movement per family per day,
  per-account detail kept in `accrual_state`) or per account only at
  application; batch the upserts (multi-row `VALUES` or `COPY`); label the
  persist phase. Open: whether the per-account luca export needs
  per-account daily postings in the ledger
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
  `components.go` and an `Authoriser` wired to `Role`, closing the gap where
  every role can browse every table
- **Working savings accounts** — full lifecycle: open, deposit, withdraw, accrue interest, close
- **Import/export via luca files** — load and save simulation state as plain-text accounting files
- **Export single savings account** — extract one account's history as a luca file
- **Accurate P&L** — end-of-day and for arbitrary periods (monthly, quarterly, annual)
- **Balance sheet** — end-of-day and for arbitrary periods
- **Customer model** — confidence score, maximum aggregate balance, realistic switching behaviour
- **Named customers behind customer wall** — PII-like data gated behind access control
- **mock-fps integration** — compiled-in Faster Payments simulator for payment processing
- **FPS stand-in mode** — simulate FPS outages, payment queuing, and recovery

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

1. **Transition to the target architecture** — [ADR-0002](adr/0002-target-architecture.md):
   one BFF for app and web, a core library of components over a database
   that holds every fact, accruals pipelined one day ahead by a start-of-day
   workflow, and a simulation that differs from production only in event
   sources and the clock. Nine stages, each leaving the demo running in
   WASM, on a server and on Hetzner. Every story ends with a release
   deployed to the Hetzner demo and sense-checked there before the next
   starts:
   1. [x] (v0.4.0) Seams — core commands/queries as interfaces; `DemoState` adapts to
      them; the BFF runs in the demo process and the app shows real data.
      Stories:
      - [x] (v0.3.54) (a) customer contracts in `core`, contract suite,
        demo adapter, BFF at `/v1/`
      - [x] (b) gobank-deploy sets `GOBANK_APP_PASSWORD` per environment
        and shows it (its story 1g, gobank-deploy v0.3.0)
      - [x] (v0.4.0) (c) Android debug build — `app/android` and `app/ios`
        generated and committed, cleartext traffic allowed in the debug
        manifest only — then (a) sense-checked from a phone against
        Hetzner (2026-10-03)
      - [x] (v0.4.0) (d) staff queries — `core.StaffQueries`, one
        interface per component (book, customer register, payments,
        products, treasury), every staff page rendering from them
      - [x] (v0.4.0) (e) commands — `core.Commands`: `Transfer` (the
        payments generator calls it) and `BuyGilt` (the treasury page
        calls it). Opening a customer stays inside the generator until
        stage 4 splits generators from the bank
   2. Stored truth — transactions as a ledger projection, stored chart
      snapshots, sessions in the database. From here every story is an
      upgrade of the running Hetzner demo, downtime accepted and recorded
      until stage 8 ([ADR-0003](adr/0003-upgrade-in-place-until-stage-8.md);
      deployment level 2 on the manual's
      [maturity ladder](https://man.bytestone.uk/maturity.html)), and
      development moves to a branch per story merged by pull request on
      the Forgejo (code level 1). Stories:
      - [x] (v0.5.0) (a) simulated day length — a setting (`GOBANK_DAY_LENGTH`,
        zero means flat out; shown and set on the simulation page) so a
        day can take two hours and an upgrade lands mid-day
      - [x] (v0.6.0) (b) resume — the demo stops dropping its tables at start and
        rebuilds its state from the database, with a schema version table
        and versioned migrations replacing the ad hoc `ALTER TABLE`. Also,
        from (a)'s sense-check: the settings form is static under the page
        poll (an HTMX status section polls, the form does not, as the
        dashboard already does) so a value being typed is not wiped before
        Save; and the dashboard's day tile counts down the seconds to the
        end of the day
      - [x] (v0.7.0) (c) upgrade drill — the demo keeps a restart
        record (`restarts`, an expand-only migration of the simulation
        component): each process start against the stop before it, with
        the downtime and the run's day and customers on both sides, shown
        on the settings page; `upgrade-drill.md` is the drill — redeploy
        N to N+1 on Hetzner mid-day, read the downtime off the record,
        roll back to N from the release store and forward again, confirm
        nothing is lost — and every later story's acceptance. Needs
        gobank-deploy story 1i (stop timeout, fetch a named tag)
      - [x] (unreleased) about/runtime shows the schema version per
        component, as `/about.json` already does
      - [x] (unreleased) (d) transactions as a ledger projection replacing `txLog`
      - [ ] (e) chart histories as stored daily snapshots
      - [ ] (f) sessions in the database
   3. Pipelined accruals — start-of-day workflow writes next-day
      projections; interest application is product code inside it
   4. Events and clock — bank and simulation split; generators and an
      injected clock
   5. Core into packages — one component at a time
   6. One BFF — staff UI and customer web through the BFF (absorbs item 2)
   7. Read/write split — separate read and write handles
   8. Many processes — several BFFs, a generator and one workflow runner;
      deploys go blue-green (deployment level 3), ending the downtime
      accepted since stage 2
   9. Simulation becomes tests
2. **Demo phone frame onto the screen layer** (ADR-0002 stage 6) — `cmd/demo/bankapp_render.go`
   becomes a caller of `screen.HTML`, so browser and app show identical screens
   from one source, and the open `/api/customer/` endpoints are retired
3. **Native security plugin** — biometric-bound keys (Secure Enclave, StrongBox),
   passkey registration and login, App Attest and Play Integrity token fetching,
   certificate pinning, screenshot blocking and app-switcher blanking, jailbreak,
   root and overlay detection; the BFF checks attestation before issuing tokens
4. **Device-bound signing** — request signing for transactions, step-up
   authentication (PSD2 SCA)
5. **Web client on the BFF** — the HTML rendering of the same endpoints becomes
   the customer web client; passkeys via WebAuthn in the browser
6. **App-shielding SDK evaluation** — Promon, Guardsquare, Appdome, Zimperium;
   chosen and integrated before any external pilot
7. **Standalone RBAC module** — extract `Role`/`Can` from `cmd/demo` into its
   own repo (not gobank-db) once the BFF is the second consumer; it then
   implements go-dbexplorer's `Authoriser`
8. **Native checkpoint** — after the first external pilot, a written list of what
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
