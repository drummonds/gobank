// Package bank is the bank: the composition root over the component
// packages (ledger, products, customers, payments, treasury, history)
// that implements the core's commands and queries (ADR-0002 stage 5,
// story 1.5.4). It owns what no single component does: the business day
// and its start, the BoE reserve's accounting, the position and the
// profit and loss. Shared state follows ADR-0004: the figures on the
// dashboard are a read model over the ledger, one mutex guards the
// day-start transition and the BoE close, and nothing else is held under
// a lock.
package bank

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/customers"
	"git.bytestone.uk/hum3/gobank/bank/history"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/bank/payments"
	"git.bytestone.uk/hum3/gobank/bank/products"
	"git.bytestone.uk/hum3/gobank/bank/schema"
	"git.bytestone.uk/hum3/gobank/bank/treasury"
	"git.bytestone.uk/hum3/gobank/core"
	store "git.bytestone.uk/hum3/gobanks-customers"
)

// Schemas is every component's schema, for the wiring to apply at start
// (the ledger's tables are go-luca's own and not listed).
func Schemas() []schema.Component {
	return []schema.Component{products.Schema, customers.Schema, payments.Schema, treasury.Schema, history.Schema}
}

// DefaultReserveRatio is the fraction of deposits the bank holds as BoE
// reserves until a treasury policy sets it.
const DefaultReserveRatio = 0.15

// opCostPerDay is the bank's operating cost, £50.00 a day.
const opCostPerDay luca.Amount = 50_00

// CodeDailyAccrual marks the BoE reserve's daily interest accrual
// movements (income recognised as it accrues) — distinct from the
// application code (luca.CodeInterestAccrual). Customer accounts carry
// their accrual on their positions and post nothing daily.
const CodeDailyAccrual = "LDAS:FTDP:ACRU"

// Options wire a Bank.
type Options struct {
	Clock    core.Clock          // where the bank reads the time; nil is the wall clock
	Rates    core.BaseRateSource // where the bank reads the base rate; nil is a flat 5.25%
	PIIKey   store.KeyProvider   // the key the customers' PII is encrypted with; nil is the demo's fixed key
	Seed     int64               // the account numbers' randomness
	PassHook func()              // called after each account the pass visits; tests cut a pass short with it
}

// Bank is the bank over its database.
type Bank struct {
	// open is held shared by every entry point and exclusively by Reopen,
	// so a fresh bank over a wiped database is never seen half-built.
	open sync.RWMutex

	db    *sql.DB
	clock core.Clock
	rates core.BaseRateSource
	key   store.KeyProvider
	seed  int64

	ledger    *ledger.Ledger
	products  *products.Products
	customers *customers.Customers
	payments  *payments.Payments
	treasury  *treasury.Treasury
	history   *history.History

	// mu guards the day and the figures only its start writes: the day
	// itself, the base rate, the BoE reserve's accrual, the day's accrual
	// for the NIM, and the pass's state.
	mu                  sync.Mutex
	day                 time.Time
	dayCount            int
	boeRate             float64 // BoE base rate as a decimal, e.g. 0.0525; moves with the rate source
	reserveRatio        float64 // fraction of deposits held as BoE reserves, e.g. 0.15
	dayComplete         bool    // every account has its position for day: the start-of-day pass is done
	nimBps              float64 // the latest snapshot's NIM, for the position
	boeAccruedNumerator int64   // BoE interest on excess reserves, numerator units over gbp.AccrualDenominator
	boePostedPence      int64   // whole pence of BoE accrual posted to the ledger, not yet applied
	boeInterestApplied  luca.Amount
	dayAccrualSavings   int64 // interest accrued today on savings, numerator units; the pass alone writes it
	dayAccrualLending   int64
	throughput          throughput

	book     *reading[bookFigures]     // the read model: the book totals and the customer count (ADR-0004)
	interest *reading[interestFigures] // the read model: customer interest to date
	progress progress                  // the day in progress, for the runtime page
	passHook func()
}

// Open is the bank over db, whose tables are migrated (Schemas) and may
// hold a run to resume: the business day is then the latest on the
// bank's own record, the daily snapshots, and the books are read back
// from the ledger.
func Open(db *sql.DB, opts Options) (*Bank, error) {
	b := &Bank{clock: opts.Clock, rates: opts.Rates, key: opts.PIIKey, seed: opts.Seed, passHook: opts.PassHook}
	b.book = newReading(b.readBook, bookTTL)
	b.interest = newReading(b.readInterest, bookTTL)
	if b.clock == nil {
		b.clock = core.ClockFunc(time.Now)
	}
	if b.rates == nil {
		b.rates = core.BaseRateFunc(func(time.Time) float64 { return 0.0525 })
	}
	if b.key == nil {
		b.key = DemoPIIKey
	}
	if err := b.openOn(db); err != nil {
		return nil, err
	}
	return b, nil
}

