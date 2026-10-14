# Sizing a retail bank: 300k, 1M and 10M customers

How much data a retail bank of each size produces, how much of it has to be
near the processor, and what that asks of the machine. The figures are
worked from gobank's own tables and its measured rates on Hetzner (see
[Performance](benchmark.md)), so they are a sizing of this design, not of
banks in general; the method carries over, the constants may not.

## Customer profiles

Two kinds of retail customer bound the problem. A **saver** holds one
account, pays in or out a couple of times a month, and receives one
interest posting a month. A **current-account customer** makes about five
payments a day (card, direct debit, transfer, salary) and usually has a
savings pot beside the current account. A real book is a mix; rather than
guess the mix, every table below gives the all-saver and all-current
bounds and a bank lands between them.

| Profile | Accounts per customer | Movements per customer per year | Source |
|---|---|---|---|
| Saver | 1 | ~40 | 2 a month in or out, 12 interest postings |
| Current account | 2 | ~1,850 | 5 a day on the current account, the pot as a saver |

A movement is one row for both sides (the ledger stores `m` from `a` to
`b`, signed), so a customer transaction is one row, not two. Interest
accrual does not post daily: the day's accrual lives in the account's
position (below) and posts once a cycle.

## What a transaction weighs

The ledger's movement row (go-luca `movements`) is four UUIDs, an amount,
a short code, two timestamps, a few integers and the description. With a
20-character description it is about 143 bytes of column data; PostgreSQL
adds a 24-byte tuple header and a 4-byte line pointer, and the five
indexes (primary key, from-account, to-account, batch, code and the day)
add about as much again.

| Form | Bytes per movement | What is counted |
|---|---|---|
| Minimum information | ~60 | two 8-byte account ids, amount, time, code, 20 chars of text |
| Minimum with UUID ids | ~76 | as above with 16-byte account ids |
| PostgreSQL row | ~180 | the 13 columns, tuple header, line pointer, alignment |
| PostgreSQL row + indexes | **~400** | the row plus five B-tree entries |
| Columnar archive (Parquet, zstd) | ~30 | the same fields, compressed in bulk |

So in the live database a transaction costs about 400 bytes and in an
archive about 30. The 7-year obligation is on the information, not the
B-trees: anything older than the online statement window can move to the
cheap form.

### The row nobody budgets for: positions

The daily pass writes every account's **position** for the day (balance,
accrued interest as a fraction, the day) into `balances_live`: about 112
bytes of row and 110 of index, **~220 bytes an account-day**, 365 rows an
account a year whether the account moved or not. For a saver that is nine
times the bytes of the saver's movements; it is the dominant storage term
for a savings bank. Positions are derived from movements, so they carry no
retention obligation of their own: the live set needs the current cycle
(say 35 days), and month-ends are enough for statements.

## Rows and bytes

Movements, per year and over the seven years:

| Customers | Profile | Movements / year | Movements / 7 years | PostgreSQL, 7 years @ 400 B | Archive, 7 years @ 30 B |
|---|---|---|---|---|---|
| 300k | saver | 12M | 84M | 34 GB | 2.5 GB |
| 300k | current | 555M | 3.9B | 1.6 TB | 117 GB |
| 1M | saver | 40M | 280M | 112 GB | 8.4 GB |
| 1M | current | 1.85B | 13B | 5.2 TB | 390 GB |
| 10M | saver | 400M | 2.8B | 1.1 TB | 84 GB |
| 10M | current | 18.5B | 130B | 52 TB | 3.9 TB |

Positions, if every account-day were kept for seven years (2,555 rows an
account, 562 KB an account):

| Customers | Profile | Accounts | Positions, 7 years @ 220 B |
|---|---|---|---|
| 300k | saver | 300k | 169 GB |
| 300k | current | 600k | 337 GB |
| 1M | saver | 1M | 562 GB |
| 1M | current | 2M | 1.1 TB |
| 10M | saver | 10M | 5.6 TB |
| 10M | current | 20M | 11 TB |

Keeping them all is the wrong answer at every size; it is in the table to
show why the placement rules below exist.

## Placement

What stays in the database is what a customer or the bank reads without
notice: live balances, the online statement window (13 months here, a
common UK figure), the current interest cycle's positions, and the
customer and account records. Everything older is written once to
columnar files on object storage and read by a batch job when a
subject-access request, a dispute or an audit asks for it.

| Data | In the database | Archived | Driver of size |
|---|---|---|---|
| Customers, accounts, products | always | copy on change | customers × ~1 KB |
| Movements, last 13 months | yes, partitioned by month | | movements/year × 13/12 × 400 B |
| Movements, months 14–84 | no: detach the month's partition, export, drop | Parquet on object storage | movements/year × 7 × 30 B |
| Positions, last 35 days | yes | | accounts × 35 × 220 B |
| Positions, month-ends | yes (statements) | | accounts × 12 × 7 × 220 B |
| Positions, other days | no: delete after the cycle | recomputable from movements | |
| Statements as rendered | no | PDF or Parquet on object storage | accounts × 84 × ~50 KB |

Hot database size under those rules:

