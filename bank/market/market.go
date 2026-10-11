// Package market is where a currency's official risk-free rate comes from
// (ADR-0009): for the pound, the Bank of England's Bank Rate, published
// at noon London time on a decision day and effective from that moment.
// The sources are code, one per currency; a publication is a setting at
// the contract's market scope, owner the currency, that only the market's
// feed writes. The component owns no table: the settings are the
// parameters component's.
package market

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/parameters"
	"git.bytestone.uk/hum3/gobank/core"
)

// ErrNoMarket is a publication for a currency the bank knows no market
// for: the bank cannot invent a risk-free rate.
var ErrNoMarket = errors.New("market: no market for the currency")

// Source is a currency's market: who publishes its risk-free rate, what
// the rate is called, where it is published for people (Page) and for
// machines (Feed), and when on a decision day (Hour and Minute in Zone).
type Source struct {
	Currency string
	Name     string
	Rate     string
	Page     string
	Feed     string
	Hour     int
	Minute   int
	Zone     string
}

// PublishedAt is the instant a decision on day is published.
func (s Source) PublishedAt(day time.Time) time.Time {
	loc, err := time.LoadLocation(s.Zone)
	if err != nil {
		loc = time.UTC
	}
	return time.Date(day.Year(), day.Month(), day.Day(), s.Hour, s.Minute, 0, 0, loc)
}

// GBP is the pound's market. The feed is the Bank's database series for
// Bank Rate, the one cmd/boefetch reads.
var GBP = Source{
	Currency: "GBP",
	Name:     "Bank of England",
	Rate:     "Bank Rate",
	Page:     "https://www.bankofengland.co.uk/monetary-policy/the-interest-rate-bank-rate",
	Feed:     "https://www.bankofengland.co.uk/boeapps/database/_iadb-fromshowcolumns.asp?SeriesCodes=IUDBEDR&CSVF=TN&UsingCodes=Y&VPD=Y",
	Hour:     12,
	Zone:     "Europe/London",
}

var sources = []Source{GBP}

// Sources is every market the bank knows, in currency order.
func Sources() []Source { return append([]Source(nil), sources...) }

// ByCurrency is the market for a currency.
func ByCurrency(currency string) (Source, bool) {
	for _, s := range sources {
		if s.Currency == currency {
			return s, true
		}
	}
	return Source{}, false
}

// Market writes publications and reads rates over the bank's settings.
type Market struct {
	params *parameters.Parameters
}

// New is the component over the settings.
func New(params *parameters.Parameters) *Market { return &Market{params: params} }

// Publish records a publication through q, the caller's transaction or
// the database: a setting at the market scope, effective and decided at
// the instant published, by the market.
func (m *Market) Publish(ctx context.Context, q parameters.Querier, p core.Publication) error {
	src, ok := ByCurrency(p.Currency)
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoMarket, p.Currency)
	}
	// A publication that leaves the rate as it was is not a change: the
	// feed replaying what the bank already has writes nothing.
	if bps, ok, err := m.Rate(ctx, q, p.Currency, p.At); err != nil {
		return err
	} else if ok && bps == p.Bps {
		return nil
	}
	return m.params.Set(ctx, q, parameters.Setting{
		Scope: gbp.ScopeMarket, Owner: p.Currency, Key: gbp.RiskFreeRateKey,
		Value: strconv.FormatInt(p.Bps, 10), Effective: p.At, Decided: p.At, By: src.Name,
	})
}

// Rate is a currency's risk-free rate in effect at an instant, as known
// now; false when the market has published nothing by then. For a day's
// rate ask at parameters.EndOfDay.
func (m *Market) Rate(ctx context.Context, q parameters.Querier, currency string, at time.Time) (int64, bool, error) {
	text, ok, err := m.params.Resolve(ctx, q, gbp.ScopeMarket, currency, gbp.RiskFreeRateKey, at, at)
	if err != nil || !ok {
		return 0, false, err
	}
	bps, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("market: %s rate %q: %w", currency, text, err)
	}
	return bps, true, nil
}
