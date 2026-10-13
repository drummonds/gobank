// Package bank is the bank: the composition root over the component
// packages (ledger, products, customers, payments, treasury, history,
// users)
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
	"git.bytestone.uk/hum3/gobank/bank/definition"
	"git.bytestone.uk/hum3/gobank/bank/events"
	"git.bytestone.uk/hum3/gobank/bank/gl"
	"git.bytestone.uk/hum3/gobank/bank/history"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/bank/market"
	"git.bytestone.uk/hum3/gobank/bank/parameters"
	"git.bytestone.uk/hum3/gobank/bank/payments"
	"git.bytestone.uk/hum3/gobank/bank/products"
	"git.bytestone.uk/hum3/gobank/bank/schema"
	"git.bytestone.uk/hum3/gobank/bank/treasury"
	"git.bytestone.uk/hum3/gobank/bank/users"
	"git.bytestone.uk/hum3/gobank/core"
	store "git.bytestone.uk/hum3/gobanks-customers"
)

// Schemas is every component's schema, for the wiring to apply at start
// (the ledger's tables are go-luca's own and not listed).
func Schemas() []schema.Component {
	return []schema.Component{products.Schema, parameters.Schema, customers.Schema, payments.Schema, treasury.Schema, history.Schema, gl.Schema, users.Schema, events.Schema}
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
	Definition definition.Bank   // what makes this bank this bank (ADR-0007): its name and the product versions it carries
	Clock      core.Clock        // where the bank reads the time; nil is the wall clock
	PIIKey     store.KeyProvider // the key the customers' PII is encrypted with; nil is the demo's fixed key
	Seed       int64             // the account numbers' randomness
	PassHook   func()            // called after each account the pass visits; tests cut a pass short with it
	// AdminPassword is the first admin's password, the deployment's
	// secret: the bank opens with the user "admin" signing in with it.
	// Empty opens the bank with no admin, so nobody signs in to the staff
	// web.
	AdminPassword string
}

// Bank is the bank over its database.
type Bank struct {
	// open is held shared by every entry point and exclusively by Reopen,
	// so a fresh bank over a wiped database is never seen half-built.
	open sync.RWMutex

	db            *sql.DB
	definition    definition.Bank
	clock         core.Clock
	market        *market.Market         // where the risk-free rate comes from (ADR-0009): the market's publications in the settings
	settings      *parameters.Parameters // the bank's settings (ADR-0006)
	events        *events.Events         // the record of what happened and what it caused (ADR-0011)
	key           store.KeyProvider
	seed          int64
	adminPassword string

	ledger    *ledger.Ledger
	gl        *gl.Ledger
	products  *products.Products
	customers *customers.Customers
	payments  *payments.Payments
	treasury  *treasury.Treasury
	history   *history.History
	users     *users.Users

	// mu guards the day and the figures only its start writes: the day
	// itself, the risk-free rate, the BoE reserve's accrual, the day's
	// accrual for the NIM, and the pass's state.
	mu                  sync.Mutex
	day                 time.Time
	dayCount            int
	riskFreeBps         int64              // GBP's risk-free rate in basis points as last known: the market's latest publication by the end of the day the bank closed
	reserveRatio        float64            // fraction of deposits held as BoE reserves, e.g. 0.15
	dayComplete         bool               // every account has its position for day: the start-of-day pass is done
	glStatus            core.GeneralLedger // the GL as of its last close, for the position
	nimBps              float64            // the latest snapshot's NIM, for the position
	boeAccruedNumerator int64              // BoE interest on excess reserves, numerator units over gbp.AccrualDenominator
	boePostedPence      int64              // whole pence of BoE accrual posted to the ledger, not yet applied
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
	if len(opts.Definition.Products) == 0 {
		return nil, fmt.Errorf("bank: a definition with products is needed")
	}
	if opts.Definition.Policy == nil {
		return nil, fmt.Errorf("bank: a definition with a policy is needed: the bank cannot run without its rates (ADR-0006)")
	}
	b := &Bank{definition: opts.Definition, clock: opts.Clock, key: opts.PIIKey, seed: opts.Seed, passHook: opts.PassHook, adminPassword: opts.AdminPassword}
	b.book = newReading(b.readBook, bookTTL)
	b.interest = newReading(b.readInterest, bookTTL)
	if b.clock == nil {
		b.clock = core.ClockFunc(time.Now)
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
	if err := parameters.CreateView(db); err != nil {
		return err
	}
	settings := parameters.Open(db)
	record := events.Open(db)
	b.market = market.New(settings)
	catalogue := products.New(db, settings, b.definition.Products, b.definition.Policy)
	if err := catalogue.Adopt(ctx, day); err != nil {
		return err
	}
	// A bank on record that the GL has never been posted for is adopted:
	// the GL's first close takes its balances from the sub-ledger.
	general, err := gl.Open(db, catalogue.All(), day, resumed)
	if err != nil {
		return err
	}
	glStatus, err := general.Status(ctx)
	if err != nil {
		return err
	}
	custs, err := customers.Open(db, b.key, books, catalogue, b.clock.Now, b.seed+int64(dayCount))
	if err != nil {
		return err
	}
	pays, err := payments.Open(db)
	if err != nil {
		return err
	}
	people := users.Open(db)
	if b.adminPassword != "" {
		if err := people.FirstAdmin(ctx, b.adminPassword); err != nil {
			return err
		}
	}
	b.mu.Lock()
	b.db = db
	b.settings, b.events = settings, record
	b.ledger, b.gl, b.products, b.customers, b.payments, b.history, b.users = books, general, catalogue, custs, pays, hist, people
	b.glStatus = glReport(glStatus)
	b.treasury = treasury.New(db, b.businessDay)
	b.day, b.dayCount = day, dayCount
	b.riskFreeBps, _, _ = b.market.Rate(ctx, nil, products.Currency, parameters.EndOfDay(day))
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
		// A payment is posted when accepted: one the previous process
		// left in flight is settled, so the bank is at rest (ADR-0010).
		if err := pays.SettleInFlight(ctx, b.clock.Now()); err != nil {
			return err
		}
		pos, _ := b.Position(ctx)
		log.Printf("resume: day %d (%s), %d customers, savings %d, lending %d", pos.DayCount, pos.Day.Format(time.DateOnly), pos.Customers, pos.Savings, pos.Lending)
	}
	return b.adoptSettings(ctx)
}