| Customers | Profile | Movements, 13 months | Positions, 35 days + month-ends | Hot total |
|---|---|---|---|---|
| 300k | saver | 5 GB | 2 GB + 6 GB | ~15 GB |
| 300k | current | 240 GB | 5 GB + 11 GB | ~260 GB |
| 1M | saver | 17 GB | 8 GB + 18 GB | ~45 GB |
| 1M | current | 800 GB | 15 GB + 37 GB | ~860 GB |
| 10M | saver | 173 GB | 77 GB + 185 GB | ~450 GB |
| 10M | current | 8.0 TB | 154 GB + 370 GB | ~8.6 TB |

The movement table's monthly partitions are the mechanism: a partition is
detached whole, exported whole and dropped whole, so retention costs no
row-by-row deletes and the live indexes never hold more than 13 months.

## Processing

Two workloads: the transaction stream through the day and the pass at the
start of the day.

### Transaction rate

Current-account customers make their five payments across roughly sixteen
waking hours, with a peak hour about two and a half times the daytime
mean. Savers add under 1% to the rate.

| Customers | Movements / day | Mean / s (24 h) | Mean / s (16 h) | Peak / s |
|---|---|---|---|---|
| 300k | 1.5M | 17 | 26 | ~70 |
| 1M | 5M | 58 | 87 | ~220 |
| 10M | 50M | 580 | 870 | ~2,200 |

Each payment is a movement insert and a position update in one
transaction. A single PostgreSQL on NVMe sustains several thousand such
transactions a second, so the stream is not what sizes the machine even at
10M; the pass is.

### The daily pass

The pass visits every account once: read yesterday's position, apply any
cycle-end postings, read the balance, write today's position (four
statements an account). Measured on a ccx33 (8 dedicated vCPU, 32 GB) the
pass ran at 40.9M account-days per 12 h at v0.12.0 (947 accounts/s) and
13.4M per 12 h at v0.18.0 with ledger movements under it (310/s); the
range is the per-account cost, 1–3 ms.

| Customers | Profile | Accounts | Pass at 947/s | Pass at 310/s |
|---|---|---|---|---|
| 300k | saver | 300k | 5 min | 16 min |
| 300k | current | 600k | 11 min | 32 min |
| 1M | saver | 1M | 18 min | 54 min |
| 1M | current | 2M | 35 min | 1.8 h |
| 10M | saver | 10M | 2.9 h | 9 h |
| 10M | current | 20M | 5.9 h | 18 h |

Against an overnight window of six hours, one node as measured carries
300k and 1M at either rate and 10M savers at the better rate. 10M current
accounts needs the per-account cost at the 1 ms end *and* the work spread
over more than one machine, or the per-account cost cut again (the
optimisation paths in [Performance](benchmark.md): cached statement
translation, projecting without re-reading). The pass is independent
account by account, so it spreads without coordination; what it needs is
the database to keep up with several passes at once.

## Hardware

| Customers | Compute | Database storage | Archive after 7 years (current bound) | Notes |
|---|---|---|---|---|
| 300k | one ccx33 (8 vCPU, 32 GB) | 500 GB volume | 120 GB object storage | the demo's benchmark machine; pass well inside the window |
| 1M | one ccx33 or ccx43 (16 vCPU, 64 GB) | 1 TB volume | 400 GB object storage | the 13-month index tail should stay in RAM: 64 GB preferred |
| 10M | ccx53 (32 vCPU, 128 GB) or several nodes | 8–10 TB, partitioned | 4 TB object storage | a 10 TB volume is Hetzner's ceiling; the pass needs sharing out; the tier where a distributed database earns its place |

Hetzner repriced its cloud on 15 June 2026; third-party trackers put a
ccx33 at about €138 a month and a ccx53 at about €533 after the change,
with block volumes of the order of €50 a TB-month and object storage
around a tenth of that. Treat those as indicative and check the price
page before costing: the trackers disagree with one another and the
cost-optimised (cx) line has shown as unavailable since late June
([privatedevops](https://privatedevops.com/news/hetzner-june-2026-cloud-price-increase-what-to-do),
[findstack](https://findstack.com/resources/hetzner-price-increase-2026),
[bitdoze](https://www.bitdoze.com/hetzner-cloud-cost-optimized-plans)).

Memory is sized by what the pass and the live reads touch: the accounts
table (~250 B an account), the latest position of every account (220 B),
and the right-hand edge of the movement indexes. At 10M current customers
that is 20M accounts, about 10 GB of rows and indexes that must stay
cached, which 128 GB covers with room for the month's partitions.

## What the note says

1. A transaction costs about 400 bytes live and 30 archived; with a
   20-character description the information itself is under 80 bytes.
2. For a savings bank the daily positions, not the transactions, are
   the storage; keep the cycle and the month-ends and drop the rest.
3. Partition movements by month and keep 13 months live; the 7 years
   are met from object storage at a thirtieth of the live cost.
4. The transaction stream never sizes the machine at these scales; the
   daily pass does. One node as measured serves 300k and 1M; 10M
   needs the per-account cost down and the pass spread out.
