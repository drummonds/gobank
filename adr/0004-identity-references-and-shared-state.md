# ADR-0004: Identity, references and shared state across topologies

Status: Accepted
Date: 2026-10-06
Stage: ADR-0002 stage 5 onwards

## Context

Stage 5 moves the bank's components into packages and dissolves the
demo's one struct, whose single mutex guards every shared fact the bank
has: the book totals, the customer count, the business day, the BoE
reserve's accrual, and the counters that number customers and payments.
One mutex is correct in one process and wrong by construction in two.
The topologies ADR-0002 leads to are many processes (stage 8) and, in a
bigger system, replicas across regions. The question is what each kind of
shared state becomes so that the same packages run in every topology, and
what the packages may assume about it now without building for a future
that may not arrive.

The two reference designs are TigerBeetle and CockroachDB. TigerBeetle
has the client mint a 128-bit identifier before the request, time-ordered
with random bits, so the server never coordinates on identity and a retry
is idempotent; its state machine is single-threaded and batched, so there
are no locks to contend. CockroachDB says the same from the storage side:
never a sequence as a key, a UUID or node-plus-timestamp identifier
instead, and multi-region tables keyed by the row's home region so a
write stays local. Both separate identity, which anyone can generate,
from the human-readable sequential reference, which is the only thing
that needs allocating.

The banking domain already encodes that separation. A sort code is the
branch prefix and the account number is local to it: prefix plus local
counter. A customer's home region is their branch; a cross-region
transfer is the rare case that pays for coordination.

## Decision

Shared state is classified by who writes it, and each class is handled
the same way in every topology. The dashboard's figures need not be live
to the penny; they must be right when they settle.

| State | Truth lives in | Writer | One process | Many processes | Multi-region |
|---|---|---|---|---|---|
| Account balance, accrual | the ledger's position row | the event or the pass, in its transaction | process lock per account, plus the transaction | the transaction alone: serializable isolation, retry on conflict; the process lock stays as a local courtesy | the same; an account's rows are regional by the customer's home region |
| Book totals, customer count, NIM inputs | the ledger's views, the customers' view | nobody: derived | a read model, cached for about a second, no mutex | the same | the same, served from the local replica, settling late by design |
| Business day, day count | the daily snapshots and the run record | StartDay | one small mutex around the transition | the one workflow runner; every other process reads the day from the record | one runner per bank, not per region |
| BoE reserve accrual, posted pence | the reserve's ledger position | StartDay | inside the same transition | the same | the same |
| Lending headroom at opening | the ledger (the totals) | policy, read at opening | the read model's value, stale by a second | the same; acceptable risk policy | the same |
| Identity of a customer, payment, account | the row | the creator | minted in the plan step, before any write | the same, no coordination | the same |
| Human reference (cust-000123, PAY-000042) | a sequence | the creator | a counter resumed from the table | block allocation, or prefix plus local counter | prefix per region, the sort-code rule |

Rules the packages follow from stage 5:

1. **Identity is a value the creator mints.** go-luca's accounts and
   movements already carry UUIDs. Customers and payments still use their
   reference as their identity; making the identity a UUID minted in the
   plan step, with the reference a separate unique column, is a schema
   change and so a story of its own with a migration and a drill, placed
   before stage 8. The packages are written so that change is local to
   them.
2. **References are each package's own.** The customers package numbers
   customers, the payments package numbers payments, each behind one
   allocator with the trivial implementation (resume from the table,
   increment). Block allocation and prefixes go behind the same seam.
3. **Derived figures are a read model.** The composite computes the
   position from the ledger's end-of-day positions for the day plus the
   day's movements, and the customer count from the customers' view,
   caches the answer briefly, and holds no running totals under a lock.
4. **One writer per transition.** The composite keeps a mutex only around
   the day-start transition and the BoE close. The pass is single-writer
   for the day's accrual figures.
5. **The ledger's account lock is a local courtesy.** It keeps two
   goroutines in one process from contending on an account's rows. Once
   there are two processes the transaction is the guard, and conflicts
   are retried.

## Consequences

Easier: the composite is small, and the simulation, the BFF and the staff
pages see no difference between one process and many; the dashboard's
read model is the one place that is tuned for scale, and it gets cheaper
with replicas rather than dearer.

Harder: a figure on the dashboard can lag the ledger by a second, and a
loan may be granted against totals a second old. Both are stated policy,
not bugs. The identity change for customers and payments is owed before
stage 8.

Given up: a count that is exact at every instant, and the comfort of one
lock. The ledger is the record; everything else is a reading of it.
