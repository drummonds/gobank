package parameters

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"fmt"
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
		{gbp.ScopeVersion, "easy-access/1", "rate_bps", "150", d(2026, 1, 1), decided, "migration", ""},
		{gbp.ScopeVersion, "easy-access/1", "rate_bps", "135", d(2026, 2, 1), decided, "treasury", ""},
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
		{gbp.ScopeBank, OwnerBank, "boe.base_rate_bps", "525", d(2026, 1, 1), d(2026, 1, 1), "market", ""},
		{gbp.ScopeBank, OwnerBank, "boe.base_rate_bps", "500", d(2026, 2, 5), d(2026, 2, 4), "market", ""}, // announced the day before
		{gbp.ScopeBank, OwnerBank, "boe.base_rate_bps", "475", d(2026, 2, 5), d(2026, 2, 6), "correction", ""},
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
	if err := p.Set(ctx, nil, Setting{gbp.ScopeAccount, "acct-1", "deposited", "100", d(2026, 1, 1), d(2026, 1, 1), "rule", ""}); err != nil {
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

// A setting is effective from an instant, not a day (ADR-0009): the Bank
// of England publishes at noon. A reader asking for a day's value asks at
// the end of that day, so a noon publication counts for the whole of its
// day and for none of the day before.
func TestResolveAtAnInstant(t *testing.T) {
	p := open(t)
	ctx := context.Background()
	noon := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	for _, s := range []Setting{
		{gbp.ScopeMarket, "GBP", "risk_free_rate_bps", "450", d(2026, 1, 1), d(2026, 1, 1), "Bank of England", ""},
		{gbp.ScopeMarket, "GBP", "risk_free_rate_bps", "425", noon, noon, "Bank of England", ""},
	} {
		if err := p.Set(ctx, nil, s); err != nil {
			t.Fatal(err)
		}
	}
	known := d(2026, 6, 1)
	for _, c := range []struct {
		name string
		at   time.Time
		want string
	}{
		{"the end of the day before", EndOfDay(d(2026, 3, 18)), "450"},
		{"the morning of the day", noon.Add(-time.Hour), "450"},
		{"the instant published", noon, "425"},
		{"the end of the day", EndOfDay(d(2026, 3, 19)), "425"},
	} {
		got, ok, err := p.Resolve(ctx, nil, gbp.ScopeMarket, "GBP", "risk_free_rate_bps", c.at, known)
		if err != nil || !ok || got != c.want {
			t.Errorf("%s (%s): %q, %v, %v; want %q", c.name, c.at.Format(time.RFC3339), got, ok, err, c.want)
		}
	}
	if end := EndOfDay(noon); !end.Before(d(2026, 3, 20)) || !end.After(noon) {
		t.Errorf("EndOfDay(%v) = %v, want inside the day after noon", noon, end)
	}
}

// What is upcoming is every setting effective after an instant, soonest
// first: decided and not yet in force, so the simulation's timeline can
// carry each as an event (ADR-0010).
func TestUpcomingIsWhatIsEffectiveAhead(t *testing.T) {
	p := open(t)
	ctx := context.Background()
	for _, s := range []Setting{
		{gbp.ScopeVersion, "easy-access", "rate_bps", "150", d(2026, 1, 1), d(2026, 1, 1), "migration", ""},
		{gbp.ScopeVersion, "easy-access", "rate_bps", "175", d(2026, 3, 1), d(2026, 2, 1), "alice", ""},
		{gbp.ScopeBank, "bank", "spread_bps", "50", d(2026, 2, 15), d(2026, 2, 1), "bob", ""},
		{gbp.ScopeAccount, "acct-1", "maturity_day", "2026-02-10", d(2026, 2, 10), d(2026, 1, 5), "rule", ""},
	} {
		if err := p.Set(ctx, nil, s); err != nil {
			t.Fatal(err)
		}
	}
	got, err := p.Upcoming(ctx, nil, d(2026, 2, 1))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Owner != "acct-1" || got[1].Key != "spread_bps" || got[2].Value != "175" || got[2].By != "alice" {
		t.Errorf("Upcoming after 1 Feb = %+v; want the maturity, the spread then the rate", got)
	}
	if got, _ := p.Upcoming(ctx, nil, d(2026, 2, 15)); len(got) != 1 || got[0].Value != "175" {
		t.Errorf("Upcoming after 15 Feb = %+v; want only the rate (a setting effective at the instant is in force)", got)
	}
	if got, _ := p.Upcoming(ctx, nil, d(2026, 3, 1)); len(got) != 0 {
		t.Errorf("Upcoming after 1 Mar = %+v; want nothing", got)
	}
}

// A setting carries the root event it was written under (ADR-0011): its
// cause, on the upcoming list and counted by cause. One from before the
// record has none until it is stamped.
func TestASettingCarriesItsCause(t *testing.T) {
	p := open(t)
	ctx := context.Background()
	decided := d(2026, 1, 10)
	caused := Setting{gbp.ScopeVersion, "easy-access", "rate_bps", "175", d(2026, 3, 1), decided, "alice", causeID("root-1")}
	orphan := Setting{gbp.ScopeBank, OwnerBank, "spread_bps", "50", d(2026, 2, 15), decided, "bob", ""}
	for _, s := range []Setting{caused, orphan} {
		if err := p.Set(ctx, nil, s); err != nil {
			t.Fatal(err)
		}
	}
	up, err := p.Upcoming(ctx, nil, decided)
	if err != nil || len(up) != 2 || up[0].Cause != "" || up[1].Cause != causeID("root-1") {
		t.Errorf("Upcoming = %+v, %v; want the orphan first with no cause, then alice's under root-1", up, err)
	}
	if n, err := p.CountByCause(ctx, causeID("root-1")); err != nil || n != 1 {
		t.Errorf("CountByCause(root-1) = %d, %v; want 1", n, err)
	}
	if err := p.SetCause(ctx, nil, orphan, causeID("root-2")); err != nil {
		t.Fatal(err)
	}
	up, _ = p.Upcoming(ctx, nil, decided)
	if len(up) != 2 || up[0].Cause != causeID("root-2") {
		t.Errorf("after stamping: %+v; want the orphan under root-2", up)
	}
	if n, _ := p.CountByCause(ctx, causeID("root-2")); n != 1 {
		t.Errorf("CountByCause(root-2) = %d, want 1", n)
	}
}

// causeID is a root event id for a test, a UUID made from a name so the
// test can read as the name while the column holds a UUID (#66).
func causeID(name string) string {
	h := sha1.Sum([]byte(name))
	h[6], h[8] = h[6]&0x0f|0x50, h[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}
