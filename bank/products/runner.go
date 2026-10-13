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

// ErrNotCarried is an account on a product this build cannot run: one
// not on record, or whose rules the build does not carry. The runner
// leaves such an account alone: the build that carries the rules picks it
// up where it was left.
var ErrNotCarried = errors.New("products: the product's rules are not carried by this build")

// Account is an account an event is for: its ledger account and the
// product it holds.
type Account struct {
	ID      string
	Product string
}

// Cause is the root event a unit of work runs because of (ADR-0011): its
// ID, carried onto every effect the work writes, and its instant, the
// bank's time the work is due at.
type Cause struct {
	ID string
	At time.Time
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
// has nothing left to do for it. The account's parameters count as
// applied up to the cause's instant, the one it opens at (ADR-0010): a
// setting effective after that is a ParameterChange owed.
func (p *Products) Open(books *ledger.Ledger, a Account, day time.Time, cause Cause) error {
	if _, err := p.run(gbp.EventOpen, books, a, day, cause, func(v gbp.Version, f gbp.Facts) (gbp.Intents, error) { return v.Open(f) }); err != nil {
		return err
	}
	if _, err := p.AccountDay(books, a, day, cause); err != nil {
		return err
	}
	if _, err := books.Querier().ExecContext(context.Background(), `INSERT INTO product_account_parameters (account_id, applied_to) VALUES ($1, $2)`, a.ID, cause.At.UTC()); err != nil {
		return fmt.Errorf("products: %s: record parameters applied: %w", a.ID, err)
	}
	return nil
}

// AccountDay is the day event: one account's share of the start-of-day
// pass, run under the account's lock (ledger.Lock) by the pass and again
// by every event that moves the balance. The version closes yesterday
// and projects today; from the same facts it asks the same, so an
// account may be visited again after a restart, or by an event after the
// pass, and the last writer's answer is the right one. The books may be
// bound to the caller's transaction.
func (p *Products) AccountDay(books *ledger.Ledger, a Account, day time.Time, cause Cause) (DayResult, error) {
	return p.run(gbp.EventDay, books, a, day, cause, func(v gbp.Version, f gbp.Facts) (gbp.Intents, error) { return v.Day(f) })
}

// PostEvent posts a customer event — a transfer, a funding — to the
// ledger (ledger.Post: a signed amount from the first account to
// the second, both positions for the day moving at once), then runs the
// post-posting rule of each product account it touched: the projection
// the pass wrote was on the balance as it stood, and the day's interest
// accrues on the closing balance. The movement each rule sees is in the
// bank's view (ADR-0008): the to account's balance rises by the amount,
// the from account's falls by it. The caller holds the accounts'
// locks for as long as its transaction holds their rows, so the event and
// the daily pass take turns on an account. What the rules changed in the
// bank's totals is returned; the day's accrual is counted by the pass
// alone, so an event's results carry none.
func (p *Products) PostEvent(books *ledger.Ledger, day time.Time, cause Cause, fromID, toID string, amount luca.Amount, code, description string, touched ...Account) ([]DayResult, error) {
	if err := books.Post(fromID, toID, amount, code, day, description); err != nil {
		return nil, err
	}
	var results []DayResult
	for _, a := range touched {
		delta := amount
		if a.ID == fromID {
			delta = -amount
		}
		m := gbp.Movement{Delta: delta, Code: code, Description: description, ValueTime: day}
		r, err := p.run(gbp.EventPostPosting, books, a, day, cause, func(v gbp.Version, f gbp.Facts) (gbp.Intents, error) { return v.PostPosting(f, m) })
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
// projected, in that order, through the books' transaction, each effect
// carrying the cause (ADR-0011). A posting is recorded against the
// cause, the account and its step, which the record's key holds unique:
// a replay that reaches a step already made is refused here and the
// transaction with it.
func (p *Products) run(event gbp.Event, books *ledger.Ledger, a Account, day time.Time, cause Cause, rule func(gbp.Version, gbp.Facts) (gbp.Intents, error)) (DayResult, error) {
	prod, ok := p.ByID(a.Product)
	if !ok || !prod.Carried() {
		return DayResult{}, fmt.Errorf("%w: %s", ErrNotCarried, a.Product)
	}
	res := DayResult{Family: prod.Family()}
	ctx := context.Background()
	f := &facts{p: p, ctx: ctx, books: books, a: a, prod: prod, day: startOfDay(day), knownAt: p.now()}
	in, err := rule(prod.Rules, f)
	if err != nil {
		return res, fmt.Errorf("%s (%s v%d) %s: %w", a.Product, prod.RulesID, prod.RulesVersion, event, err)
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
		if err := p.params.Set(ctx, q, parameters.Setting{Scope: gbp.ScopeAccount, Owner: a.ID, Key: s.Key, Value: s.Text, Effective: s.Effective, Decided: p.now(), By: "rule", Cause: cause.ID}); err != nil {
			return res, err
		}
		if err := p.effect(a.ID, "setting"); err != nil {
			return res, err
		}
	}
	for _, posting := range in.Postings {
		if posting.Step == "" {
			return res, fmt.Errorf("products: %s (%s v%d) %s posts %q with no step", a.Product, prod.RulesID, prod.RulesVersion, event, posting.Description)
		}
		// The posting names its counterparty by path: a P&L account on
		// the ledger's chart. It is recorded as written (ADR-0008): from
		// the counterparty to the account, the amount signed as the
		// balance is, so savings interest is a negative amount from the
		// expense.
		counter, err := books.Account(posting.Counterparty)
		if err != nil {
			return res, err
		}
		input, _ := f.Position(posting.ValueTime)
		m, err := books.RecordMovement(counter, a.ID, posting.Amount, posting.Code, posting.ValueTime, posting.Description)
		if err != nil {
			return res, fmt.Errorf("products: post %s: %w", posting.Description, err)
		}
		if _, err := q.ExecContext(ctx, `INSERT INTO product_postings (movement_id, account_id, product_id, rules, rules_version, event, parameters, input_balance, input_accrued_num, posted_at, cause_id, step)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			m.ID, a.ID, a.Product, prod.RulesID, prod.RulesVersion, string(event), f.readsText(), int64(input.Balance), accrualNumerator(input.Accrued), f.knownAt.UTC(), cause.ID, posting.Step); err != nil {
			return res, fmt.Errorf("products: record posting %s (%s, step %s): %w", m.ID, event, posting.Step, err)
		}
		if err := p.effect(a.ID, "posting"); err != nil {
			return res, err
		}
		res.Applied += posting.Amount
	}
	for i, pos := range in.Positions {
		if _, err := books.Project(pos.AccountID, pos.Day, pos.Accrued); err != nil {
			return res, fmt.Errorf("products: project %s on %s: %w", pos.AccountID, pos.Day.Format(time.DateOnly), err)
		}
		if err := p.effect(a.ID, "position"); err != nil {
			return res, err
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
	prod    Product
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
	return f.p.versionResolver(f.ctx, f.books.Querier(), f.prod, f.a.ID, f.knownAt, &f.reads).Parameter(key, day)
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

// resolver is the parameter resolution of ADR-0006 for one declarer: a
// product's rules, for an account on the product, or the bank's policy.
// A derived parameter is its formula over its source, or its algorithm
// run over the resolver itself; a declared one is the latest setting in
// effect at the instant at its scope, as known at knownAt, or, at the
// declarer's own scope, the value it publishes. A read at the bank scope
// that the policy declares is the policy's to answer, so a product that
// tracks the bank's rate reads the policy's algorithm. Every read made
// by day, an algorithm's included, is logged where the facts ask.
type resolver struct {
	p       *Products
	ctx     context.Context
	q       ledger.Querier
	who     string // for errors: "easy-access v2", "Model Bank policy v1"
	decls   []gbp.Declaration
	own     gbp.Scope // the scope whose published values are the declarer's
	owner   func(gbp.Scope) string
	knownAt time.Time
	reads   *[]gbp.Param
	policy  *resolver // the policy's, for a version's reads at the bank scope; nil for the policy itself, or with none
}

// versionResolver resolves a product's declarations for an account on it,
// logging reads to reads when given.
func (p *Products) versionResolver(ctx context.Context, q ledger.Querier, prod Product, accountID string, knownAt time.Time, reads *[]gbp.Param) *resolver {
	return &resolver{p: p, ctx: ctx, q: q, who: fmt.Sprintf("%s v%d", prod.RulesID, prod.RulesVersion), decls: prod.Rules.Parameters(),
		own: gbp.ScopeVersion, owner: func(scope gbp.Scope) string { return p.owner(scope, prod, accountID) },
		knownAt: knownAt, reads: reads, policy: p.policyResolver(ctx, q, knownAt, reads)}
}

// policyResolver resolves the policy's declarations; nil with no policy.
func (p *Products) policyResolver(ctx context.Context, q ledger.Querier, knownAt time.Time, reads *[]gbp.Param) *resolver {
	if p.policy == nil {
		return nil
	}
	return &resolver{p: p, ctx: ctx, q: q, who: fmt.Sprintf("%s v%d", p.policy.Name(), p.policy.Version()), decls: p.policy.Parameters(),
		own: gbp.ScopeBank, owner: func(scope gbp.Scope) string { return p.owner(scope, Product{Currency: Currency}, "") },
		knownAt: knownAt, reads: reads}
}

// Parameter is the value for a day: at the end of the day (ADR-0009: a
// setting is effective from an instant, and a noon publication counts
// for its day). Logged.
func (r *resolver) Parameter(key string, day time.Time) (gbp.Param, error) {
	param, err := r.at(key, parameters.EndOfDay(day))
	if err == nil {
		r.log(param)
	}
	return param, err
}

func (r *resolver) log(p gbp.Param) {
	if r.reads != nil {
		*r.reads = append(*r.reads, p)
	}
}

// at is the value at an instant.
func (r *resolver) at(key string, at time.Time) (gbp.Param, error) {
	d, ok := declared(r.decls, key)
	if !ok {
		return gbp.Param{}, fmt.Errorf("parameter %s: not declared by %s", key, r.who)
	}
	if d.Scope == gbp.ScopeBank && r.policy != nil {
		if _, ok := declared(r.policy.decls, key); ok {
			return r.policy.at(key, at)
		}
	}
	switch {
	case d.Derived != nil:
		source, err := r.at(d.Derived.Source, at)
		if err != nil {
			return gbp.Param{}, err
		}
		r.log(source) // the source is what produced the value: on the record
		return d.Parse(strconv.FormatInt(d.Derived.Apply(source.Bps()), 10))
	case d.Algorithm != nil:
		text, err := d.Algorithm(r, startOfDay(at))
		if err != nil {
			return gbp.Param{}, fmt.Errorf("parameter %s: %w", key, err)
		}
		return d.Parse(text)
	}
	text, found, err := r.p.params.Resolve(r.ctx, r.q, d.Scope, r.owner(d.Scope), key, at, r.knownAt)
	if err != nil {
		return gbp.Param{}, err
	}
	if !found && d.Scope == r.own && d.Published != "" {
		text, found = d.Published, true
	}
	if !found {
		return gbp.Param{}, fmt.Errorf("parameter %s: no %s setting for %s at %s", key, d.Scope, r.owner(d.Scope), at.Format(time.RFC3339))
	}
	return d.Parse(text)
}

func declared(decls []gbp.Declaration, key string) (gbp.Declaration, bool) {
	for _, d := range decls {
		if d.Key == key {
			return d, true
		}
	}
	return gbp.Declaration{}, false
}

// resolve is a product's parameter for a day, for an account on it, as
// a rule reads it: at the end of the day.
func (p *Products) resolve(ctx context.Context, q ledger.Querier, prod Product, accountID, key string, day, knownAt time.Time) (gbp.Param, error) {
	return p.resolveAt(ctx, q, prod, accountID, key, parameters.EndOfDay(day), knownAt)
}

// resolveAt is a product's parameter at an instant, for an account on it.
func (p *Products) resolveAt(ctx context.Context, q ledger.Querier, prod Product, accountID, key string, at, knownAt time.Time) (gbp.Param, error) {
	return p.versionResolver(ctx, q, prod, accountID, knownAt, nil).at(key, at)
}

// ProductParameter is one of a product's parameters at an instant, as
// known at knownAt, with no account: what the bank's pages show, and the
// rate a product pays now. A product this build does not carry has none.
func (p *Products) ProductParameter(ctx context.Context, q ledger.Querier, prod Product, key string, at, knownAt time.Time) (gbp.Param, error) {
	if !prod.Carried() {
		return gbp.Param{}, fmt.Errorf("%w: %s", ErrNotCarried, prod.ID)
	}
	return p.resolveAt(ctx, q, prod, "", key, at, knownAt)
}

// BankParameter is one of the policy's parameters at an instant, as
// known at knownAt: a published value the bank may have set, or a rate
// the policy derives. Refused with no policy.
func (p *Products) BankParameter(ctx context.Context, q ledger.Querier, key string, at, knownAt time.Time) (gbp.Param, error) {
	r := p.policyResolver(ctx, q, knownAt, nil)
	if r == nil {
		return gbp.Param{}, errors.New("products: the bank has no policy")
	}
	return r.at(key, at)
}

// owner is whose setting a parameter at a scope is, for an account on a
// product: the currency's, the bank's, the product's or the account's.
func (p *Products) owner(scope gbp.Scope, prod Product, accountID string) string {
	switch scope {
	case gbp.ScopeMarket:
		return prod.Currency
	case gbp.ScopeVersion:
		return prod.ID
	case gbp.ScopeAccount:
		return accountID
	}
	return parameters.OwnerBank
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
