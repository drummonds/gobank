# Benchmark Results

Run on Intel Core Ultra 7 165H, Linux, Go 1.25.3, SQLite `:memory:` backend.

## End-of-Day Processing (interest accrual)

Each account gets a £1,000 deposit on day 1, then the simulation advances day-by-day
calculating daily interest (3.65% annual) via go-luca's `CalculateDailyInterest`.

| Scenario                 | Wall Time | Peak RAM | Cumulative Allocs    |
| ------------------------ | --------- | -------- | -------------------- |
| 1,000 accounts x 3 days  | 4.8s      | ~145 MB  | 3.3 GB / 7.3M allocs |
| 1,000 accounts x 30 days | 47s       | -        | 28.9 GB / 63M allocs |
| 10,000 accounts x 3 days | 53s       | -        | 33.3 GB / 73M allocs |

**~1.5ms per account-day**, scaling linearly.

## Notes on memory

The "Cumulative Allocs" column (`B/op` from `go test -benchmem`) is the total bytes
allocated and freed over the entire run, not peak usage. Go's GC reclaims most of it.
Actual peak RSS for 1,000 accounts x 3 days is ~145 MB.

## Bottleneck

The dominant cost is go-luca's per-account-per-day SQL round-trips:

1. `GetAccountByID` — fetch account details
2. `BalanceAt` — query closing balance
3. `RecordMovement` — insert interest movement

That's 3 SQL operations per account per day. Batch interest processing in go-luca
(e.g. a single query to compute all balances, then a bulk insert for interest movements)
would be the main optimisation path.
