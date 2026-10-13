package policy_test

import (
	"testing"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank-products/testkit"
	policy "git.bytestone.uk/hum3/gobank/modelbank/policy/v1"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// reader is the policy over a market that published 5.00% in 2025, cut
// to 4.00% on 10 January 2026 and raised to 4.50% on 10 February.
func reader(t *testing.T) *testkit.Reader {
	t.Helper()
	r := testkit.NewReader(policy.Version.Name(), policy.Version.Parameters())
	r.Set(gbp.ScopeMarket, gbp.RiskFreeRateKey, "500", day(2025, 1, 1))
	r.Set(gbp.ScopeMarket, gbp.RiskFreeRateKey, "400", day(2026, 1, 10))
	r.Set(gbp.ScopeMarket, gbp.RiskFreeRateKey, "450", day(2026, 2, 10))
	if err := policy.Version.StartUp(r, day(2026, 1, 1)); err != nil {
		t.Fatalf("start-up: %v", err)
	}
	return r
}

// The lag and take (ADR-0006, the bank's rates): a cut reaches savers at
// once and a rise after the lag; borrowers the other way about. The
// published spreads are 50 bps under the market for savings and 300
// over it for lending, the lag 14 days for both.
func TestACutReachesSaversAtOnceAndARiseAfterTheLag(t *testing.T) {
	r := reader(t)
	cases := []struct {
		on               time.Time
		savings, lending int64
	}{
		{day(2026, 1, 5), 450, 800},  // 500 − 50; 500 + 300
		{day(2026, 1, 10), 350, 800}, // the cut: savers at once, borrowers not yet
		{day(2026, 1, 23), 350, 800}, // the last day of the borrowers' lag
		{day(2026, 1, 24), 350, 700}, // the cut reaches borrowers
		{day(2026, 2, 10), 350, 750}, // the rise: borrowers at once, savers not yet
		{day(2026, 2, 23), 350, 750}, // the last day of the savers' lag
		{day(2026, 2, 24), 400, 750}, // the rise reaches savers
	}
	for _, c := range cases {
		for key, want := range map[string]int64{gbp.SavingsRateKey: c.savings, gbp.LendingRateKey: c.lending} {
			p, err := r.Parameter(key, c.on)
			if err != nil || p.Bps() != want {
				t.Errorf("%s on %s = %d, %v; want %d", key, c.on.Format("2006-01-02"), p.Bps(), err, want)
			}
		}
	}
}

// The spread and the lag are the bank's settings: a wider spread from a
// day, or a longer lag, changes the rate from that day with no release.
func TestTheSpreadAndTheLagAreSettings(t *testing.T) {
	r := reader(t)
	r.Set(gbp.ScopeBank, policy.SavingsSpreadKey, "-100", day(2026, 3, 1))
	r.Set(gbp.ScopeBank, policy.LendingLagKey, "30", day(2026, 1, 1))
	if p, _ := r.Parameter(gbp.SavingsRateKey, day(2026, 2, 28)); p.Bps() != 400 {
		t.Errorf("savings the day before the spread widens = %d, want 400", p.Bps())
	}
	if p, _ := r.Parameter(gbp.SavingsRateKey, day(2026, 3, 1)); p.Bps() != 350 {
		t.Errorf("savings from the wider spread = %d, want 350", p.Bps())
	}
	if p, _ := r.Parameter(gbp.LendingRateKey, day(2026, 2, 8)); p.Bps() != 800 {
		t.Errorf("lending with a 30-day lag on 8 Feb = %d, want 800: the cut of 10 Jan has not reached borrowers", p.Bps())
	}
	if p, _ := r.Parameter(gbp.LendingRateKey, day(2026, 2, 9)); p.Bps() != 700 {
		t.Errorf("lending with a 30-day lag on 9 Feb = %d, want 700", p.Bps())
	}
}

// Before the market has been publishing for a lag's length the series is
// taken as its first value, so a change soon after the first publication
// is lagged as any other; with no publication at all the policy cannot
// start.
func TestBeforeTheFirstPublicationTheSeriesIsItsFirstValue(t *testing.T) {
	r := reader(t)
	if p, err := r.Parameter(gbp.SavingsRateKey, day(2025, 1, 5)); err != nil || p.Bps() != 450 {
		t.Errorf("savings four days after the first publication = %d, %v; want 450", p.Bps(), err)
	}
	young := testkit.NewReader(policy.Version.Name(), policy.Version.Parameters())
	young.Set(gbp.ScopeMarket, gbp.RiskFreeRateKey, "500", day(2026, 1, 1))
	young.Set(gbp.ScopeMarket, gbp.RiskFreeRateKey, "400", day(2026, 1, 3)) // a cut two days in
	for _, c := range []struct {
		on               time.Time
		savings, lending int64
	}{{day(2026, 1, 3), 350, 800}, {day(2026, 1, 16), 350, 800}, {day(2026, 1, 17), 350, 700}} {
		for key, want := range map[string]int64{gbp.SavingsRateKey: c.savings, gbp.LendingRateKey: c.lending} {
			if p, err := young.Parameter(key, c.on); err != nil || p.Bps() != want {
				t.Errorf("%s on %s = %d, %v; want %d", key, c.on.Format("2006-01-02"), p.Bps(), err, want)
			}
		}
	}
	silent := testkit.NewReader(policy.Version.Name(), policy.Version.Parameters())
	if err := policy.Version.StartUp(silent, day(2026, 1, 1)); err == nil {
		t.Error("the policy started with the market silent")
	}
}

func TestDeclarations(t *testing.T) {
	if policy.Version.Name() == "" || policy.Version.Version() != 1 {
		t.Errorf("identity %q v%d", policy.Version.Name(), policy.Version.Version())
	}
	derived := map[string]bool{}
	for _, d := range policy.Version.Parameters() {
		derived[d.Key] = d.IsDerived()
		if d.Scope == gbp.ScopeBank && !d.IsDerived() && d.Published == "" {
			t.Errorf("%s: a bank parameter the policy does not derive publishes a value", d.Key)
		}
		if d.Scope == gbp.ScopeMarket && d.Key != gbp.RiskFreeRateKey {
			t.Errorf("%s: the policy reads the market's risk-free rate alone", d.Key)
		}
	}
	for _, key := range []string{gbp.SavingsRateKey, gbp.LendingRateKey} {
		if !derived[key] {
			t.Errorf("%s is the policy's to derive, not to set", key)
		}
	}
}
