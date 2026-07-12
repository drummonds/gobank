# Benchmark Results

Run on Intel Core Ultra 7 165H, Linux, Go 1.25.3, pglike (SQLite `:memory:`) backend.
(These results predate the move to Go 1.26.0 — re-run to refresh.)

## Day-scaling (real customer pipeline, pglike)

Uses `generateCustomer` with seed 42 — each customer gets 1-3 accounts randomly.
Pre-populated customers, then simulation advances day-by-day with interest accrual,
BoE rate lookup, history recording, and go-luca ledger movements.

| Customers | Accounts | Days | Total | us/day | acct-days/sec | Allocs |
|-----------|----------|------|---------|---------|---------------|--------|
| 1 | 3 | 7 | 50ms | 7,262 | 413 | 16K |
| 1 | 3 | 30 | 58ms | 1,943 | 1,544 | 67K |
| 1 | 3 | 60 | 200ms | 3,335 | 900 | 135K |
| 1 | 3 | 180 | 615ms | 3,417 | 878 | 405K |
| 1 | 3 | 365 | 1.4s | 3,721 | 806 | 820K |
| 10 | 20 | 7 | 140ms | 20,092 | 995 | 84K |
| 10 | 20 | 60 | 1.0s | 17,445 | 1,146 | 719K |
| 10 | 20 | 365 | 7.0s | 19,246 | 1,039 | 4.4M |
| 100 | 199 | 7 | 1.4s | 203,752 | 977 | 834K |
| 100 | 199 | 60 | 11.4s | 190,591 | 1,044 | 7.1M |
| 100 | 199 | 365 | 77s | 211,220 | 942 | 43.5M |

**~1000 acct-days/sec**, consistent across all sizes — linear scaling, no degradation over time.

## HTTP overhead

1 customer, 3 accounts, 60 days via httptest POST /advance:

| Mode | Total | us/day |
|------|-------|--------|
| Direct | 200ms | 3,335 |
| HTTP | ~76ms | 1,276 |

HTTP overhead is small relative to the simulation cost.

## Dashboard render

~1ms per render (1 customer, 60 days of history).

## Bottleneck

The dominant cost is go-luca's per-account-per-day SQL round-trips through go-postgres:

1. `GetAccountByID` — fetch account details
2. `validateSameExponent` / `BalanceAt` — query closing balance
3. `RecordMovement` — insert interest movement

That's 3 SQL operations per account per day. Each query passes through go-postgres's
PG→SQLite translation layer (~15 regex passes per query).

### go-postgres translation overhead

CPU profiling shows `go-postgres.Translate` at ~13% of runtime. Memory profiling shows
the `translate*` functions (Tokenize, translateInterval, translateNow, translateCast,
translateSerial, etc.) allocating ~800MB cumulatively per benchmark run.

### Optimisation paths

1. **Batch interest in go-luca** — single bulk query for all balances, bulk insert for
   movements (eliminates per-account round-trips)
2. **Real PostgreSQL** — bypasses the go-postgres translation layer entirely
3. **Query caching in go-postgres** — cache translated SQL to avoid re-translating
   identical prepared statements

## Notes on memory

The "Allocs" column (`allocs/op` from `go test -benchmem`) is cumulative allocations
over the run, not peak usage. Go's GC reclaims most of it.

## End-of-Day Processing (legacy results)

| Scenario                 | Wall Time | Peak RAM | Cumulative Allocs    |
| ------------------------ | --------- | -------- | -------------------- |
| 1,000 accounts x 3 days  | 4.8s      | ~145 MB  | 3.3 GB / 7.3M allocs |
| 1,000 accounts x 30 days | 47s       | -        | 28.9 GB / 63M allocs |
| 10,000 accounts x 3 days | 53s       | -        | 33.3 GB / 73M allocs |
