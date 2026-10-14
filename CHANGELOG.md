# Changelog

## [Unreleased]

### Added
- Per-route performance (roadmap #142): the running process counts and
  times every route it serves (the staff web and the customer BFF share
  one set of figures): requests, p50 and p95 of the last 1,024, the
  maximum, the most in flight at once and the rate over the last minute.
  About → API shows requests and p95 per route and the route's page a
  Performance block with the capacity it has shown — in flight at the
  peak over p95, as requests a second and as clients acting every ten
  seconds — and `/about.json` carries the rows for a program to read.

## [0.50.0] - 2026-10-14

### Added
- Research note `sizing.md`: data volume, placement and processing for
  300k, 1M and 10M retail customers, worked from the ledger's row shapes
  and the Hetzner pass rates; linked from the docs site.
- Roadmap: a Milestones epic at the end of the file, each milestone
  with what sits before and after it; #151 (capacity of one synchronous
  system over seven years, the 1M gate) and #152 (real money, separate
  from game money) filed.

## [0.49.0] - 2026-10-14

 - About → API: static index in sections, a page per route linking its source; roadmap #141 and #142 filed

### Fixed
- About → API no longer loses an opened route when the run's poll
  re-renders the page: the index is a static page whose rows link to a
  page per route (`about/api/{method}/{path}`), and the routes fall into
  sections by the function they are registered in, titled from its doc
  comment — the simulation console is one. Each route's page links its
  registration in the source at the build's tag. The docs site page
  carries the same, anchored on one page.

