package parameters

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "git.bytestone.uk/hum3/go-postgres"
	gbp "git.bytestone.uk/hum3/gobank-products"
)

func open(t *testing.T) *Parameters {
	t.Helper()
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, m := range Schema.Migrations {
		for _, stmt := range m.Statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatal(err)
			}
		}
	}
	return Open(db)
}

func d(y int, m time.Month, dd int) time.Time { return time.Date(y, m, dd, 0, 0, 0, 0, time.UTC) }

// A setting is effective from its day and invisible before it: a rate
// change decided in January for February is February's.
func TestResolveIsEffectiveDated(t *testing.T) {
	p := open(t)
	ctx := context.Background()
	decided := d(2026, 1, 10)
	for _, s := range []Setting{
		{gbp.ScopeVersion, "easy-access/1", "rate_bps", "150", d(2026, 1, 1), decided, "migration"},
		{gbp.ScopeVersion, "easy-access/1", "rate_bps", "135", d(2026, 2, 1), decided, "treasury"},
	} {
		if err := p.Set(ctx, nil, s); err != nil {
			t.Fatal(err)
		}
	}
	known := d(2026, 3, 1)
	for _, c := range []struct {
		on   time.Time
		want string
		ok   bool
	}{
		{d(2025, 12, 31), "", false},
		{d(2026, 1, 1), "150", true},
		{d(2026, 1, 31), "150", true},
		{d(2026, 2, 1), "135", true},
		{d(2026, 6, 1), "135", true},
	} {
		got, ok, err := p.Resolve(ctx, nil, gbp.ScopeVersion, "easy-access/1", "rate_bps", c.on, known)
		if err != nil || got != c.want || ok != c.ok {
			t.Errorf("on %s: %q, %v, %v; want %q, %v", c.on.Format("2006-01-02"), got, ok, err, c.want, c.ok)
		}
	}
}

// What the bank knew when: a setting decided later is not seen by a
// question asked as of earlier; a correction (decided later, effective
// earlier) wins over what it corrects.
func TestResolveIsBitemporal(t *testing.T) {
	p := open(t)
	ctx := context.Background()
	for _, s := range []Setting{
		{gbp.ScopeBank, OwnerBank, "boe.base_rate_bps", "525", d(2026, 1, 1), d(2026, 1, 1), "market"},
		{gbp.ScopeBank, OwnerBank, "boe.base_rate_bps", "500", d(2026, 2, 5), d(2026, 2, 4), "market"}, // announced the day before
		{gbp.ScopeBank, OwnerBank, "boe.base_rate_bps", "475", d(2026, 2, 5), d(2026, 2, 6), "correction"},
	} {
		if err := p.Set(ctx, nil, s); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		on, known time.Time
		want      string
	}{
		{d(2026, 2, 5), d(2026, 2, 3), "525"}, // the change not yet announced
		{d(2026, 2, 5), d(2026, 2, 4), "500"}, // announced
		{d(2026, 2, 5), d(2026, 2, 6), "475"}, // corrected
		{d(2026, 2, 4), d(2026, 2, 6), "525"}, // the day before the change
	} {
		got, _, err := p.Resolve(ctx, nil, gbp.ScopeBank, OwnerBank, "boe.base_rate_bps", c.on, c.known)
		if err != nil || got != c.want {
			t.Errorf("on %s known %s: %q, %v; want %q", c.on.Format("01-02"), c.known.Format("01-02"), got, err, c.want)
		}
	}
}

// Settings are kept apart by scope and owner.
func TestResolveIsScoped(t *testing.T) {
	p := open(t)
	ctx := context.Background()
	if err := p.Set(ctx, nil, Setting{gbp.ScopeAccount, "acct-1", "deposited", "100", d(2026, 1, 1), d(2026, 1, 1), "rule"}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := p.Resolve(ctx, nil, gbp.ScopeAccount, "acct-2", "deposited", d(2026, 1, 1), d(2026, 1, 1)); ok {
		t.Error("another account's setting resolved")
	}
	if _, ok, _ := p.Resolve(ctx, nil, gbp.ScopeVersion, "acct-1", "deposited", d(2026, 1, 1), d(2026, 1, 1)); ok {
		t.Error("a setting resolved at the wrong scope")
	}
}

type stub struct{ gbp.Rules }

func (stub) Product() string               { return "easy-access" }
func (stub) Version() int                  { return 1 }
func (stub) Name() string                  { return "" }
func (stub) Family() gbp.ProductFamily     { return gbp.FamilySavings }
func (stub) Parameters() []gbp.Declaration { return nil }
