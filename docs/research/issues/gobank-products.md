# Issue draft — hum3/gobank-products

**Title:** Interest accrual truncates sub-penny daily (accumulator unused); go-luca pin lags its own refactor

Found in a July 2026 cold review of the gobank family (see
gobank `docs/research/cold-review-2026-07.md`).

1. **Interest truncation (High).** At v0.1.6, `computeDailyInterest`
   (interest.go:46-52) computes `balance*rate/365` in decimal then
   `luca.DecimalToInt` truncates toward zero, discarding the remainder every
   day; `luca.Account.InterestAccumulator` is never used. Verified effect (on
   the equivalent v0.2.25 go-luca path gobank actually ships): accounts under
   ~£122 @ 3% earn zero interest forever; £10k @ 3% loses ~70p/yr.
   Also: `acct.GrossInterestRate` is float64 and enters via
   `decimal.NewFromFloat` (interest.go:49), and rate-change events carry
   float64 rates (event.go:88-89).

2. **Dependency pin contradiction.** v0.1.6's changelog describes the "move
   interest logic to product layer" refactor, but its go.mod still pins
   `go-luca v0.2.25` — the *pre-refactor* release. go-luca v0.2.26+ also
   introduces gobank-db, so consumers get a different dependency graph
   depending on which side of this pin they resolve. Recommend releasing a
   version pinned to the go-luca it was actually developed against.
