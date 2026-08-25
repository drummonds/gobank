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
