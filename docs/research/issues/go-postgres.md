# Issue draft — hum3/go-postgres

**Title:** nextval() is UPDATE-then-SELECT (non-atomic) with sequence name spliced into SQL; file::memory: DSN not treated as shared

Found in a July 2026 cold review of the gobank family (see
gobank `docs/research/cold-review-2026-07.md`).

1. **`nextval` atomicity** (v0.5.2 driver.go:298-302; still present at
   v0.5.4): an `UPDATE seq SET last_value = last_value + 1` followed by a
   *separate* `SELECT last_value` — interleaving between the two statements
   can hand two callers the same value. A stress test (8 goroutines × 50
   calls, shared file DB, v0.5.2) did **not** reproduce duplicates — execution
   appears serialized today — but the pattern is fragile.
   `UPDATE … RETURNING last_value` closes the window for free (SQLite ≥3.35
   supports RETURNING).

2. **SQL splicing of sequence names**: `'%s'` interpolation with no quote
   escaping in driver.go:298/302/307 and translate_sequence.go:110/149. Any
   application that derives sequence names from input has an injection
   primitive. Bind as a parameter, or validate the identifier and double
   embedded quotes.

3. **In-memory DSN detection is exact-match only** (driver.go:36): only the
   literal `":memory:"` gets the shared-database treatment. gobank's demo
   uses `file::memory:?_pragma=temp_store(2)` and, verified, each pooled
   connection gets a separate empty database ("no such table" on the second
   connection). Consider recognising all in-memory forms (`file::memory:`,
   `mode=memory`) or documenting the constraint prominently.