// DemoPIIKey is the key the demo encrypts PII with when none is
// configured.
var DemoPIIKey store.KeyProvider = store.FixedKeyProvider{Key: []byte("gobank-demo-pii-key-32bytes!!!!!")}

// Reopen is a fresh bank over db, a wiped and migrated database: the
// state of a bank on its first day, under the same handle, so the
// simulation and the pages that hold it see the new run.
func (b *Bank) Reopen(db *sql.DB) error {
	b.open.Lock()
	defer b.open.Unlock()
	return b.openOn(db)
}

// openOn opens every component on db and reads the bank's state back
// from it.
func (b *Bank) openOn(db *sql.DB) error {
	ctx := context.Background()
	books, err := ledger.Open(db)
	if err != nil {
		return err
	}
	hist := history.New(db)
	first, latest, resumed, err := hist.Span(ctx)
	if err != nil {
		return err
	}
	day, dayCount := core.BusinessDay(b.clock.Now()), 0
	if resumed {
		day, dayCount = latest, int(latest.Sub(first).Hours()/24)
	}
	catalogue := products.New(db)
	custs, err := customers.Open(db, b.key, books, catalogue, b.seed+int64(dayCount))
	if err != nil {
		return err
	}
	pays, err := payments.Open(db)
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.db = db
	b.ledger, b.products, b.customers, b.payments, b.history = books, catalogue, custs, pays, hist
	b.treasury = treasury.New(db, b.businessDay)
	b.day, b.dayCount = day, dayCount
	b.boeRate = b.rates.BaseRate(day)
	b.reserveRatio = DefaultReserveRatio
	b.dayComplete = true
	b.nimBps = 0
	b.boeAccruedNumerator, b.boePostedPence, b.boeInterestApplied = 0, 0, 0
	b.dayAccrualSavings, b.dayAccrualLending = 0, 0
	b.throughput = throughput{}
	b.progress.reset()
	b.syncFromLedgerLocked()
	if resumed {
		if pending, err := catalogue.AnyUnprojected(ctx, day); err == nil {
			b.dayComplete = !pending
		}
	}
	b.mu.Unlock()
	b.book.invalidate()
	b.interest.invalidate()

	// The opening day's snapshot: the bank's record of having begun it.
	// On a resumed run the day is on record already and keeps its row.
	savings, lending, count := b.freshBook(ctx)
	b.mu.Lock()
	snapshot := b.snapshotLocked(savings, lending, count)
	b.mu.Unlock()
	b.saveSnapshot(snapshot)
	if latest, ok, err := hist.Latest(ctx); err != nil {
		log.Print(err)
	} else if ok {
		b.mu.Lock()
		b.nimBps = latest.NIMBps // the day's NIM is on record; this process has no accrual yet
		b.mu.Unlock()
	}
	if resumed {
		pos, _ := b.Position(ctx)
		log.Printf("resume: day %d (%s), %d customers, savings %d, lending %d", pos.DayCount, pos.Day.Format(time.DateOnly), pos.Customers, pos.Savings, pos.Lending)
	}
	return nil
}

// The components, for the wiring and for tests.
func (b *Bank) Ledger() *ledger.Ledger          { return b.ledger }
func (b *Bank) Catalogue() *products.Products   { return b.products }
func (b *Bank) Customers() *customers.Customers { return b.customers }
func (b *Bank) Payments() *payments.Payments    { return b.payments }

// SetPassHook sets the function called after each account the pass
// visits; tests cut a pass short with it.
func (b *Bank) SetPassHook(fn func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.passHook = fn
}

// businessDay is the day the bank is on, which a component books its
// facts against.
// advanceLedgerDay moves the ledger's business day to the bank's, before
// the pass projects anything on it: the live view then reads the day and
// the day before as two slices, an account the pass has reached on the
// first and one it has not on the second (go-luca v0.5.0). The same day
// again, after a restart, is a no-op.
func (b *Bank) advanceLedgerDay(day time.Time) error {
	if err := b.ledger.AdvanceDay(day); err != nil {
		return fmt.Errorf("bank: start day: %w", err)
	}
	return nil
}

func (b *Bank) businessDay() time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.day
}

