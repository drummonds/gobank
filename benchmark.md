# Performance

Two figures say how fast the demo bank is, both read off the demo's
`/about.json` so a program can take them: **customers added per second**
(a batch add through the real pipeline: customer, accounts, register,
funding movements with projections) and **account days per 12h** (the
start-of-day pass's rate over the whole day, projected to a bank's
overnight run: every account visited once, its position for the day
written). The Hetzner runs are the numbers that matter; the laptop tables
below them are the baseline the design was measured against as it changed.

## Hetzner runs

Made by gobank-deploy's **perf** workflow (`tp secrets gobank-deploy perf
perf`): an environment is created at each scale, the demo set flat out,
customers added for ten minutes, days run for ten more, the rates read,
the servers removed. The command writes each run's row into this table
(`small` is cx23, shared; `large` is ccx33, dedicated cores, so the
account-days figure says whether the pass uses them); commit and release
to publish. The spans and the workflow's steps are on the gobank-deploy
Performance page.

| Date | gobank | Scale | Server | Customers reached | Customers/s | Days run | Account days / 12h | Last day |
|---|---|---|---|---|---|---|---|---|
| 2026-10-06 | v0.12.0 | small | cx23: 4 GB | 60,000 | 142.1 | 3 | 18,388,121 | 4m41s over 119,794 accounts |
| 2026-10-06 | v0.12.0-1-g479cbf4 | large | ccx33: 32 GB | 60,000 | 251.3 | 5 | 40,914,910 | 1m52s over 119,794 accounts |
| 2026-10-06 | v0.12.0 | small | cx23: 4 GB | 60,000 | 142.1 | 3 | 18,388,121 | 4m41s over 119,794 accounts |
| 2026-10-06 | v0.12.0-1-g479cbf4 | large | ccx33: 32 GB | 60,000 | 251.3 | 5 | 40,914,910 | 1m52s over 119,794 accounts |
| 2026-10-06 | v0.12.0 | small | cx23: 4 GB | 60,000 | 142.1 | 3 | 18,388,121 | 4m41s over 119,794 accounts |
| 2026-10-06 | v0.12.0-1-g479cbf4 | large | ccx33: 32 GB | 60,000 | 251.3 | 5 | 40,914,910 | 1m52s over 119,794 accounts |

For comparison, the Hetzner demo of 2026-10-01 (300k customers, v0.3.x,
one accrual posting per account per day) took over ten minutes a
simulated day at about 1.2k postings/s.

## Laptop baseline (pglike)

Run 2026-10-06 on Intel Core Ultra 7 165H, Linux, Go 1.26.0, pglike (SQLite
`:memory:`) backend, at the stage 4 (c) branch (what became v0.15.0):
positions as the truth, the start-of-day pass, the simulation a package
of its own. `task bench:days` and `task bench:baseline` reproduce them.

## Day-scaling (real customer pipeline, pglike)

Uses `generateCustomer` with seed 42 — each customer gets 1-3 accounts randomly.
Pre-populated customers, then simulation advances day-by-day with interest accrual,
BoE rate lookup, history recording, and go-luca ledger movements.

| Customers | Accounts | Days | Total | us/day | acct-days/sec | Allocs |
|-----------|----------|------|---------|---------|---------------|--------|
| 1 | 3 | 7 | 27ms | 3,898 | 770 | 94K |
| 1 | 3 | 30 | 124ms | 4,147 | 723 | 405K |
| 1 | 3 | 60 | 255ms | 4,256 | 705 | 823K |
| 1 | 3 | 180 | 755ms | 4,199 | 715 | 2.5M |
| 1 | 3 | 365 | 1.5s | 4,157 | 722 | 5.0M |
| 10 | 22 | 7 | 128ms | 18,353 | 1,199 | 456K |
| 10 | 22 | 30 | 573ms | 19,116 | 1,151 | 2.0M |
| 10 | 22 | 60 | 1.1s | 18,403 | 1,195 | 4.0M |
| 10 | 22 | 180 | 3.3s | 18,315 | 1,201 | 11.9M |
| 10 | 22 | 365 | 6.5s | 17,728 | 1,241 | 24.1M |
| 100 | 198 | 7 | 1.0s | 143,921 | 1,376 | 3.8M |
| 100 | 198 | 30 | 4.2s | 140,028 | 1,414 | 16.2M |
| 100 | 198 | 60 | 8.1s | 135,512 | 1,461 | 33.0M |
| 100 | 198 | 180 | 22.0s | 122,222 | 1,620 | 98.7M |
| 100 | 198 | 365 | 41.6s | 114,049 | 1,736 | 200.5M |

A day has a fixed cost (closing the bank's books, the snapshot, the
unprojected-accounts query) of about 4 ms, so a book of three accounts
runs at ~700 acct-days/sec and the rate climbs with the book: ~1,200 at
22 accounts, ~1,700 at 198, and 41.6s for a hundred customers' year
against 77s before positions became the truth. The Hetzner rows above
are the figure at scale.

## HTTP overhead

1 customer, 3 accounts, 60 days, three runs each:

| Mode | Total | us/day |
|------|-------|--------|
| Direct | 247–256ms | 4,130–4,269 |
| HTTP (httptest POST /advance) | 269–276ms | 4,488–4,615 |

HTTP adds about 7% to a day of three accounts; at any real size it is
noise.

## Dashboard render

~0.26ms per render (1 customer, 60 days of history), 2,251 allocations.

## Bottleneck

Profiled 2026-10-06 (`BenchmarkDayScale/pglike/c100/d60`, CPU profile):
the pass is 54% of samples and every account's day is a handful of SQL
statements through go-luca:

1. `PositionAt` yesterday — one query
2. `Apply` postings at a cycle end — a movement insert and a reprojection, monthly
3. `Balance` — one query
4. `Project` today — a reprojection and a position upsert

`SQLLedger.Project` alone is 40% of samples (`reproject` 29%,
`upsertPosition` 15%). Below it, go-postgres's PG→SQLite translation is
20% (`Translate`, `translateTokens`, `Tokenize`), because every statement
is re-translated at prepare time, and the garbage collector is 32%, the
translation's token slices being the bulk of what it collects.

### Optimisation paths

1. **Cache translated statements in go-postgres** — the pass issues the
   same few statements with different arguments; translating each once
   would take most of the 20% and much of the GC with it
2. **Project without re-reading** — `Project` re-sums to reproject; the
   pass already holds yesterday's position and today's balance, so a
   write-only projection would halve its queries
3. **Real PostgreSQL** — bypasses the translation layer entirely; the
   Hetzner rows above are on it

## Notes on memory

The "Allocs" column (`allocs/op` from `go test -benchmem`) is cumulative allocations
over the run, not peak usage. Go's GC reclaims most of it.

## End-of-Day Processing (legacy results)

| Scenario                 | Wall Time | Peak RAM | Cumulative Allocs    |
| ------------------------ | --------- | -------- | -------------------- |
| 1,000 accounts x 3 days  | 4.8s      | ~145 MB  | 3.3 GB / 7.3M allocs |
| 1,000 accounts x 30 days | 47s       | -        | 28.9 GB / 63M allocs |
| 10,000 accounts x 3 days | 53s       | -        | 33.3 GB / 73M allocs |