// adoptSettings gives a root to what was decided before the record of
// events (ADR-0011's adoption, the upgrade's one-off): a setting
// upcoming with no cause is raised as a setting event at its instant, and
// accounts owed a change already due are raised a setting event at the
// clock's instant, so the next StartDay fires them.
func (b *Bank) adoptSettings(ctx context.Context) error {
	now := b.clock.Now()
	upcoming, err := b.settings.Upcoming(ctx, nil, now)
	if err != nil {
		return err
	}
	for _, s := range upcoming {
		if s.Cause != "" {
			continue
		}
		root, err := b.events.Raise(ctx, nil, events.Setting, s.Effective, s.By)
		if err != nil {
			return err
		}
		if err := b.settings.SetCause(ctx, nil, s, root.ID); err != nil {
			return err
		}
	}
	owed, err := b.products.CountParameterChangesOwed(ctx, now)
	if err != nil {
		return err
	}
	due, err := b.events.CountDue(ctx, now)
	if err != nil {
		return err
	}
	if owed > 0 && due == 0 {
		if _, err := b.events.Raise(ctx, nil, events.Setting, now, "adoption"); err != nil {
			return err
		}
	}
	return nil
}

// Name is the bank's name, from its definition.
func (b *Bank) Name() string { return b.definition.Name }

// The components, for the wiring and for tests.
func (b *Bank) Ledger() *ledger.Ledger          { return b.ledger }
func (b *Bank) GeneralLedger() *gl.Ledger       { return b.gl }
func (b *Bank) Catalogue() *products.Products   { return b.products }
func (b *Bank) Customers() *customers.Customers { return b.customers }
func (b *Bank) Payments() *payments.Payments    { return b.payments }
func (b *Bank) Users() *users.Users             { return b.users }

// AuthenticateUser implements core.Users: the staff web's login.
func (b *Bank) AuthenticateUser(ctx context.Context, login, password string) (core.User, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	return b.users.AuthenticateUser(ctx, login, password)
}

// User implements core.Users: the user a session names.
func (b *Bank) User(ctx context.Context, id string) (core.User, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	return b.users.User(ctx, id)
}

// SetPassHook sets the function called after each account the pass
// visits; tests cut a pass short with it.
func (b *Bank) SetPassHook(fn func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.passHook = fn
}

// advanceLedgerDay moves both ledgers' business day to the bank's, before
// the pass projects anything on it: the live view then reads the day and
// the day before as two slices, an account the pass has reached on the
// first and one it has not on the second (go-luca v0.5.0). The same day
// again, after a restart, is a no-op.
func (b *Bank) advanceLedgerDay(day time.Time) error {
	if err := b.ledger.AdvanceDay(day); err != nil {
		return fmt.Errorf("bank: start day: %w", err)
	}
	if err := b.gl.AdvanceDay(day); err != nil {
		return fmt.Errorf("bank: start day: %w", err)
	}
	return nil
}

