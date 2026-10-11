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
// machines (Feed), when on a decision day (Hour and Minute in Zone), and
// the decision days themselves (Calendar), as the market has issued them
// ahead: UTC dates, in order. A decision day is an event at which the
// market publishes a change or nothing (ADR-0010); what it will publish
// is not known.
type Source struct {
	Currency string
	Name     string
	Rate     string
	Page     string
	Feed     string
	Hour     int
	Minute   int
	Zone     string
	Calendar []time.Time
}

// PublishedAt is the instant a decision on day is published.
func (s Source) PublishedAt(day time.Time) time.Time {
	loc, err := time.LoadLocation(s.Zone)
	if err != nil {
		loc = time.UTC
	}
	return time.Date(day.Year(), day.Month(), day.Day(), s.Hour, s.Minute, 0, 0, loc)
}

// NextDecision is the first decision day's publication instant after
// an instant; false when the calendar runs out.
func (s Source) NextDecision(after time.Time) (time.Time, bool) {
	for _, day := range s.Calendar {
		if at := s.PublishedAt(day); at.After(after) {
			return at, true
		}
	}
	return time.Time{}, false
}

// GBP is the pound's market. The feed is the Bank's database series for
// Bank Rate, the one cmd/boefetch reads. The calendar is the Monetary
// Policy Committee's announcement dates from 2020, as the Bank published
// them (bankofengland.co.uk/news, "Monetary Policy Committee dates"),
// with the two special meetings of March 2020.
var GBP = Source{
	Currency: "GBP",
	Name:     "Bank of England",
	Rate:     "Bank Rate",
	Page:     "https://www.bankofengland.co.uk/monetary-policy/the-interest-rate-bank-rate",
	Feed:     "https://www.bankofengland.co.uk/boeapps/database/_iadb-fromshowcolumns.asp?SeriesCodes=IUDBEDR&CSVF=TN&UsingCodes=Y&VPD=Y",
	Hour:     12,
	Zone:     "Europe/London",
	Calendar: days(
		"2020-01-30", "2020-03-11", "2020-03-19", "2020-03-26", "2020-05-07", "2020-06-18", "2020-08-06", "2020-09-17", "2020-11-05", "2020-12-17",
		"2021-02-04", "2021-03-18", "2021-05-06", "2021-06-24", "2021-08-05", "2021-09-23", "2021-11-04", "2021-12-16",
		"2022-02-03", "2022-03-17", "2022-05-05", "2022-06-16", "2022-08-04", "2022-09-22", "2022-11-03", "2022-12-15",
		"2023-02-02", "2023-03-23", "2023-05-11", "2023-06-22", "2023-08-03", "2023-09-21", "2023-11-02", "2023-12-14",
		"2024-02-01", "2024-03-21", "2024-05-09", "2024-06-20", "2024-08-01", "2024-09-19", "2024-11-07", "2024-12-19",
		"2025-02-06", "2025-03-20", "2025-05-08", "2025-06-19", "2025-08-07", "2025-09-18", "2025-11-06", "2025-12-18",
		"2026-02-05", "2026-03-19", "2026-04-30", "2026-06-18", "2026-07-30", "2026-09-17", "2026-11-05", "2026-12-17",
	),
}

// days parses calendar dates, UTC.
func days(dates ...string) []time.Time {
	out := make([]time.Time, len(dates))
	for i, d := range dates {
		t, err := time.Parse(time.DateOnly, d)
		if err != nil {
			panic(err)
		}
		out[i] = t
	}
	return out
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