// syncFromLedgerLocked reads back what the ledger holds of the bank's
// own position: the BoE reserve's accrual from its position and the BoE
// interest applied from its balance. Customer accounts need nothing: each
// one's balance and accrual is its ledger position, read where it is
// shown. Must be called with mu held.
func (b *Bank) syncFromLedgerLocked() {
	reserves := b.ledger.Chart.BoEReserves
	if p, err := b.ledger.PositionAt(reserves, b.day); err != nil {
		log.Printf("bank: BoE reserves position: %v", err)
	} else if p != nil && p.Accrued.Den == gbp.AccrualDenominator {
		b.boeAccruedNumerator = p.Accrued.Num
		// Daily posting maintains posted == floor(numerator/denominator)
		// at every sync point, so the tracker is derivable on restore.
		b.boePostedPence = p.Accrued.Num / gbp.AccrualDenominator
	}
	if bal, err := b.ledger.Balance(reserves); err == nil {
		b.boeInterestApplied = bal
	} else {
		log.Printf("bank: BoE reserves balance: %v", err)
	}
}

// --- The start of a day ---

// StartDay is the bank following its clock (core.DayCommands): a pass
// the day in progress still owes is finished first, then every day the
// clock has moved on to is started and its pass run, one day at a time,
// until the bank is on the clock's day. Stops early when ctx ends,
// leaving the rest for the next call. Nothing happens while the clock is
// still on the bank's day.
func (b *Bank) StartDay(ctx context.Context) (time.Time, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	for ctx.Err() == nil {
		day := b.businessDay()
		pending, err := b.products.AnyUnprojected(ctx, day)
		if err != nil {
			return day, err
		}
		if pending {
			b.progress.begin(day) // resuming the day's pass
		} else if day.Before(core.BusinessDay(b.clock.Now())) {
			day = b.startDay(ctx)
		} else {
			// On the clock's day with the day's work done: a fresh bank
			// or a restart; the ledger's day is this one.
			return day, b.advanceLedgerDay(day)
		}
		if err := b.advanceLedgerDay(day); err != nil {
			return day, err
		}
		b.mu.Lock()
		hook := b.passHook
		b.mu.Unlock()
		res := b.products.RunPass(ctx, b.ledger, day, &b.progress, hook)

		// The throughput the dashboard quotes is the accounts the pass
		// visited over the whole day, begin to finish: the span the runtime
		// page reports as the last day's duration.
		elapsed := b.progress.finish()
		left, err := b.products.AnyUnprojected(ctx, day)
		b.mu.Lock()
		b.throughput.record(res.Visited, elapsed)
		b.dayComplete = err == nil && !left
		b.dayAccrualSavings += res.AccruedSavings
		b.dayAccrualLending += res.AccruedLending
		b.mu.Unlock()
		b.book.invalidate()
		b.interest.invalidate()
	}
	return b.businessDay(), ctx.Err()
}

// startDay closes yesterday's bank-level books and begins the next day:
// BoE interest accrues on the excess reserves yesterday closed with and is
// projected on the reserve account; the date and the base rate move on;
// the day's snapshot is written, so a restart from here resumes this
// day. Returns the new day. Who joins the bank on the new day is the
// simulation's business, not the bank's.
func (b *Bank) startDay(ctx context.Context) time.Time {
	savings, lending, count := b.freshBook(ctx)
	b.mu.Lock()
	closed := b.day
	excess := savings - lending - requiredReserves(savings, b.reserveRatio)
	if excess > 0 {
		// Exact BoE interest accrual: numerator over gbp.AccrualDenominator.
		b.boeAccruedNumerator += int64(excess) * gbp.RateBps(b.boeRate)
	}
	movements := b.collectBoEInterestLocked(closed)
	numerator := b.boeAccruedNumerator

	b.day = closed.AddDate(0, 0, 1)
	b.dayCount++
	b.dayComplete = false
	b.boeRate = b.rates.BaseRate(b.day)
	day := b.day

	// The snapshot is the day the bank starts, with the NIM of the day
	// just closed; the pass accrues the new day from zero.
	snapshot := b.snapshotLocked(savings, lending, count)
	b.dayAccrualSavings, b.dayAccrualLending = 0, 0
	b.mu.Unlock()

	b.progress.begin(day)
	b.progress.phase("closing yesterday", 0)
	for _, m := range movements {
		if _, err := b.ledger.RecordMovement(m.from, m.to, m.amount, m.code, m.at, m.description); err != nil {
			log.Printf("bank: BoE interest: %v", err)
		}
	}
	// The BoE reserves account's accrual is a position like any other.
	reserves := b.ledger.Chart.BoEReserves
	unlock := b.ledger.Lock(reserves)
	if _, err := b.ledger.Project(reserves, closed, luca.Fraction{Num: numerator, Den: gbp.AccrualDenominator}); err != nil {
		log.Printf("bank: BoE reserves position: %v", err)
	}
	unlock()
	// The day's snapshot is the bank's record of having begun it: a
	// restart from here resumes it.
	b.saveSnapshot(snapshot)
	return day
}

// requiredReserves is the ratio of deposits the bank must hold at the BoE.
func requiredReserves(savings luca.Amount, ratio float64) luca.Amount {
	return luca.Amount(float64(savings) * ratio)
}