// closeGL closes the general ledger through the day before day, once
// day's pass is complete (ADR-0005: the pass of D closes D-1), and keeps
// what the close reported for the position. A break is logged as well as
// shown; a close that fails is logged and owed to the next start.
func (b *Bank) closeGL(ctx context.Context, day time.Time) {
	s, err := b.gl.Close(ctx, day.AddDate(0, 0, -1))
	if err != nil {
		log.Printf("bank: GL close: %v", err)
		return
	}
	for _, br := range s.Breaks {
		log.Printf("bank: GL break on %s: %s control %d, sub-ledger %d", br.Day.Format(time.DateOnly), br.ProductID, br.Control, br.SubLedger)
	}
	b.mu.Lock()
	b.glStatus = glReport(s)
	b.mu.Unlock()
}

// glReport is the GL's status as the position carries it.
func glReport(s gl.Status) core.GeneralLedger {
	return core.GeneralLedger{PostedThrough: s.PostedThrough, Breaks: len(s.Breaks)}
}

// businessDay is the day the bank is on, which a component books its
// facts against.
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
// until the bank is on the clock's day; then every root event due by the
// clock's instant and not done is settled: its cascade run, or run again
// after a restart, and the root marked done (ADR-0011). Stops early when
// ctx ends, leaving the rest for the next call. Nothing happens while the
// clock is still where the bank is.
func (b *Bank) StartDay(ctx context.Context) (time.Time, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	for ctx.Err() == nil {
		day := b.businessDay()
		pending, err := b.products.AnyUnprojected(ctx, day)
		if err != nil {
			return day, err
		}
		var root events.Event
		if pending {
			// Resuming the day's pass under the day's root: the one on
			// record, or raised now for a day begun before the record.
			if root, err = b.events.RaiseOnce(ctx, nil, events.StartOfDay, day, "clock"); err != nil {
				return day, err
			}
			b.progress.begin(day)
		} else if day.Before(core.BusinessDay(b.clock.Now())) {
			if day, root, err = b.startDay(ctx); err != nil {
				return day, err
			}
		} else {
			// On the clock's day with the day's work done: a fresh bank
			// or a restart; the ledgers' day is this one, and a GL close
			// the day's pass left owing is made now.
			if err := b.advanceLedgerDay(day); err != nil {
				return day, err
			}
			b.closeGL(ctx, day)
			return day, b.settle(ctx)
		}
		if err := b.advanceLedgerDay(day); err != nil {
			return day, err
		}
		b.mu.Lock()
		hook := b.passHook
		b.mu.Unlock()
		res := b.products.RunPass(ctx, b.ledger, day, products.Cause{ID: root.ID, At: root.At}, &b.progress, hook)
		left, err := b.products.AnyUnprojected(ctx, day)
		if err == nil && !left {
			b.progress.phase("closing the general ledger", 0)
			b.closeGL(ctx, day)
			if err := b.events.Done(ctx, nil, root.ID); err != nil {
				log.Print(err)
			}
		}
		if res.Visited == 0 && res.Skipped > 0 && left {
			// Only accounts on versions this build does not carry are left:
			// the day cannot complete here, and spinning on it helps nobody.
			b.progress.finish()
			return day, fmt.Errorf("bank: %d accounts are on product versions this build does not carry", res.Skipped)
		}

		// The throughput the dashboard quotes is the accounts the pass
		// visited over the whole day, begin to finish: the span the runtime
		// page reports as the last day's duration.
		elapsed := b.progress.finish()
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

// settle runs the cascade of every root due by the clock's instant and
// not done, in instant order, and marks each done when its cascade has
// finished (ADR-0011): a publication's or a setting's is the parameter
// changes it fires (ADR-0010), each account's in a transaction of its
// own, so a root replayed after a restart finds the accounts already
// applied and fires the rest; a day's root left pending by a crash after
// its last effect has nothing left and is done. What the rules posted
// moves the book. Stops early when ctx ends, leaving the rest owed.
func (b *Bank) settle(ctx context.Context) error {
	now := b.clock.Now()
	due, err := b.events.Due(ctx, nil, now)
	if err != nil {
		return err
	}
	for _, root := range due {
		if ctx.Err() != nil {
			return nil
		}
		switch root.Kind {
		case events.Publication, events.Setting:
			res := b.products.ApplyParameterChanges(ctx, b.ledger, products.Cause{ID: root.ID, At: root.At}, now, &b.progress)
			if res.Fired > 0 {
				b.book.invalidate()
				b.interest.invalidate()
			}
			left, err := b.products.CountParameterChangesOwed(ctx, root.At)
			if err != nil {
				return err
			}
			if left > 0 {
				continue // cut short, or accounts on rules this build lacks
			}
		}
		if err := b.events.Done(ctx, nil, root.ID); err != nil {
			return err
		}
	}
	return nil
}

// Owed implements core.BookQueries: what the bank has still to do for the
// instant its clock is at, read from the record (ADR-0011).
func (b *Bank) Owed(ctx context.Context) (core.Owed, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	now := b.clock.Now()
	day := b.businessDay()
	var o core.Owed
	if clockDay := core.BusinessDay(now); clockDay.After(day) {
		o.Days = int(clockDay.Sub(day).Hours() / 24)
	}
	var err error
	if o.Unprojected, err = b.products.CountUnprojected(ctx, day); err != nil {
		return o, err
	}
	if o.Events, err = b.events.CountDue(ctx, now); err != nil {
		return o, err
	}
	return o, nil
}

// Cascades implements core.EventQueries: the latest roots, newest first,
// each with what its cascade has written.
func (b *Bank) Cascades(ctx context.Context) ([]core.Cascade, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	recent, err := b.events.Recent(ctx, CascadesShown)
	if err != nil {
		return nil, err
	}
	out := make([]core.Cascade, 0, len(recent))
	for _, ev := range recent {
		c := core.Cascade{Event: eventRecord(ev)}
		if c.Postings, err = b.products.CountPostingsByCause(ctx, ev.ID); err != nil {
			return nil, err
		}
		if c.Settings, err = b.settings.CountByCause(ctx, ev.ID); err != nil {
			return nil, err
		}
		if c.Payments, err = b.payments.CountByCause(ctx, ev.ID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// CascadesShown is how many of the latest roots Cascades lists.
const CascadesShown = 50

// eventRecord is an event as the core publishes it.
func eventRecord(ev events.Event) core.EventRecord {
	return core.EventRecord{ID: ev.ID, Kind: string(ev.Kind), At: ev.At, Status: string(ev.Status), By: ev.RaisedBy, DoneAt: ev.DoneAt}
}

// Markets implements core.MarketQueries: each market the bank knows, with
// the rate the bank's day runs on (in force by its end, as the position
// has it) and its next decision day after the clock's instant.
func (b *Bank) Markets(ctx context.Context) ([]core.Market, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	now := b.clock.Now()
	day := b.businessDay()
	var out []core.Market
	for _, src := range market.Sources() {
		bps, _, err := b.market.Rate(ctx, nil, src.Currency, parameters.EndOfDay(day))
		if err != nil {
			return nil, err
		}
		m := core.Market{Currency: src.Currency, Name: src.Name, Rate: src.Rate, Page: src.Page, RateBps: bps}
		if next, ok := src.NextDecision(now); ok {
			m.NextDecision = next
		}
		out = append(out, m)
	}
	return out, nil
}

// startDay closes yesterday's bank-level books and begins the next day:
// the day's root event is raised (ADR-0011: the one on record if a
// restart comes back to it); BoE interest accrues on the excess reserves
// yesterday closed with, at the risk-free rate the market had published
// by the end of that day (ADR-0009), and is projected on the reserve
// account; the date moves on; the day's snapshot is written, so a
// restart from here resumes this day. Returns the new day and its root.
// Refused, with nothing changed, while the market has published no rate
// for the day being closed: the bank cannot accrue at a rate it does not
// have. Who joins the bank on the new day is the simulation's business,
// not the bank's.
func (b *Bank) startDay(ctx context.Context) (time.Time, events.Event, error) {
	closing := b.businessDay()
	bps, ok, err := b.market.Rate(ctx, nil, products.Currency, parameters.EndOfDay(closing))
	if err != nil {
		return closing, events.Event{}, err
	}
	if !ok {
		return closing, events.Event{}, fmt.Errorf("bank: %s has published no %s rate for %s: the day cannot start", market.GBP.Name, products.Currency, closing.Format(time.DateOnly))
	}
	// The day begins, for the runtime page, before its first write: the
	// root, then the books.
	b.progress.begin(closing.AddDate(0, 0, 1))
	root, err := b.events.RaiseOnce(ctx, nil, events.StartOfDay, closing.AddDate(0, 0, 1), "clock")
	if err != nil {
		b.progress.finish()
		return closing, events.Event{}, err
	}
	savings, lending, count := b.freshBook(ctx)
	b.mu.Lock()
	closed := b.day
	excess := savings - lending - requiredReserves(savings, b.reserveRatio)
	if excess > 0 {
		// Exact BoE interest accrual: numerator over gbp.AccrualDenominator.
		b.boeAccruedNumerator += int64(excess) * bps
	}
	movements := b.collectBoEInterestLocked(closed)
	numerator := b.boeAccruedNumerator

	b.day = closed.AddDate(0, 0, 1)
	b.dayCount++
	b.dayComplete = false
	b.riskFreeBps = bps
	day := b.day

	// The snapshot is the day the bank starts, with the NIM of the day
	// just closed; the pass accrues the new day from zero.
	snapshot := b.snapshotLocked(savings, lending, count)
	b.dayAccrualSavings, b.dayAccrualLending = 0, 0
	b.mu.Unlock()
	// The bank's own rates in force by the end of the closing day, as the
	// risk-free rate is: the record the rates chart draws.
	snapshot.SavingsRateBps, snapshot.LendingRateBps = b.bankRates(ctx, parameters.EndOfDay(closing))

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
	return day, root, nil
}

// Publish is the feed's command (core.MarketCommands, ADR-0009): a
// market's publication, recorded as a setting at the market scope under
// a root event of its own, raised at the instant published (ADR-0011);
// its cascade, the parameter changes it fires, runs when the clock is
// there: now, for a publication the clock has passed (the feed catching
// up at the opening with the rate in force before it), else at StartDay.
// A publication that changes nothing raises nothing.
// The bank's own figure for the rate follows at once when the
// publication is in force by the end of the day the bank is on; the next
// day's start accrues at it.
func (b *Bank) Publish(ctx context.Context, p core.Publication) error {
	b.open.RLock()
	defer b.open.RUnlock()
	src, ok := market.ByCurrency(p.Currency)
	if !ok {
		return fmt.Errorf("%w: %s", market.ErrNoMarket, p.Currency)
	}
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("bank: publish: %w", err)
	}
	root, err := b.events.Raise(ctx, tx, events.Publication, p.At, src.Name)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	recorded, err := b.market.Publish(ctx, tx, p, root.ID)
	if err != nil || !recorded {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("bank: publish: commit: %w", err)
	}
	day := b.businessDay()
	if p.Currency != products.Currency || p.At.After(parameters.EndOfDay(day)) {
		return nil
	}
	bps, ok, err := b.market.Rate(ctx, nil, products.Currency, parameters.EndOfDay(day))
	if err != nil || !ok {
		return err
	}
	b.mu.Lock()
	b.riskFreeBps = bps
	b.mu.Unlock()
	if !p.At.After(b.clock.Now()) {
		return b.settle(ctx)
	}
	return nil
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
// NIM and the risk-free rate — for the caller to store off the lock. Must be
// called with mu held.
func (b *Bank) snapshotLocked(savings, lending luca.Amount, count int) history.Snapshot {
	// Today's interest in minor units, from the rules' exact accrual (rate
	// maths for the NIM ratio, not storage).
	depositInterest := -float64(b.dayAccrualSavings) / gbp.AccrualDenominator // savings accrue negative (ADR-0008)
	loanInterest := float64(b.dayAccrualLending) / gbp.AccrualDenominator
	// NIM in bps: (loan interest income + BoE interest - deposit interest expense) / total deposits * 365 * 10000
	excess := savings - lending - requiredReserves(savings, b.reserveRatio)
	boeInterest := 0.0
	if excess > 0 {
		boeInterest = float64(excess) * float64(b.riskFreeBps) / 10000 / 365.0
	}
	nimBps := 0.0
	if savings > 0 {
		nimBps = (loanInterest + boeInterest - depositInterest) / float64(savings) * 365.0 * 10000.0
	}
	b.nimBps = nimBps
	return history.Snapshot{Day: b.day, Savings: savings, Lending: lending, Customers: count, NIMBps: nimBps, RiskFreeBps: b.riskFreeBps}
}

// saveSnapshot stores a day's snapshot on the bank's record and logs a
// refusal.
// bankRates is the policy's savings and lending rate at an instant, as
// known now; nil for one the policy cannot resolve (logged).
func (b *Bank) bankRates(ctx context.Context, at time.Time) (savings, lending *int64) {
	now := b.clock.Now()
	rate := func(key string) *int64 {
		p, err := b.products.BankParameter(ctx, nil, key, at, now)
		if err != nil {
			log.Printf("bank: %s at %s: %v", key, at.Format(time.RFC3339), err)
			return nil
		}
		bps := p.Bps()
		return &bps
	}
	return rate(gbp.SavingsRateKey), rate(gbp.LendingRateKey)
}

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
