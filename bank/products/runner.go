package products

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/bank/parameters"
)

// ErrUnknownVersion is an account on a version this build does not carry.
// The runner leaves such an account alone: the build that carries the
// version picks it up where it was left.
var ErrUnknownVersion = errors.New("products: version not carried by this build")

// Account is an account an event is for: its ledger account and the
// version it runs on.
type Account struct {
	ID      string
	Product string
	Version int
}

// DayResult is what one account's event changed in the bank's totals: the
// interest applied to its balance and the day's accrual, in numerator
// units over gbp.AccrualDenominator.
type DayResult struct {
	Family  gbp.ProductFamily
	Applied luca.Amount
	Accrued int64
}

// Open runs the open event for a new account and then the day's rule, so
// the account has its settings and its first position: the day's pass
// has nothing left to do for it.
func (p *Products) Open(books *ledger.Ledger, a Account, day time.Time) error {
	if _, err := p.run(gbp.EventOpen, books, a, day, func(v gbp.Version, f gbp.Facts) (gbp.Intents, error) { return v.Open(f) }); err != nil {
		return err
	}
	_, err := p.AccountDay(books, a, day)
	return err
}

// AccountDay is the day event: one account's share of the start-of-day
// pass, run under the account's lock (ledger.Lock) by the pass and again
// by every event that moves the balance. The version closes yesterday
// and projects today; from the same facts it asks the same, so an
// account may be visited again after a restart, or by an event after the
// pass, and the last writer's answer is the right one. The books may be
// bound to the caller's transaction.
func (p *Products) AccountDay(books *ledger.Ledger, a Account, day time.Time) (DayResult, error) {
	return p.run(gbp.EventDay, books, a, day, func(v gbp.Version, f gbp.Facts) (gbp.Intents, error) { return v.Day(f) })
}

// PostEvent posts a customer event — a transfer, a funding — to the
// ledger (ledger.Post: amount credited to the first account, debited to
// the second, both positions for the day moving at once), then runs the
// post-posting rule of each product account it touched: the projection
// the pass wrote was on the balance as it stood, and the day's interest
// accrues on the closing balance. The movement each rule sees is in the
// bank's view (ADR-0008): the credited account's balance falls by the
// amount, the debited account's rises. The caller holds the accounts'
// locks for as long as its transaction holds their rows, so the event and
// the daily pass take turns on an account. What the rules changed in the
// bank's totals is returned; the day's accrual is counted by the pass
// alone, so an event's results carry none.
func (p *Products) PostEvent(books *ledger.Ledger, day time.Time, creditID, debitID string, amount luca.Amount, code, description string, touched ...Account) ([]DayResult, error) {
	if err := books.Post(creditID, debitID, amount, code, day, description); err != nil {
		return nil, err
	}
	var results []DayResult
	for _, a := range touched {
		delta := amount
		if a.ID == creditID {
			delta = -amount
		}
		m := gbp.Movement{Delta: delta, Code: code, Description: description, ValueTime: day}
		r, err := p.run(gbp.EventPostPosting, books, a, day, func(v gbp.Version, f gbp.Facts) (gbp.Intents, error) { return v.PostPosting(f, m) })
		if err != nil {
			return results, fmt.Errorf("products: %s after %s: %w", a.ID, description, err)
		}
		r.Accrued = 0
		results = append(results, r)
	}
	return results, nil
}

