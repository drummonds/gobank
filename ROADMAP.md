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
  the in-memory `bff/stubbank` until the core is extracted
- **Flutter shell** — `app/`: login, screen renderer, navigation; session token
  held in memory until the native plugin exists

### To Do (in order)

1. **Extract the banking core** — move `DemoState` and the domain types out of
   `cmd/demo` (package main) into an importable package and implement
   `bff.Bank` on it, so `cmd/bff` serves real data
2. **Demo phone frame onto the screen layer** — `cmd/demo/bankapp_render.go`
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

Same banking core, deployed as services in a Kubernetes cluster with a real database.

- Deploy to Kubernetes cluster
- AlloyDB (Postgres-compatible) backend
- Unbundle mock-fps as a separate service (HTTP API)
- Full HTTP API for all GUI actions (automation/scenario testing)
- SCV regulatory report at scale (100K+ customers)

## Phase 4 — CockroachDB + Scale

Prove the core works across regions with distributed SQL.

- CockroachDB backend
- Multi-region deployment
- Blue-green schema migrations (zero-downtime data swaps)

## What Success Looks Like

**Phase 1 complete** means: you can open the WASM demo, run a simulation, see accurate financials (P&L, balance sheet), export any account as a luca file, trigger an FPS outage, and watch the bank handle it. All actions available via API for automated testing.

**Phase 2 complete** means: you can open the app on a phone, log in with a biometric-bound passkey, see live balances served by the BFF, and make a deposit or withdrawal signed with the device key. The browser app runs against the same BFF. Nothing in either client decides anything.

**Phase 3 complete** means: the same code runs in Kubernetes with AlloyDB, mock-fps is a separate service, and SCV reports generate correctly at scale.

**Phase 4 complete** means: the bank runs across regions on CockroachDB with zero-downtime migrations.