// ledgerMovement is a movement decided under mu and written to the
// ledger after it is released.
type ledgerMovement struct {
	from, to    string
	amount      luca.Amount
	code        string
	at          time.Time
	description string
}

// collectBoEInterestLocked models BoE reserve interest in the ledger:
// newly accrued whole pence post daily as Income:Interest:BoE ->
// Asset:AccruedInterest:BoE (income recognised as it accrues, receivable
// builds up), and at month end the receivable moves into
// Asset:BoEReserves as the interest is received. Sub-penny remainders
// carry forward in boeAccruedNumerator. The movements are returned for
// the caller to write with no lock held; a later write failure is logged
// and the counters stay ahead of the ledger. Must be called with mu held.
func (b *Bank) collectBoEInterestLocked(day time.Time) []ledgerMovement {
	chart := b.ledger.Chart
	var out []ledgerMovement
	valueTime := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 58, 0, day.Location())
	if newPence := b.boeAccruedNumerator/gbp.AccrualDenominator - b.boePostedPence; newPence > 0 {
		out = append(out, ledgerMovement{chart.IncomeBoE, chart.AccruedBoE, luca.Amount(newPence),
			CodeDailyAccrual, valueTime, "Daily BoE reserve interest accrual"})
		b.boePostedPence += newPence
	}
	if monthEnd := day.Month() != day.AddDate(0, 0, 1).Month(); monthEnd && b.boePostedPence > 0 {
		desc := fmt.Sprintf("BoE reserve interest received for month ending %s", day.Format(time.DateOnly))
		applyTime := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, day.Location())
		out = append(out, ledgerMovement{chart.AccruedBoE, chart.BoEReserves, luca.Amount(b.boePostedPence),
			luca.CodeInterestAccrual, applyTime, desc})
		b.boeAccruedNumerator -= b.boePostedPence * gbp.AccrualDenominator
		b.boeInterestApplied += luca.Amount(b.boePostedPence)
		b.boePostedPence = 0
	}
	return out
}

// boeInterestTotalLocked is the cumulative BoE reserve interest earned:
// applied into Asset:BoEReserves plus accrued-but-unapplied whole pence.
// Must be called with mu held.
func (b *Bank) boeInterestTotalLocked() luca.Amount {
	return b.boeInterestApplied + luca.Amount(b.boeAccruedNumerator/gbp.AccrualDenominator)
}

// snapshotLocked takes the day's snapshot — the book, the customers, the
// NIM and the base rate — for the caller to store off the lock. Must be
// called with mu held.
func (b *Bank) snapshotLocked(savings, lending luca.Amount, count int) history.Snapshot {
	// Today's interest in minor units, from the rules' exact accrual (rate
	// maths for the NIM ratio, not storage).
	depositInterest := float64(b.dayAccrualSavings) / gbp.AccrualDenominator
	loanInterest := float64(b.dayAccrualLending) / gbp.AccrualDenominator
	// NIM in bps: (loan interest income + BoE interest - deposit interest expense) / total deposits * 365 * 10000
	excess := savings - lending - requiredReserves(savings, b.reserveRatio)
	boeInterest := 0.0
	if excess > 0 {
		boeInterest = float64(excess) * b.boeRate / 365.0
	}
	nimBps := 0.0
	if savings > 0 {
		nimBps = (loanInterest + boeInterest - depositInterest) / float64(savings) * 365.0 * 10000.0
	}
	b.nimBps = nimBps
	return history.Snapshot{Day: b.day, Savings: savings, Lending: lending, Customers: count, NIMBps: nimBps, BoERate: b.boeRate}
}

// saveSnapshot stores a day's snapshot on the bank's record and logs a
// refusal.
func (b *Bank) saveSnapshot(s history.Snapshot) {
	if err := b.history.Save(context.Background(), s); err != nil {
		log.Print(err)
	}
}

// --- Export and import ---

// Export writes the ledger in go-luca's text form.
func (b *Bank) Export(w interface{ Write([]byte) (int, error) }) error {
	b.open.RLock()
	defer b.open.RUnlock()
	return b.ledger.Export(w)
}

// Import reads movements in go-luca's text form into the ledger, opening
// accounts as needed, and reads the bank's position back.
func (b *Bank) Import(r interface{ Read([]byte) (int, error) }) error {
	b.open.RLock()
	defer b.open.RUnlock()
	if err := b.ledger.Import(r, &luca.ImportOptions{AutoCreateAccounts: true, DefaultCommodity: ledger.Currency}); err != nil {
		return err
	}
	b.mu.Lock()
	b.syncFromLedgerLocked()
	b.mu.Unlock()
	b.book.invalidate()
	b.interest.invalidate()
	return nil
}
