# Changelog## [0.3.2] - 2026-03-01

 - Updating BOE display and version## [0.3.4] - 2026-03-01

 - Update WASM## [0.3.6] - 2026-03-02

 - Adding benchmarks## [0.3.8] - 2026-03-02

 - Release prep## [0.3.10] - 2026-03-02

 - Starting to get form## [0.3.12] - 2026-03-04

 - Updating gotreesitter and docs## [0.3.14] - 2026-03-04

 - Adding github pages## [0.3.16] - 2026-03-06

 - Adding project info page## [0.3.18] - 2026-03-07

 - Adding version## [0.3.20] - 2026-03-14

 - Updating to goluca
## [0.3.21] - 2026-03-14

 - Removing .task from vc

## [0.3.19] - 2026-03-07

 - Adding software hierarchy to docs

## [0.3.17] - 2026-03-07

 - Concept of blue green dB releases and unfying metatdate documentation

## [0.3.15] - 2026-03-05

 - Improving treasury display

## [0.3.13] - 2026-03-04

 - Updating check to include docs build

## [0.3.11] - 2026-03-04

 - Release prep

## [0.3.9] - 2026-03-02

 - Adding HTMX to  be more dynamic

## [0.3.7] - 2026-03-02

 - Adding graphs getting better

## [0.3.5] - 2026-03-01

 - Changing money format

## [0.3.3] - 2026-03-01

 - Changed build version number for demo

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
