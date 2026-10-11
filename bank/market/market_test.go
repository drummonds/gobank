package market

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "git.bytestone.uk/hum3/go-postgres"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/parameters"
	"git.bytestone.uk/hum3/gobank/core"
)

func open(t *testing.T) (*Market, *parameters.Parameters) {
	t.Helper()
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, m := range parameters.Schema.Migrations {
		for _, stmt := range m.Statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatal(err)
			}
		}
	}
	params := parameters.Open(db)
	return New(params), params
}

func at(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, time.UTC) }

// The pound's market is the Bank of England: Bank Rate, published at noon
// London time, with a page for people and a feed for machines (ADR-0009).
func TestThePoundsMarketIsTheBankOfEngland(t *testing.T) {
	src, ok := ByCurrency("GBP")
	if !ok {
		t.Fatal("no GBP source")
	}
	if src.Name != "Bank of England" || src.Rate != "Bank Rate" || src.Page == "" || src.Feed == "" {
		t.Errorf("GBP source = %+v", src)
	}
	publishes := src.PublishedAt(at(2026, 3, 19, 0))
	if london, _ := time.LoadLocation("Europe/London"); !publishes.Equal(time.Date(2026, 3, 19, 12, 0, 0, 0, london)) {
		t.Errorf("published at %v, want noon London", publishes)
	}
	if _, ok := ByCurrency("XXX"); ok {
		t.Error("an unknown currency has a source")
	}
}

// A publication is a setting at the market scope: owner the currency, the
// contract's key, effective and decided at the instant published, by the
// market. The rate for an instant is the latest publication by then.
func TestAPublicationIsAMarketSetting(t *testing.T) {
	m, params := open(t)
	ctx := context.Background()
	noon := at(2026, 3, 19, 12)
	for _, p := range []core.Publication{
		{Currency: "GBP", Bps: 450, At: at(2026, 1, 1, 0)},
		{Currency: "GBP", Bps: 425, At: noon},
	} {
		if err := m.Publish(ctx, nil, p); err != nil {
			t.Fatal(err)
		}
	}
	text, ok, err := params.Resolve(ctx, nil, gbp.ScopeMarket, "GBP", gbp.RiskFreeRateKey, noon, noon)
	if err != nil || !ok || text != "425" {
		t.Errorf("the setting = %q, %v, %v", text, ok, err)
	}
	for _, c := range []struct {
		at   time.Time
		want int64
		ok   bool
	}{
		{at(2025, 12, 31, 23), 0, false},
		{noon.Add(-time.Minute), 450, true},
		{noon, 425, true},
		{parameters.EndOfDay(noon), 425, true},
	} {
		got, ok, err := m.Rate(ctx, nil, "GBP", c.at)
		if err != nil || got != c.want || ok != c.ok {
			t.Errorf("rate at %v = %d, %v, %v; want %d, %v", c.at, got, ok, err, c.want, c.ok)
		}
	}
	if got, ok, _ := m.Rate(ctx, nil, "EUR", noon); ok || got != 0 {
		t.Errorf("EUR, never published, = %d, %v", got, ok)
	}
}

// Only a currency with a market can be published: the bank cannot invent
// a risk-free rate.
func TestPublishRefusesACurrencyWithNoMarket(t *testing.T) {
	m, _ := open(t)
	err := m.Publish(context.Background(), nil, core.Publication{Currency: "XXX", Bps: 100, At: at(2026, 1, 1, 0)})
	if !errors.Is(err, ErrNoMarket) {
		t.Errorf("publishing XXX: %v, want ErrNoMarket", err)
	}
}

// The feed replaying a publication the bank already has writes nothing:
// a resumed run publishes its whole past again without a second record.
func TestAReplayedPublicationWritesNothing(t *testing.T) {
	m, params := open(t)
	ctx := context.Background()
	p := core.Publication{Currency: "GBP", Bps: 450, At: at(2026, 1, 1, 12)}
	for range 3 {
		if err := m.Publish(ctx, nil, p); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := params.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM parameter_settings`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d settings after three publications of one rate, want 1", n)
	}
}