### Roadmap
- API detail pages carry a worked example per route (#141); per-route
  performance on the API detail page (#142).
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

## [0.48.0] - 2026-10-14

 - Customers retype migration drops its contract view first: the v0.43.0 → v0.45.0 drill failed on PostgreSQL's "cannot alter type of a column used by a view" (#66)

### Fixed
- The customers v2 migration (`ledger_account_id` to `UUID`, #66) drops
  `contract_customer_accounts` before the retype. The previous release
  leaves its view in place and PostgreSQL will not retype a column a view
  reads, so the preprod upgrade from v0.43.0 died in `migrate` after the
  products retype had committed; the bank recreates the view at open, as
  it always did. A Postgres-gated test in `bank/customers` reproduces the
  refusal (`GOBANK_PG_DSN`), and bank package tests may now import the
  pgx driver for that purpose.

## [0.47.0] - 2026-10-14

 - Escape from the automatic upgrade drill: SKIP_DRILL=1 tp release fetches but does not drill, and task cloud:drill:cancel stops a running one (#147)

### Added
- `SKIP_DRILL=1 tp release` runs the post-release fetch but skips the
  upgrade drill on preprod, saying so; one release at a time, since the
  variable lives only on that command line (#147).
- `task cloud:drill:cancel` posts the appliance's cancel for preprod
  (`ENV` overrides), stopping a drill already running; the environment is
  left where the drill got to and the demo workflow resumes it.

## [0.46.0] - 2026-10-14

 - Pin go-postgres v0.10.0 and go-luca v0.6.0 in place of the local replaces v0.44.0 and v0.45.0 shipped with

### Fixed
- go.mod and cmd/demo/go.mod pin go-postgres v0.10.0 and go-luca v0.6.0;
  v0.44.0 and v0.45.0 shipped with local `replace` directives to working
  copies, so `go install` of the module failed and neither release could
  be installed or deployed.

## [0.45.0] - 2026-10-14

## [0.44.0] - 2026-10-14

 - UUID columns across the family (#66): ledger ids retyped in customers, products and gl; needs go-postgres v0.9.0 and go-luca v0.6.0

### Changed
- UUID columns across the family (#66): every column that holds a ledger
  id is `UUID`, not `VARCHAR`: `customer_accounts.ledger_account_id`,
  `product_postings.movement_id` and `account_id`,
  `product_account_parameters.account_id`, `gl_journal.movement_id`,
  each by a migration that retypes the column in place with its rows,
  and the ledger's own ids by go-luca v0.6.0, which retypes a text-id
  ledger on open. On PostgreSQL that is 16 bytes against 37 in every id
  and its indexes (the movements indexes are the cost at scale: the
  go-postgres study measured TEXT keys at 1.7x the bytes and 2.8x the
  join at 100k accounts), and the database rejects a non-UUID. The pass
  cursors over accounts start from the nil UUID, since a uuid column
  cannot be compared to an empty string there, and a setting's owner is
  compared to an account id as text. Needs go-postgres v0.9.0, whose
  pglike retypes by rebuilding the table and validates UUID columns:
  tests that wrote a made-up cause such as `root-1` now write a UUID.

### Roadmap
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

## [0.43.0] - 2026-10-13

 - The balance sheet balances: both statements read from the ledger, operating costs and gilts posted, dated with the 31 March year end

### Fixed
- The balance sheet balances. It is read from the ledger (ADR-0008):
  operating costs are posted daily from `Asset:Cash` to the new
  `Expense:Operating` instead of being a counter, a gilt purchase moves
  its face value from cash to the new `Asset:Gilts` in the same
  transaction as the holding, BoE interest is the ledger's income and
  reserves, and the customers' accrued interest — which the products
  hold as positions until applied — is shown on both sides as the P&L
  counts it (accrued interest receivable and payable). Equity is share
  capital plus retained earnings, and so the CET1 capital; the cash
  line no longer hides a negative behind a clamp. A bank from before
  this version adopts the two accounts on open: the costs of the days
  already run and the gilts already held are posted once, so the
  statements go on from where the counters left them.

### Added
- The statements are dated: the balance sheet as at the business day,
  the P&L for the period from the opening day to it, in the financial
  year named by its end — 31 March, kept in the bank's definition
  (`definition.Bank.YearEnd`). The year-to-date cut and the year-end
  close are roadmap #96.

## [0.42.0] - 2026-10-13

## [0.41.0] - 2026-10-13

 - API documentation read off the code (#133): the walker, /v1/openapi.json, About → API and api.html on the docs site

### Added
- Full API documentation (#133), read off the code: `api/walk` loads the
  BFF and staff packages with go/packages and finds every route they
  register — directly or through a helper that passes the pattern on —
  with what each handler reads and answers (status, media, the type
  encoded, parameters, the cookies and headers its wrappers check, the
  role action), the gate in front of the staff mux, and the schemas of
  every body type from the Go declarations and their comments. `api`
  renders it as OpenAPI 3.1, served at `/v1/openapi.json`, and as a
  page: About → API in the staff web and `api.html` on the docs site
  (with `openapi.json` beside it). `go generate ./api` writes the
  document and the surface; the `api` test regenerates and fails when
  the committed files are behind the code, and each server's test holds
  what its mux serves at run time to exactly what the walker found.
  Every route carries a comment in the code, which is its description.
  Replaces the hand-written BFF API research page.

### Changed
- The BFF's error, login and logout bodies are named types
  (`errorBody`, `loginResponse`, `screenBody`), unchanged on the wire.

## [0.40.0] - 2026-10-13

 - The family map: the build's module graph on the docs site with lags drawn; roadmap #135 and #136; diagrams no longer 403 on the docs site

### Added
- The family map (`family.html`): one node per repository of our own
  with the version the demo's build selected and one edge per
  requirement, generated by `cmd/gendocs` from `go mod graph` and
  `go list -m all` in `cmd/demo` (`internal/family`); a requirement for
  an older version than the build selects is drawn as a lag. Replaces
  the hand-drawn Project Hierarchy research page.

### Changed
- `docs:build` sets every rendered SVG to 0644: d2 writes 0600 and the
  rsync deploy preserved it, so the docs site answered 403 for every
  diagram (task-plus #61 fixes the deploy side).
- Roadmap: RapiDoc viewer for the API in the staff web (#135), the
  staff web icon (#136).

## [0.39.0] - 2026-10-13

 - One tp release per story with the drill started from post_release; roadmap item #133 full API documentation

### Changed
- One `tp release` on the story branch now opens the pull request, waits
  up to ten minutes for the merge (`merge_wait`), tags, publishes, has
  the gobank-deploy appliance fetch the binaries and starts the upgrade
  drill on preprod (`task cloud:drill`); promoting prod stays a button
  once the drill has passed.
- The roadmap gains full API documentation (#133) under the transition
  epic; #108 narrows to the performance gate in the pipeline.

## [0.38.0] - 2026-10-13

## [0.37.0] - 2026-10-13

 - Story 1.8.9 policy as code (#68 the bank module deferred), story 9.4 API token for the console, roadmap migrated to tp roadmap

### Added
- Story 1.8.9: policy as code. The contract (gobank-products) gains a
  parameter derived by code, `Declaration.Algorithm`, the hook ADR-0006's
  lag and take become, and `Policy`, a bank's own rules as versioned
  code. The Model Bank's policy, `modelbank/policy/v1`, derives the
  bank's savings and lending rates from the pound's risk-free rate with
  a spread and an asymmetric lag (a cut reaches savers at once and a
  rise after fourteen days, borrowers the other way about; the series
  before its first publication is taken as its first value); the spreads
  and lags are published values the bank sets from the parameter page,
  the rates derived and read. The definition carries the policy
  (`definition.Bank.Policy`); the bank refuses to open without one. The
  runner resolves a bank-scoped read through the policy, logging the
  algorithm's reads on the posting's record; `Products.BankParameter`
  and `ProductParameter` resolve for the pages, so a product's and an
  account's rate is the one in force, not the published figure. The
  Bank tab gains the policy: its name and version, the chart of the
  risk-free rate against the bank's savings and lending rates over the
  run (the daily snapshot now carries both), and its parameters at the
  bank scope, each with its page and, for a published value, the set
  form; `SetParameter` takes an owner, a product or the bank. The
  Model Bank lists `easy-access-tracker` on `easyaccess/v2`, tracking
  the bank's savings rate at 15 bps under it, floor 0, so a new account
  may open on it and its accrual moves when the risk-free rate does.
  The core gains `PolicyQueries`, `core.Policy` and `BankOwner`.
- Story 9.4: an API token for the console. A request carrying
  `GOBANK_API_TOKEN` as a bearer token is served as the operator, who
  may drive the simulation console and set its settings and nothing
  else; a wrong token is refused with 401, never sent to the login
  page. gobank-deploy's drill, which sets the day length through the
  console, had been redirected to the login since the staff web went
  behind it (1.7.1) and read the redirect as success, which is why the
  v0.35.0 and v0.36.0 drills failed at the upgrade hop.

### Changed
- The roadmap is in task-plus's `tp roadmap` format: twenty named epics
  of items, each an open issue labelled `roadmap` on the Forgejo (#66 to
  #130), the forge the truth about what is done. The shipped stories
  of the old numbered roadmap are archived in this changelog under the
  versions that finished them, as `### Roadmap` lists; the stages with
  open stories (products as versioned code, users and access, speed)
  are epics of their own; `roadmap.d2` is drawn from the open items.
  ADRs and `upgrade-drill.md` still cite the old story numbers as
  history.
- The market's series opens with the publication in force before the
  opening day (0.75%, decided 2 August 2018) rather than a seed dated
  the opening day's noon, so a bank has its market's rate from the
  start and a tracker may open on the first morning; the first
  publication the run meets as an event is the cut of 11 March 2020
  (ADR-0010 amended). A publication the clock has already passed has
  its cascade settled by `Publish` itself.
- What a rule writes in answer to an event (an account setting, the
  ISA's running total) is no longer counted as a parameter change owed
  to the account that wrote it: a reopened bank raised an adoption root
  for it and a console step settled instead of starting the day.

### Roadmap
- Story 1.8.9: policy as code — the contract gains a parameter derived
  by code, the hook ADR-0006's lag and take become, and the Model Bank's
  policy is code (the extraction of `modelbank` into a module of its own
  is a roadmap item of its own): its savings and lending base rates from
  the GBP risk-free rate with a spread and an asymmetric lag (a cut
  reaches savers at once and a rise after a lag, borrowers the other way
  about), the decision an algorithm the bank's module owns, spread and
  lag published and changeable by setting; and a documentation structure
  for a bank definition with the graphs that show it: the risk-free rate
  against the bank's savings and lending rates over the run.
  `easyaccess/v2` tracking the bank savings rate at − 15 bps, floor 0,
  new accounts opening on it, its accrual moving when the risk-free rate
  does
- Story 9.4 API token for the console — gobank-deploy drives the demo's
  console as a third party (the drill's day length, the performance
  run's batch add, Run and Stop), and since 1.7.1 the console sits
  behind the staff login, so its posts were redirected to the login
  page and read as success (the v0.35.0 and v0.36.0 drills failed on
  it, see 1.8.8). A request carrying `GOBANK_API_TOKEN` as a bearer
  token is served as the operator, a role that may drive the console
  and set its settings and nothing else; a wrong token is refused
  outright, never sent to the login. gobank-deploy generates the token
  per environment as it does the passwords, keeps it on the server's
  labels, writes it to the deploy env, sends it on every console post
  and treats a redirect to the login as a refusal. Ships with 1.8.9.
  Out of scope: tokens for users, scopes beyond the console

## [0.36.0] - 2026-10-13

 - Story 1.8.8: the Bank tab and parameter pages in HTMX master-detail; ADRs served whole with their diagrams

### Added
- Story 1.8.8: the bank's parameters and their editing. A third tab
  under Products, Bank, carries what is the bank's and not a product's:
  the pound's market (the rate in force, the next decision day, the
  rate's history, moved off the savings page), the reserve ratio (moved
  off the console's settings) and the parameter at the market scope; the
  bank scope waits for policy as code. A product page shows its own
  parameters, the version scope, alone. Every parameter row links the
  parameter's own page, `products/{owner}/parameters/{key}`: its
  declaration, the value in force at the clock's instant, the settings
  upcoming and the set form, which moves there from the product card.
  The pages are a master-detail in HTMX (lofigui's multipage pattern):
  the detail loads beside the master as a fragment, the form posts and
  swaps the detail, and while the simulation runs the master and the
  detail's state re-poll themselves alone, so a page open while an
  event fires does not go stale and nothing typed is wiped; the same
  route without `HX-Request` renders the whole page, so a parameter
  deep-links. The core gains `ProductQueries.Parameter(owner, key)`.
- ADRs published whole: the `adr` package embeds each record's diagrams
  and their goluca sources beside its markdown and serves them at
  `about/docs/adr/`, so a served ADR shows its pictures; `about/adr`
  lists the records with number, title, status and date and is in the
  About menu, the documentation page keeping its summary; the static
  site renders `adr/*.md` to `docs/adr/` with the same index.

### Changed
- Events moves from the top of the staff navigation into the Internal
  menu.
- The roadmap is seventeen numbered epics in place of four phases: the
  ADR-0002 transition is epic 1 with its story numbers unchanged, and the
  Phase 1 backlog, telemetry and Phases 2 to 4 are regrouped into epics
  2 to 17 (mobile apps, payment rails, workflows, customers, finance,
  performance, resilience, delivery pipeline, blue-green, Kubernetes,
  data governance, bank migration, a second bank, Isle of Alba, local
  process on the device, distributed scale), each opening with its
  done-when. `roadmap.d2` is drawn from the file. The duplicate item 5
  (the demo phone frame, story 1.6.2) is dropped.
- The movement page writes money in the drawing as the bank writes it
  (`-£1,001.27`) and fills the boxes in the bank's colours, through
  pta2svg's new `Commodities` and `Palette` options, in the browser as
  by the BFF; the drawing's caption goes; the page is quiescent once
  loaded, never re-polled; and the movement's code is expanded into its
  ISO 20022 domain, family and sub-family (`PMNT:RCDT:BOOK`: Payments /
  Received Credit Transfers / Internal Book Transfer). pta2svg is
  pinned at v0.5.0 in both modules.

### Fixed
- The product total on the products page follows the session's view, as
  every other balance does; it was always the bank's view.
- Found by a headless-browser check before release: while the
  simulation ran, the layout's one-second whole-page poll re-fetched the
  product masters and wiped the detail a row had loaded, and a row link
  inside the polling master inherited its `hx-select` so the detail it
  fetched was filtered to nothing. The masters now render with the
  whole-page poll off (`masterPage`) and poll themselves, and a row link
  selects the detail from its reply. The layout's status tag now follows
  the console on every page, static ones included; it said Stopped on a
  parameter page while the clock ran.
- The set form pre-filled tomorrow as the effective instant, which went
  stale while the clock ran (the form never re-renders) and the core
  refused it; while the simulation runs the instant is left empty,
  which means now.
- The pass test's expectation of a January application on a savings
  account read the balance's sign in the customer's view; since
  ADR-0008 (v0.35.0) a savings balance is negative in the bank's view,
  and the test went red on main.

### Roadmap
- Story 1.8.8: the bank's parameters and their editing — a third tab
  under Products, `products/bank`, for what is the bank's and not a
  product's: the pound's market (rate in force, next decision day,
  history — moved off the savings page), the reserve ratio (moved off
  the console), and from policy as code the policy's spread and lag. The
  tab shows the contract's `bank` and `market` scopes, a product page
  its `version` scope alone, the console keeping the simulation's
  controls. A parameter row links to the parameter's own page,
  `products/{owner}/parameters/{key}`: its declaration (scope, kind,
  derivation), the value in force, the settings upcoming, and the set
  form. HTMX (lofigui's multipage pattern): the detail loads into the
  master as a fragment, the form posts and swaps the detail and the
  upcoming list, and the lists re-poll as the clock moves, so a page
  open while an event fires does not go stale; the same route renders
  the full page without `HX-Request`, so a parameter deep-links. Found
  on the way and fixed here: the product total on the products page
  ignores the session's view (`FormatMoney(p.Balance)` where every other
  page goes through `view.Money`). Out of scope: setting a market value
  (it is a feed), setting a derived parameter, bulk set, history
  (1.8.12). Drill 2026-10-13 preprod: v0.35.0 → v0.36.0, upgraded 15 s,
  handover intact; the drill failed at the upgrade hop's gate, "the run
  went on before the stop: day 8273 when last observed, day 8277 at the
  stop", and rollback and forward did not run. Cause: gobank-deploy's
  console client sets the 2 h day length by posting the settings form
  with no session; since v0.28.0 (story 1.7.1) the staff web is behind
  the login, answers the post with a redirect to it, and the client
  reads anything under 400 as success, so the day length stayed at zero
  and the run went on flat out (12 s a day) between the observation and
  the stop. The v0.34.0 → v0.35.0 drill failed the same way (3984 →
  3991); the drills before it passed on an empty bank at day 0, which
  cannot move. Fixed by the API token (story 9.4)
- Story 9.3 ADRs published whole, with a root of their own — the running
  system serves an ADR's diagrams beside its text (`adr` embeds the
  SVGs and goluca sources with the markdown, served at
  `about/docs/adr/`; found on ADR-0008, every ADR with a d2 is
  affected); `about/adr` lists the records (number, title, status,
  date) and is linked from the nav, the docs page keeping its summary;
  the static site renders `adr/*.md` to `docs/adr/` with the same
  index, so docs.bytestone.uk carries them. Ships with 1.8.8. Out of
  scope: ADR templates, a status workflow

## [0.35.0] - 2026-10-12

 - Story 1.8.7: a movement keeps its sign (ADR-0008 amended); reset release

### Changed
- Story 1.8.7, ADR-0008 amended: a movement is a signed amount m from
  account a to account b, kept as written. The runner records a
  contract posting as the contract wrote it, so savings interest is a
  negative amount from `Expense:Interest` to the account and loan
  interest a positive one from `Income:Interest`; the ledger wrapper's
  `Post` no longer turns a negative amount about and its parameters are
  `from`, `to` and `amount`. The bank's own events follow the same rule:
  a deposit is negative from `Asset:Cash` to the savings account, a
  drawdown positive to the loan, a transfer negative from the payer's
  account to the payee's; the GL's funding, transfer, opening balances
  and journal likewise. `core.Movement` has `From` and `To` legs in
  place of `Credit` and `Debit`; the movement page shows them so, and
  its goluca text and drawing carry the sign. The statement reads a
  line's type from the account's signed change. The ADR's decided and
  movement diagrams are redrawn. The page and the diagrams use pta2svg's
  layered layout, so the account a movement is from sits left of the
  account it is to and the arrow reads with the movement; pta2svg
  v0.4.0 spaces the boxes to fit the arrow's label. The BFF hands
  pta2svg the movement as its model, the ledger as data, so no text is
  parsed: the page embeds the model as JSON and the browser draws it
  with pta2svg's lite build compiled by TinyGo, 0.7 MB in place of the
  24 MB parser build (`task wasm:pta2svg` needs `tinygo`). The goluca
  text is still shown and served. Running banks: preprod and prod are
  reset on this release, the drill a fresh deploy as in 1.8.2

### Roadmap
- Story 1.8.7: a movement keeps its sign (ADR-0008 amended) — the ledger
  records a contract posting as written, a signed amount from the
  counterparty to the account, and the bank writes its own events the
  same way (a deposit negative from cash, a transfer negative from payer
  to payee); the runner's and the wrapper's turning-about of a negative
  amount goes, the wrapper's parameters are `from`, `to` and `amount`,
  the GL's funding, transfer, opening and journal follow, the statement
  reads a line's type from the account's signed change, and the
  movement page shows savings interest as −£1.27 from `Expense:Interest`
  to the account. Credit and debit leave the vocabulary. Preprod and
  prod reset on the release, the drill a fresh deploy as in 1.8.2

## [0.34.0] - 2026-10-12

 - Story 1.8.6: events and their cascades are recorded (ADR-0011 accepted); Owed read from the record; Events page

### Added
- Story 1.8.6, ADR-0011 accepted: events and their cascades are
  recorded. A new `events` component holds the roots: every command
  (the start of a day, a publication, a payment, a staff setting, an
  opening) raises one, pending at the instant its work is due and done
  when its cascade has finished. Postings, settings and payments carry
  their root as `cause_id`; a rule's posting names its `step` and the
  record holds (cause, account, step) unique, so a double effect is
  refused by the database. Each account's work in the daily pass is a
  transaction of its own: the movement, the posting record and the
  position commit together or not at all. `StartDay` settles every root
  due: a restart replays the roots, each effect proven by its own fact.
  An Events page on the staff web lists the latest roots with their
  status and what each wrote; the movement page names a rule posting's
  cause and step. A kill-and-replay test fails the transaction after
  each effect in turn and proves the facts identical to an uninterrupted
  run; a second proves a failed command leaves nothing.
- ADR-0011 (proposed): events and their cascades are recorded. Every
  command is a root event, its effects carry the cause, a restart
  replays the root, and idempotency is structural from the first slice.
  Roadmap story 1.8.6 builds its first slice ahead of policy as code;
  the stories after it are renumbered 1.8.7 to 1.8.9.
- Roadmap: 1.8.7 the bank's parameters tab and HTMX master-detail
  editing (with the products page's view fix), 1.8.11 history of
  parameters and products, and ADRs published whole with a root page;
  policy as code and the stories after it renumbered 1.8.8 to 1.8.10.

### Changed
- `core.Owed` is days, unprojected accounts and events (root events due
  and not done); the parameter-change and payments-in-flight counts are
  gone, the first being the pending roots and the second done in the
  payment's own transaction. `/about.json` reports `owed.events` in
  place of `parameter_changes` and `payments`; `at_rest` is unchanged.
- gobank-products v0.8.0: `Posting.Step`; the runner refuses a posting
  with no step.
- Schema: `events` v1; `products` v5 (`cause_id`, `step`, unique index
  on the effect); `parameters` v2 and `payments` v2 (`cause_id`, and
  `contract_payments` carries it). Additive; an older build ignores them.

### Roadmap
- Story 1.8.6: events and their cascades are recorded (ADR-0011) — every
  command is a root event in `bank/events` (`events`: roots and, later,
  continuations, with a status), written by the command and carried
  down as the cause of what it writes: `cause_id` on postings, settings
  and payments, `step` on postings with a unique key on (cause, account,
  step), recorded with the movement in its transaction; each account's
  pass work in a transaction of its own; `Owed` read from the record plus the pass's
  unprojected count; the console showing cascades by root and a
  posting's cause; gobank-products gains `Posting.Step` (v1 names its
  one posting `apply`). Restart replays the root, never the effects: a
  kill-and-replay test fails the transaction after each effect in turn
  and asserts the facts identical. One root per day for accruals, the
  position each account's fact. Out of scope: continuations (sleep,
  maturity, 1.8.11), human steps (the first workflow), any change to
  `core.Commands`

## [0.33.0] - 2026-10-11

## [0.32.0] - 2026-10-11

 - Story 1.8.5: the simulation moves by events (ADR-0010) — stepped clock, timeline, Owed, ParameterChange cascade, staff settings at an instant

### Added
- Stage 8 story 1.8.5: the simulation moves by events (ADR-0010). The
  clock is stepped: it stands at an event's instant while the bank
  processes the event and everything it causes, and moves to the next
  event only when the bank reports itself at rest. The timeline is the
  start of each day, the market's decision days (the Bank of England's
  MPC calendar, in `bank/market`, the next one named on the products
  page), the series' publications, the generator's payments (a set
  number a day, drawn from the seed, raised when the clock reaches
  each) and the settings staff make. The core gains `Owed` (days,
  accounts unprojected, parameter changes unfired, payments in flight;
  on the dashboard and in `/about.json` as `owed`, with `at_rest` for a
  drill to wait on), `Markets`, `UpcomingSettings` and the
  `SetParameter` command; the products page sets one of a product's
  own parameters effective at an instant and lists what is upcoming.
  The `ParameterChange` event is fired: `bank/products` keeps the
  instant each account's parameters are applied up to
  (`product_account_parameters`) and, when a setting at a scope the
  account's rules read becomes effective after it, fires the rule with
  the value then and now, so a market publication reaches every
  account through its derivation with no list of dependants kept.
- The settings page gains the generator's payments per day; the
  parameters component publishes `contract_parameter_settings`.

### Changed
- The day length is spread between a day's events in proportion to the
  simulated gap between them, and applies to the wait in progress; the
  run row records the instant (`sim_run.current_at`), the day and a slot
  start still written for one release for the drill's rollback.
- A transfer is completed when accepted: the posting is the settlement,
  stamped by the clock, so the bank is at rest when the command
  returns; a payment a previous process left in flight is settled when
  the bank reopens.
- Auto Send puts payments on the timeline: they flow while the run is
  going, not at wall pace; Send Payment is still an event run at once.
- The opening day's rate arrives with the market's noon publication, an
  event, rather than being published at open.

### Roadmap
- Story 1.8.5: the event clock (ADR-0010) — the simulation moves by events,
  not slots: a timeline of the start of day, the market's decision days
  (its published calendar: an event at which the feed publishes a change
  or nothing, the next one named on the products page), the payments the
  generator raises and the settings staff make. The
  clock stands at an event while the bank processes it and all it
  causes — a market change reaching the policy, the policy the product
  rates, the rates every account through the `ParameterChange` event,
  fired here (moved up from the last story) — and moves to the next
  event only when the bank reports itself at rest: the pass complete, no
  account unprojected, no parameter change unfired, no payment in
  flight. The day-length knob paces the wall clock between events. A
  staff page sets a product's declared parameter effective at a future
  instant, one more event on the timeline, with upcoming settings shown
  on the products page

## [0.31.0] - 2026-10-11

 - Story 1.8.4: the risk-free rate is market data (ADR-0009) — bank/market, settings effective at an instant, Publish command, float rates retired

### Added
- Stage 8 story 1.8.4: the risk-free rate is market data (ADR-0009). A
  currency's official rate comes from its market: `bank/market` names the
  pound's (the Bank of England's Bank Rate, its page and feed, published
  at noon London time) and records each publication as a setting at the
  contract's new `market` scope, owner the currency, that only the feed
  writes through the one new core command, `Publish`. Settings are
  effective from an instant rather than a day, and a rule's parameter for
  a day is its value at the end of that day, so a noon publication counts
  for the whole day. The treasury's reserve accrual reads the pound's
  rate; a day the market has published no rate for is refused, with the
  reason on the dashboard and in `/about.json`. The simulation replays
  the Bank's historical series as the feed, catching up when the bank
  opens so a run from before this release has its publications.

### Changed
- The last floating-point rates are gone: `core.BaseRateSource` is
  retired, the position, the history and the charts carry the rate in
  basis points, and `daily_snapshots` gains `risk_free_bps` (the decimal
  column is still written for one release, for the drill's rollback).
- gobank-products pinned at v0.7.0 (the `market` scope).

### Roadmap
- Story 1.8.4: the risk-free rate (ADR-0009; the first half of what was
  1.8.3) — a currency's official risk-free rate is market data, not the
  bank's: `bank/market` names each currency's source (GBP: the Bank of
  England Bank Rate, its page and the IADB feed `cmd/boefetch` already
  reads, published 12:00 London on decision days; EUR and USD later,
  each with its own process) and writes each publication as a setting at
  the contract's new `market` scope, owner the currency, key
  `risk_free_rate_bps`, effective at the instant published and decided
  then. Settings become effective at an instant rather than a day, and a
  rule's parameter for a day is its value at the end of that day, so a
  noon publication counts for the whole day, as the Bank intends. The
  simulation replays the historical series through the component as the
  feed would; a deployment polls. The treasury's reserve accrual reads
  GBP's rate; `core.BaseRateSource` and the last `float64` rates go,
  basis points in their place (`boe_rate` written beside them for one
  release, for the drill's rollback); the products page shows the rate's
  history and the rate in force; a day the market has published no rate
  for is refused, the console showing why. Nothing cascades yet: the v1
  rules ignore a parameter change and the day rule reads the rate for its
  day. Drill: preprod accrues the same reserve interest as before

## [0.30.0] - 2026-10-11

 - Stage 8 story 1.8.2: the catalogue as products on rules versions (ADR-0006 as amended), the runner over the product contract, the parameters component

### Added
- Stage 8 story 1.8.2: the catalogue and the runner (ADR-0006 as
  amended). A product is an instance of a rules version with its own ID
  and name: the `products` table records each with the rules version it
  runs on, its adoption and withdrawal days and the gobank-products
  release that carried its rules; the six initial products are adopted
  from the Model Bank's definition (`bank/definition`, package
  `modelbank`) when the bank opens. `bank/products` dispatches the open,
  post-posting and day events to the rules an account's product runs on
  and records every rule posting in `product_postings` with the product,
  its rules, the event, the parameters read and the position they applied
  to. Accounts on a product whose rules the build does not carry are left
  unprojected and counted. `bank/parameters` holds the bank's settings,
  each effective from a day and decided at a moment; a product's
  parameters are its own, defaulting to what its rules publish.
- The products page shows each product's rules version, published
  parameters and the accounts holding it; rates are shown in basis points
  everywhere and the `float64` rate is gone from the bank.

### Changed
- gobank-products pinned at v0.6.0 (the product contract).
- The demo's bank-package import rule lets test files alone import the
  Model Bank's definition and the product versions, since a component's
  tests run it against the Model Bank (ADR-0007).

### Roadmap
- Story 1.8.1: (gobank-products v0.6.0) gobank-products: the contract
  and v1 of every product —
  `Version`, `Facts`, events and intents, parameter declarations with
  scope and derivation; six `<product>/v1` packages reproducing today's
  behaviour exactly with the bank's rates moved in as published basis
  points; goldens re-pinned per package; the feature framework,
  `SimContext`, `Simulation` and `ParameterStore` retired. A library
  release; gobank stayed on v0.3.0 until the catalogue
- Story 1.8.3: the catalogue and the runner —
  `products` as instances of rules versions (ADR-0006 as amended: each
  with its own ID and name, the rules version it runs on, adoption and
  withdrawal days; the six initial products adopted from the definition
  at open) and `product_postings` recording the product, its rules, the
  event, the parameters read and the position applied to; accounts name
  the product alone; `bank/parameters` with its settings table (the
  rules' account settings need it from the start; a product's parameters
  are its own, defaulting to what its rules publish); `bank/products`
  dispatching open, post-posting and day to the account's product's
  rules and leaving accounts on products whose rules the build lacks
  unprojected and counted; the bank opened with its definition
  (`bank/definition`, package `modelbank`); the `float64` rate gone from
  the bank, rates in basis points through the customer view; the
  products page showing each product's rules, parameters and accounts.
  Found and fixed on the way: the GL journal collision at month end
  (v0.29.1)

## [0.29.1] - 2026-10-11

 - Fix: the GL close failed at every month end once a product had more than one account (the journal now aggregates a product's applications)

### Fixed
- The GL close failed at every month end once a product had more than
  one account: the journal grouped the day's interest applications by
  each customer account's own path, so a product with several
  applications inserted duplicate journal lines and the close stopped at
  the day before. The journal now groups by the P&L account and its side,
  one line per product and code, and the GL posts through the month end.

## [0.29.0] - 2026-10-10

 - Stage 8 story 1.8.2a: the ledger is the bank's view (ADR-0008), the movement page drawn by pta2svg, the staff view switch; preprod and prod are reset on this release

### Added
- ADR-0007 (accepted): the bank's definition is a module of its own, the
  client bank's repository on its own cadence, over a stable interface.
- ADR-0008 (accepted): the ledger is the bank's view. Credit-normal
  accounts (liabilities, equity, income) are negative, debit-normal
  (assets, expenses) positive, and a movement runs from the account
  credited to the account debited. `Asset:Cash` is the counterparty of
  customer flows on both charts: a deposit credits the saver and debits
  cash, a drawdown credits cash and debits the loan, savings interest
  credits the saver and debits the expense. All contracts and code speak
  this view; presentation varies.
- The movement page beneath every statement line (`movements/{id}`): the
  journal entry's two legs with the balances they are left with, the
  amount, code, value and knowledge times, and the movement drawn by
  pta2svg from goluca text; served as the page, as goluca
  (`movements/{id}.goluca`) and as JSON. The core gains
  `LedgerQueries.Movement`. The page draws in the browser with pta2svg's
  WASM build when `task wasm:pta2svg` has put it in the binary (the
  release build does), else by the BFF. The build is 26 MB of
  tree-sitter grammars; a small model-input build is pta2svg #8.
- The staff web's view switch: balances in the bank's view by default,
  the customer's per session from the navbar. The customer app is the
  customer's view, savings and loans both positive, with the customer's
  position with the bank as the headline. `internal/view`.
- `core.Transaction.Family`.

### Changed
- Every read of a balance stops correcting signs: the book negates the
  savings positions, interest expense is the expense account's balance,
  the transfer funds check reads the liability, statement running
  balances are the bank's view, the BBSI figure is the positive amount
  paid, the position's cash is the `Asset:Cash` balance and the balance
  sheet's cash follows it. The GL's funding, transfer and journal post
  the same directions; `ledger.Post` names its parameters credit and
  debit.
- Preprod and prod are reset on this release rather than their history
  converted: the first story whose drill is a fresh deploy, named as
  ADR-0003's exception (see upgrade-drill.md).

### Roadmap
- Story 1.8.2: the ledger is the bank's view
  (ADR-0008; taken before the catalogue completed; preprod and prod
  reset on it) — posting directions per the ADR's table, `Asset:Cash`
  on both charts, the ledger wrapper's parameters named credit and debit,
  every read's sign correction gone, the staff web's view switch (bank's
  view by default) and the app's customer view with its position with the
  bank; a movement page beneath every statement line, the journal entry
  drawn as the directed graph of Accounting for Programmers by pta2svg,
  as a WASM component on the staff and customer web, the movement served
  as goluca text and as data (the core gains a movement-by-ID query;
  pta2svg needs a public package, a WASM build and opening balances
  first, raised there); no conversion
  of running banks: preprod and prod are reset on this release, the
  drill a fresh deploy named as ADR-0003's exception

## [0.28.0] - 2026-10-09

 - Stage 7 story 1.7.1: the users component and the staff login

### Added
- ADR-0006, products as versioned code with bitemporal parameters
  (accepted 2026-10-10): a product version is a package in
  gobank-products answering a fixed event set with intents; rates are
  parameters with effective-dated settings, a product rate deriving from
  a bank rate that the bank's policy derives from the base rate with a
  spread and a lag; the bank stores adoption, the account's version and
  a record on every rule posting. Roadmap 1.8 refined into four stories;
  story 1.8.1 shipped as gobank-products v0.6.0, which gobank adopts in
  1.8.2.
- ADR-0002 stage 7 story 1.7.1, the users component and the staff login.
  `bank/users` owns `users` and `user_roles`: a user is a login name, an
  argon2id password hash (the OWASP minimum, 19 MiB, t=2, p=1, stored as
  a PHC string) and the roles it holds, with a UUID v4 identity that is
  never the login name. The bank opens with its first admin, the user
  `admin`, whose password is the deployment's `GOBANK_ADMIN_PASSWORD`
  (set again at every start, so a database restored from another
  environment takes this one's; `demo` in the tab). The staff web has a
  login page: every page but the login, the favicon and `about.json`
  needs a signed-in member of staff, the role is the user's, and the
  layout shows who is signed in with a log out. Staff sessions are kept
  in the database beside the customer app's (`staff_sessions`, sessions
  component v2), so an upgrade keeps staff signed in; a reset is a new
  run and signs everyone out. Five failed logins lock a login name or an
  address for fifteen minutes. gobank-deploy sets and shows the admin
  password as it does the app password.

### Changed
- The role switch is gone: the role comes from the signed-in user. Until
  more users can be made, the demo has the admin alone.
- The sessions and the login lockout live in `internal/session`, shared
  by the BFF and the staff web; package `bff` keeps their names.

### Roadmap
- Story 1.7.1: the component and staff login — the `users` table and
  migration, password hashes, roles; a login page on the staff web; the
  first admin's password a per-environment secret gobank-deploy sets and
  shows, as it does the app password today; the session carries the
  user and the role switch goes. Done when preprod is logged into with
  a staff password after the drill

## [0.27.1] - 2026-10-09

 - Updating roadmap

## [0.27.0] - 2026-10-09

 - Merge old products branch

## [0.26.0] - 2026-10-09

 - Stage 6a story 1.6a.2: the general ledger opened, posted and closed in shadow, reconciled daily

### Added
- ADR-0005 stage 6a story 1.6a.2, the general ledger in shadow. A second
  go-luca ledger in the bank's database (`bank/gl`; tables and views
  prefixed `gl_`) with a control account per product and the bank's own
  accounts. Funding and transfers across products post to the controls in
  the event's transaction, inserts only; `Transfer` now writes in one
  transaction. Once a day's pass completes the GL closes the day before:
  the journal (the day's interest applications from the sub-ledger's
  movements, one movement per product and code), each control's position
  with the sub-ledger's summed accrual, the reconciliation per product and
  the close on record. Days close in sequence, so a catch-up after
  downtime skips none, and a close owed at start is made then. The
  position carries the posted-through day and the break count and the
  dashboard shows them; reads are otherwise unchanged. A bank that ran
  before the GL existed is adopted: its first close takes each control's
  opening balance from the sub-ledger, so the upgrade reconciles from its
  first day.

### Known
- An import through the console rebuilds the sub-ledger but not the GL,
  so the close after it reports a break per product.

### Roadmap
- Story 1.6a.1: (go-luca v0.4.0) go-luca enablers (go-luca #8, #9) — a second ledger in one database (a table
  and view prefix on `NewSQLLedger`), and indexes for day-bounded reads
  (movements by value time, positions by day) so the journal and the
  reconciliation scan a day, not the table
- Story 1.6a.2: the GL opened and posted in shadow — the chart, per-event
  control movements in the event's transaction, the journal and the GL
  close at pass completion (idempotent per day, product, code), the
  posted-through day, the reconciliation reported on the dashboard;
  reads unchanged. Done when preprod closes days with no break

## [0.25.0] - 2026-10-09

 - adding business day

### Changed
- go-luca v0.5.0, go-postgres v0.8.0 and gobank-db v0.3.0. The ledger's
  business day follows the bank's: `StartDay` advances it before the
  pass projects anything on it, and on a restart or a fresh bank on the
  clock's day, so go-luca's live positions view reads the day and the
  day before as two slices of the day index instead of finding each
  account's latest row (go-luca #7). Whole-ledger reads such as the book
  are what this is for. go-postgres makes `sum`, `avg`, `min` and `max`
  over NUMERIC columns exact on pglike.

## [0.24.0] - 2026-10-09

 - Stage 6 story 1.6.4: the console is served by the staff web over a Console interface; stage 6 (One BFF) is complete

### Changed
- ADR-0002 stage 6 story 1.6.4, the console through the BFF. The
  simulation console's pages (the dashboard and its controls, the
  payments generator, settings and the restart record, the runtime page,
  export and import, the DB explorer and `/about.json`) are served by the
  staff web (`bff/staff`) over `staff.Console`, an interface `DemoState`
  implements: start, stop, advance, reset, add customers, the payments
  generator, settings, the restart record, the runtime (process, data
  store, schema versions, day in progress), export, import and the
  database for the explorer. The site serves the console's pages only
  when given a console. `Site.Handle` and the exported page services are
  gone. The day-length parser is `internal/daylength`, shared by the
  simulation and the settings page. Stage 6's done-when holds: `cmd/demo`
  has no HTML and no handler, only wiring, the simulation and the
  console, and the app, the customer web and the staff web are all served
  by the BFF, in the tab as on a server. The JSON at `/about.json` is
  unchanged.

### Roadmap
- Story 1.6: One BFF — the staff UI and the customer web render through the
  BFF, which becomes the demo's one handler, and the WASM build serves that
  handler in the tab (ADR-0002 stage 6; absorbs item 5 and the HTML half of
  item 8, passkeys excluded). The BFF keeps no state of its own: the console
  it serves is an interface the demo implements. Out of scope: a staff
  login (the role switch stays; 1.7 adds it), staff sessions in the
  database (10.3 needs them), the audience split, and the screens app.md draws (its stories
  1 to 4). Done-when of the stage: `cmd/demo` holds no HTML and no handler,
  only wiring, the simulation and the console.
- Story 1.6.4: the console through the BFF — the dashboard controls, settings,
  runtime, restarts, export and import and the DB explorer move to
  `bff/staff` over a `Console` interface (start, stop, advance, reset, add
  customers, the payments generator's send, run and stop, settings,
  restarts, export, import, explorer, the status `/about.json` reports)
  that `DemoState` implements, replacing `Site.Handle`. Done-when of the
  stage. Done: `staff.Config.Console`; the site serves the console's
  pages only when given one; `internal/daylength` parses the day length
  for the simulation and the settings page; `cmd/demo` holds no HTML and
  no handler

## [0.23.0] - 2026-10-09

 - No changes: tagged before story 1.6.4 was merged

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

### Fixed
- The products pages took eight seconds on the Hetzner bank: each request
  summed every ledger movement per product. The products are now the
  catalogue over the book reading, which already groups the live
  positions by product, so the savings and lending pages cost what the
  dashboard does and are served from the same cached, background-refreshed
  reading. `products.Books` is gone; `Bank.Products` composes it.

### Roadmap
- Story 1.6.3: staff pages through the BFF — the layout, the role switch, PII
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

### Roadmap
- Story 1.6.2: customer web through the BFF — the hand-rendered phone frame
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

### Roadmap
- Story 1.6.1: one handler, in the tab — the demo's routes become one
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

## [0.18.0] - 2026-10-07

 - Stage 5 story 1.5.4: products, customers, payments and the bank itself are packages; the core is in packages and the demo is wiring, console and UI (ADR-0004 applied)

### Roadmap
- Story 1.5: Core into packages — one component at a time, into `bank/<component>`
  packages of the root module, each owning its tables and migrations behind
  its API, with `bank` the composition root that implements `core.Commands`
  and `core.StaffQueries`. Smallest coupling first so the pattern (the
  registry names a package, the contract-view test scans `bank/`, an
  import test is the boundary) is proven before the entangled components
  move. Every story is a pure move, no schema change. Done-when of the
  stage: `cmd/demo` holds only wiring, the console and the UI.
- Story 1.5.4: products, customers, payments and the bank, as one story:
  `bank/products` (the catalogue, the pass and the accrual over the
  ledger), `bank/customers` (the store, the register, opening and
  transactions, over ledger and products), `bank/payments` over all of
  the above, and `bank` the composite that replaces `DemoState` and
  dissolves `coreAdapter`; book and about read through `core`. Stories
  1.5.4 to 1.5.7 of the original breakdown, taken together and released
  once; the stage's done-when

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

### Roadmap
- Story 1.5.2: history — `bank/history`: the daily snapshots
- Story 1.5.3: ledger — `bank/ledger`: the go-luca wrapper, the chart of
  accounts, the account locks and `postEvent`

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

### Roadmap
- Story 1.5.1: treasury — `bank/treasury`: gilt yields and holdings,
  `Buy`; reads only the bank's business day

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

### Roadmap
- Story 1.4: Events and clock — bank and simulation split; generators and an
  injected clock. The simulation differs from the bank in where its
  events come from and in the clock and market data it reads, nothing
  else; the stage ends with the simulation a package of its own that
  knows the bank only through `core`.
- Story 1.4.3: the split — the simulation moves into `cmd/demo/sim`, built
  on `core` alone: run loop, generators, settings, rate series and
  console status; it drives the bank through `core.Commands` and
  reads it through `core.StaffQueries`. Reset becomes wiring (a fresh
  bank over a wiped database); separate locks replace the shared
  `ds.mu`. Done-when of the stage: the compiler is the enforcement
  and a test checks the package's imports

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

### Roadmap
- Story 1.4.2: the clock — `core.Clock` injected into the bank, which takes
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

### Roadmap
- Story 1.4.1: entry points — `core.CustomerCommands.OpenCustomer` (record,
  PII, opening deposits and loan disbursements, with the lending
  headroom a bank rule that trims or refuses the loan) and
  `core.DayCommands.StartDay`; the generators and the console reach
  the bank through these and `Transfer` only, and the daily
  new-customer roll leaves `startDay` for the run loop. Drops
  `accrual_state`, retired since v0.11.0. Drilled on preprod 2026-10-06

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

### Roadmap
- Story 7.1 **Performance run on Hetzner, small and large** — a gobank-deploy
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
  on the box, OpenTelemetry (7.2), cost per customer. Open: the spans
  (10 minutes each?); whether a run should also report the day's
  wall-clock at a fixed customer count, for comparison with the
  2026-10-01 ten-minute days

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

### Roadmap
- Story 1.3: Pipelined accruals — start-of-day workflow writes next-day
  projections; interest application is product code inside it. A
  projection is an account's position at the start of a day: its
  balance and its accrued-but-unapplied interest. go-luca publishes
  the position as contract views and decides how it is kept (stored
  projections, sums over ranges); gobank-products holds the rules,
  one account at a time, and stores nothing; the demo's products
  component owns no table once `accrual_state` is retired. Numbers
  cross the contract as NUMERIC, so how go-luca stores them
  (integers, numerator and denominator) stays inside it.
- Story 1.3.5: the start-of-day workflow — the date advances at the
  start of the slot; the pass over every account runs at the start
  of the day at the system's capacity (the day length is headroom,
  the rest of the day idle) and resumes after a restart from the
  projections already written; a transfer or funding rewrites
  the touched account's next-day projection; the engine's account
  map and the end-of-day sweep go. Done-when of the stage; drilled
  with a restart mid-pass

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

### Roadmap
- Story 1.3.1: (go-postgres v0.7.0) enabler, go-postgres: NUMERIC in pglike — a `::numeric`
  cast and arithmetic, rounding and comparison on it give exact
  decimals with PostgreSQL's scale rules, so a contract view can
  publish a NUMERIC column computed from integers on both drivers
- Story 1.3.2: (go-luca v0.3.0) go-luca: two contract views — `contract_ledger_eod_positions`
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
- Story 1.3.3: (gobank-products v0.2.0, gobank v0.10.0) gobank-products: `Product.NextDay` — a pure function from
  an account's projection and the day's balance to the next day's
  projection and the ledger postings it calls for, with the
  application cycle (daily, monthly, annual) a product parameter
  and month-end application gone as a separate pass; the existing
  sweep loops over it so the goldens hold
- Story 1.3.4: projections are the truth — the demo writes each day's
  pass as go-luca projections and posts customer movements with
  projections; balance and accrued reads come from the views;
  `accrual_state` goes (the BoE reserve's accrual becomes a
  projection on its account). Shipped with `accrual_state` still
  shadow-written so the drill's rollback loses nothing; (e) drops
  it. Needed go-luca v0.3.1 (`Positions`, constant-cost movement
  projection, accrual carried forward on a movement's new day)

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

### Roadmap
- Story 1.2: Stored truth — transactions as a ledger projection, stored chart
  snapshots, sessions in the database. From here every story is an
  upgrade of the running Hetzner demo, downtime accepted and recorded
  until stage 8 ([ADR-0003](adr/0003-upgrade-in-place-until-stage-8.md);
  deployment level 2 on the manual's
  [maturity ladder](https://man.bytestone.uk/maturity.html)), and
  development moves to a branch per story merged by pull request on
  the Forgejo (code level 1).
- Story 1.2.6: chart histories as stored daily snapshots —
  the `history` component, one row a day in `daily_snapshots`
- Story 1.2.7: sessions in the database — `bff.Sessions`,
  with a SQL store over the demo's database (the `sessions`
  component)

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

### Roadmap
- Story 1.2.4: about/runtime shows the schema version per
  component, as `/about.json` already does
- Story 1.2.5: transactions as a ledger projection replacing `txLog`

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

### Roadmap
- Story 1.2.3: upgrade drill — the demo keeps a restart
  record (`restarts`, an expand-only migration of the simulation
  component): each process start against the stop before it, with
  the downtime and the run's day and customers on both sides, shown
  on the settings page; `upgrade-drill.md` is the drill — redeploy
  N to N+1 on Hetzner mid-day, read the downtime off the record,
  roll back to N from the release store and forward again, confirm
  nothing is lost — and every later story's acceptance. Needs
  gobank-deploy story 1i (stop timeout, fetch a named tag)
- Story 9.1 Release from the story branch — `tp release` on the branch
  runs the checks, writes the changelog and opens the pull request; on
  main it tags the merged heading (task-plus story 006). gobank's share:
  the branch-per-story habit and `task-plus.yml`

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

### Roadmap
- Story 1.2.2: resume — the demo stops dropping its tables at start and
  rebuilds its state from the database, with a schema version table
  and versioned migrations replacing the ad hoc `ALTER TABLE`. Also,
  from (a)'s sense-check: the settings form is static under the page
  poll (an HTMX status section polls, the form does not, as the
  dashboard already does) so a value being typed is not wiped before
  Save; and the dashboard's day tile counts down the seconds to the
  end of the day

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

### Roadmap
- Story 1.2.1: simulated day length — a setting (`GOBANK_DAY_LENGTH`,
  zero means flat out; shown and set on the simulation page) so a
  day can take two hours and an upgrade lands mid-day

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

### Roadmap
- Story 1.1: Seams — core commands/queries as interfaces; `DemoState` adapts to
  them; the BFF runs in the demo process and the app shows real data.
 
- Story 1.1.2: (gobank-deploy v0.3.0) gobank-deploy sets `GOBANK_APP_PASSWORD` per environment
  and shows it (its story 1g, gobank-deploy v0.3.0)
- Story 1.1.3: Android debug build — `app/android` and `app/ios`
  generated and committed, cleartext traffic allowed in the debug
  manifest only — then (a) sense-checked from a phone against
  Hetzner (2026-10-03)
- Story 1.1.4: staff queries — `core.StaffQueries`, one
  interface per component (book, customer register, payments,
  products, treasury), every staff page rendering from them
- Story 1.1.5: commands — `core.Commands`: `Transfer` (the
  payments generator calls it) and `BuyGilt` (the treasury page
  calls it). Opening a customer stays inside the generator until
  stage 4 splits generators from the bank

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

### Roadmap
- Story 1.1.1: customer contracts in `core`, contract suite,
  demo adapter, BFF at `/v1/`

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
