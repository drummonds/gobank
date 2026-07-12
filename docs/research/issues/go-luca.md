# Issue draft — hum3/go-luca

**Title:** Daily interest truncates sub-penny to zero (accumulator unused); knowledge_time not persisted; float64 rates vs README claim

Found in a July 2026 cold review of the gobank family (see
gobank `docs/research/cold-review-2026-07.md`).

1. **Interest truncation (High, verified at v0.2.25; same logic post-refactor).**
   `defaultInterest` (interest.go) computes `balance*rate/365` then
   `DecimalToInt` truncates toward zero — *every day*. Verified:
   - £100 @ 3%: 0p/day forever (any balance under ~£122 @3% earns nothing).
   - £10,000 @ 3%: 82p vs exact 82.19p — ~70p/yr lost per account.
   - `Account.InterestAccumulator` ("sub-unit fractions at extended
     precision") is never read or written by the default path.
   Fix: accumulate the remainder in InterestAccumulator and post when it
   reaches a whole unit, or accrue at extended precision and settle monthly.
   (Post-v0.2.26 this code moved to gobank-products — the fix belongs
   wherever the accrual now lives, but v0.2.25 is what gobank ships.)

2. **`RecordMovement` doesn't persist `knowledge_time`** (db.go:266): the
   INSERT relies on the schema `DEFAULT NOW()` (database clock) while the
   returned `Movement.KnowledgeTime` is Go's `time.Now()` — two clocks, and
   the caller never sees the stored value. `RecordLinkedMovements` (db.go:320)
   writes it explicitly; the two paths disagree. Bind the same timestamp that
   is returned.

3. **README says "no floating-point in core accounting"** but
   `Account.GrossInterestRate` is float64 (luca.go) and enters interest math
   via `decimal.NewFromFloat`. Amounts are properly int64 — consider scaled
   integer basis points for rates, or soften the README claim.

Positive note: the movement model (one row, one amount, from/to) is balanced
by construction and `validateSameExponent` blocks cross-exponent movements —
that core is solid.
