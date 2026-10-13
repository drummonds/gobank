package products

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
	easyaccess "git.bytestone.uk/hum3/gobank-products/easyaccess/v2"
	"git.bytestone.uk/hum3/gobank/bank/parameters"
)

// testPolicy is a policy for the component's tests: the bank's savings
// rate is the market's plus a published spread the bank may set. The lag
// is the Model Bank's business, not the runner's.
type testPolicy struct{}

func (testPolicy) Name() string { return "Test policy" }
func (testPolicy) Version() int { return 1 }
func (testPolicy) Parameters() []gbp.Declaration {
	return []gbp.Declaration{
		{Key: gbp.RiskFreeRateKey, Scope: gbp.ScopeMarket, Kind: gbp.KindBps},
		{Key: "savings_spread_bps", Scope: gbp.ScopeBank, Kind: gbp.KindBps, Published: "-50", Description: "the savings rate's spread under the market"},
		{Key: gbp.SavingsRateKey, Scope: gbp.ScopeBank, Kind: gbp.KindBps, Description: "the market plus the spread", Algorithm: func(p gbp.ParameterReader, day time.Time) (string, error) {
			rfr, err := p.Parameter(gbp.RiskFreeRateKey, day)
			if err != nil {
				return "", err
			}
			spread, err := p.Parameter("savings_spread_bps", day)
			if err != nil {
				return "", err
			}
			return strconv.FormatInt(rfr.Bps()+spread.Bps(), 10), nil
		}},
	}
}
func (p testPolicy) StartUp(r gbp.ParameterReader, day time.Time) error {
	return gbp.CheckPolicy(p, r, day)
}

// A product that reads the bank's rate resolves it through the bank's
// policy (ADR-0006, story 1.8.9): the algorithm over the market and the
// policy's published spread, or the spread the bank has set; what the
// algorithm read is on the posting's record. Without a policy the bank
// scope has only its settings, as before.
func TestBankParametersResolveThroughThePolicy(t *testing.T) {
	ctx := context.Background()
	tracker := Listing{ID: "tracker", Name: "Tracker", Rules: easyaccess.Version, Description: "tracks the bank"}
	p, books := openWith(t, testPolicy{}, append(listings(), tracker))
	if err := p.params.Set(ctx, nil, parameters.Setting{Scope: gbp.ScopeMarket, Owner: Currency, Key: gbp.RiskFreeRateKey, Value: "500", Effective: jan2.AddDate(-1, 0, 0), Decided: jan2.AddDate(-1, 0, 0), By: "market"}); err != nil {
		t.Fatal(err)
	}
	prod, _ := p.ByID("tracker")
	if got, err := p.resolve(ctx, nil, prod, "", "rate_bps", jan2, jan2); err != nil || got.Bps() != 435 {
		t.Errorf("rate_bps = %+v, %v; want 435: 500 − 50 by the policy, − 15 by the rules", got, err)
	}
	if got, err := p.BankParameter(ctx, nil, gbp.SavingsRateKey, parameters.EndOfDay(jan2), jan2); err != nil || got.Bps() != 450 {
		t.Errorf("the bank's savings rate = %+v, %v; want 450", got, err)
	}
	if got, err := p.BankParameter(ctx, nil, "savings_spread_bps", parameters.EndOfDay(jan2), jan2); err != nil || got.Bps() != -50 {
		t.Errorf("the spread = %+v, %v; want the published −50", got, err)
	}
	if _, err := p.BankParameter(ctx, nil, "no-such", parameters.EndOfDay(jan2), jan2); err == nil {
		t.Error("a key the policy does not declare resolved")
	}
	// The bank widens the spread from the next day: the product's rate follows.
	jan3 := jan2.AddDate(0, 0, 1)
	if err := p.params.Set(ctx, nil, parameters.Setting{Scope: gbp.ScopeBank, Owner: parameters.OwnerBank, Key: "savings_spread_bps", Value: "-100", Effective: jan3, Decided: jan2, By: "alice"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := p.resolve(ctx, nil, prod, "", "rate_bps", jan2, jan3); got.Bps() != 435 {
		t.Errorf("rate on jan2 after the setting = %d, want 435 still", got.Bps())
	}
	if got, _ := p.resolve(ctx, nil, prod, "", "rate_bps", jan3, jan3); got.Bps() != 385 {
		t.Errorf("rate on jan3 = %d, want 385", got.Bps())
	}
	// An account's day records what the algorithm read.
	a := register(t, p, books, "tracker", "ada", 1000_00, jan2)
	if err := p.Open(books, a, jan2, root("open-ada")); err != nil {
		t.Fatal(err)
	}
	if _, err := p.AccountDay(books, a, jan3, root("day-3")); err != nil {
		t.Fatal(err)
	}
	f := &facts{p: p, ctx: ctx, books: books, a: a, prod: prod, day: jan3, knownAt: jan3}
	if _, err := f.Parameter("rate_bps", jan3); err != nil {
		t.Fatal(err)
	}
	if reads := f.readsText(); !strings.Contains(reads, gbp.RiskFreeRateKey+"=500") || !strings.Contains(reads, "savings_spread_bps=-100") || !strings.Contains(reads, gbp.SavingsRateKey+"=400") || !strings.HasSuffix(reads, "rate_bps=385") {
		t.Errorf("reads = %q; want the market, the spread and the bank's rate before the product's", reads)
	}
	if pol := p.Policy(); pol == nil || pol.Name() != "Test policy" {
		t.Errorf("Policy() = %v", pol)
	}

	bare, _ := openWith(t, nil, append(listings(), tracker))
	if _, err := bare.resolve(ctx, nil, prod, "", "rate_bps", jan2, jan2); err == nil || !strings.Contains(err.Error(), "no bank setting") {
		t.Errorf("without a policy the bank's rate resolved: %v", err)
	}
	if err := bare.params.Set(ctx, nil, parameters.Setting{Scope: gbp.ScopeBank, Owner: parameters.OwnerBank, Key: gbp.SavingsRateKey, Value: "300", Effective: jan2, Decided: jan2, By: "alice"}); err != nil {
		t.Fatal(err)
	}
	if got, err := bare.resolve(ctx, nil, prod, "", "rate_bps", jan2, jan2); err != nil || got.Bps() != 285 {
		t.Errorf("without a policy a bank setting serves: %+v, %v; want 285", got, err)
	}
}