// run dispatches one event to the account's version and carries out the
// intents: settings written, postings made and recorded, positions
// projected, in that order, through the books' transaction.
func (p *Products) run(event gbp.Event, books *ledger.Ledger, a Account, day time.Time, rule func(gbp.Version, gbp.Facts) (gbp.Intents, error)) (DayResult, error) {
	prod, ok := p.At(a.Product, a.Version)
	if !ok {
		return DayResult{}, fmt.Errorf("%w: %s v%d", ErrUnknownVersion, a.Product, a.Version)
	}
	res := DayResult{Family: prod.Family()}
	ctx := context.Background()
	f := &facts{p: p, ctx: ctx, books: books, a: a, v: prod.Version, day: startOfDay(day), knownAt: p.now()}
	in, err := rule(prod.Version, f)
	if err != nil {
		return res, fmt.Errorf("%s v%d %s: %w", a.Product, a.Version, event, err)
	}
	if f.err != nil {
		return res, f.err
	}
	// The accrual the event adds is the last position's numerator over
	// what the day started from: yesterday's closed numerator when the
	// event closed it, else yesterday's as stored.
	base, _ := f.Position(f.day.AddDate(0, 0, -1))
	q := books.Querier()
	for _, s := range in.Settings {
		if err := p.params.Set(ctx, q, parameters.Setting{Scope: gbp.ScopeAccount, Owner: a.ID, Key: s.Key, Value: s.Text, Effective: s.Effective, Decided: p.now(), By: "rule"}); err != nil {
			return res, err
		}
	}
	for _, posting := range in.Postings {
		// The posting names its counterparty by path: a P&L account on
		// the ledger's chart. Its amount is signed as the balance is, so
		// savings interest is negative: credited to the saver, debited to
		// the expense (ADR-0008), which is posted the other way about
		// with the amount positive.
		counter, err := books.Account(posting.Counterparty)
		if err != nil {
			return res, err
		}
		input, _ := f.Position(posting.ValueTime)
		credit, debit, amount := counter, a.ID, posting.Amount
		if amount < 0 {
			credit, debit, amount = a.ID, counter, -amount
		}
		m, err := books.RecordMovement(credit, debit, amount, posting.Code, posting.ValueTime, posting.Description)
		if err != nil {
			return res, fmt.Errorf("products: post %s: %w", posting.Description, err)
		}
		if _, err := q.ExecContext(ctx, `INSERT INTO product_postings (movement_id, account_id, product_id, version, event, parameters, input_balance, input_accrued_num, posted_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			m.ID, a.ID, a.Product, a.Version, string(event), f.readsText(), int64(input.Balance), accrualNumerator(input.Accrued), f.knownAt.UTC()); err != nil {
			return res, fmt.Errorf("products: record posting %s: %w", m.ID, err)
		}
		res.Applied += posting.Amount
	}
	for i, pos := range in.Positions {
		if _, err := books.Project(pos.AccountID, pos.Day, pos.Accrued); err != nil {
			return res, fmt.Errorf("products: project %s on %s: %w", pos.AccountID, pos.Day.Format(time.DateOnly), err)
		}
		if i == len(in.Positions)-2 {
			base = pos // yesterday closed by this event
		}
	}
	if n := len(in.Positions); n > 0 {
		res.Accrued = accrualNumerator(in.Positions[n-1].Accrued) - accrualNumerator(base.Accrued)
	}
	return res, nil
}

// facts is the runner's Facts for one event: positions and the balance
// from the ledger, parameters resolved from the version's declarations
// and the bank's settings as known now. Every parameter read is logged,
// for the record a posting carries. A ledger read that fails is kept and
// reported after the rule, since a rule cannot return the ledger's error.
type facts struct {
	p       *Products
	ctx     context.Context
	books   *ledger.Ledger
	a       Account
	v       gbp.Version
	day     time.Time
	knownAt time.Time
	reads   []gbp.Param
	err     error
}

func (f *facts) Account() string { return f.a.ID }
func (f *facts) Day() time.Time  { return f.day }

func (f *facts) Position(day time.Time) (luca.Position, bool) {
	pos, err := f.books.PositionAt(f.a.ID, startOfDay(day))
	if err != nil {
		f.fail(err)
		return luca.Position{}, false
	}
	if pos == nil {
		return luca.Position{}, false
	}
	return *pos, true
}

func (f *facts) Balance() luca.Amount {
	b, err := f.books.Balance(f.a.ID)
	if err != nil {
		f.fail(err)
	}
	return b
}

func (f *facts) fail(err error) {
	if f.err == nil {
		f.err = fmt.Errorf("products: %s: %w", f.a.ID, err)
	}
}

func (f *facts) Parameter(key string, day time.Time) (gbp.Param, error) {
	param, err := f.p.resolve(f.ctx, f.books.Querier(), f.v, f.a.ID, key, startOfDay(day), f.knownAt)
	if err == nil {
		f.reads = append(f.reads, param)
	}
	return param, err
}

// readsText is the parameters read, "key=value" in the order first read,
// a key once with its last value.
func (f *facts) readsText() string {
	var keys []string
	last := map[string]string{}
	for _, r := range f.reads {
		if _, seen := last[r.Key]; !seen {
			keys = append(keys, r.Key)
		}
		last[r.Key] = r.Text
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + last[k]
	}
	return strings.Join(parts, " ")
}

// resolve is the parameter resolution of ADR-0006: a derived parameter
// from its source and the formula; a declared one from the latest setting
// effective on or before the day at its scope, as known at knownAt, or,
// at version scope, the published value.
func (p *Products) resolve(ctx context.Context, q ledger.Querier, v gbp.Version, accountID, key string, day, knownAt time.Time) (gbp.Param, error) {
	d, ok := gbp.Declared(v, key)
	if !ok {
		return gbp.Param{}, fmt.Errorf("parameter %s: not declared by %s v%d", key, v.Product(), v.Version())
	}
	if d.Derived != nil {
		source, err := p.resolve(ctx, q, v, accountID, d.Derived.Source, day, knownAt)
		if err != nil {
			return gbp.Param{}, err
		}
		return d.Parse(strconv.FormatInt(d.Derived.Apply(source.Bps()), 10))
	}
	owner := parameters.OwnerBank
	switch d.Scope {
	case gbp.ScopeVersion:
		owner = parameters.Owner(v)
	case gbp.ScopeAccount:
		owner = accountID
	}
	text, found, err := p.params.Resolve(ctx, q, d.Scope, owner, key, day, knownAt)
	if err != nil {
		return gbp.Param{}, err
	}
	if !found && d.Scope == gbp.ScopeVersion && d.Published != "" {
		text, found = d.Published, true
	}
	if !found {
		return gbp.Param{}, fmt.Errorf("parameter %s: no %s setting for %s on %s", key, d.Scope, owner, day.Format(time.DateOnly))
	}
	return d.Parse(text)
}

func accrualNumerator(f luca.Fraction) int64 {
	switch f.Den {
	case 0:
		return 0
	case gbp.AccrualDenominator:
		return f.Num
	}
	return f.Num * gbp.AccrualDenominator / f.Den
}

func startOfDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
