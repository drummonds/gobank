// Package policy is the Model Bank's policy, version 1 (ADR-0006, the
// bank's rates; ADR-0007): how its savings and lending base rates follow
// the pound's risk-free rate. Each is the market's rate with a spread, and
// the bank does not pass a change straight through: a cut reaches savers
// at once and a rise after a lag, borrowers the other way about. The
// spreads and lags are the bank's settings; the taking is this code.
// Immutable once adopted: a change to the rule is version 2.
package policy

import (
	"strconv"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
)

// The bank's settings the policy publishes: each may be set from a day.
const (
	SavingsSpreadKey = "savings_spread_bps"
	SavingsLagKey    = "savings_lag_days"
	LendingSpreadKey = "lending_spread_bps"
	LendingLagKey    = "lending_lag_days"
)

// Version is the Model Bank's policy v1.
var Version gbp.Policy = version{}

type version struct{}

func (version) Name() string { return "Model Bank policy" }
func (version) Version() int { return 1 }

func (version) Parameters() []gbp.Declaration {
	return []gbp.Declaration{
		{Key: gbp.RiskFreeRateKey, Scope: gbp.ScopeMarket, Kind: gbp.KindBps, Description: "the pound's risk-free rate, as its market publishes it"},
		{Key: SavingsSpreadKey, Scope: gbp.ScopeBank, Kind: gbp.KindBps, Published: "-50", Description: "the savings rate's spread over the risk-free rate"},
		{Key: SavingsLagKey, Scope: gbp.ScopeBank, Kind: gbp.KindInt, Published: "14", Description: "the days a rise in the risk-free rate takes to reach savers; a cut reaches them at once"},
		{Key: LendingSpreadKey, Scope: gbp.ScopeBank, Kind: gbp.KindBps, Published: "300", Description: "the lending rate's spread over the risk-free rate"},
		{Key: LendingLagKey, Scope: gbp.ScopeBank, Kind: gbp.KindInt, Published: "14", Description: "the days a cut in the risk-free rate takes to reach borrowers; a rise reaches them at once"},
		{Key: gbp.SavingsRateKey, Scope: gbp.ScopeBank, Kind: gbp.KindBps, Algorithm: lagged(SavingsSpreadKey, SavingsLagKey, lower),
			Description: "the bank's savings rate: the lower of the risk-free rate on the day and savings_lag_days earlier, plus savings_spread_bps"},
		{Key: gbp.LendingRateKey, Scope: gbp.ScopeBank, Kind: gbp.KindBps, Algorithm: lagged(LendingSpreadKey, LendingLagKey, higher),
			Description: "the bank's lending rate: the higher of the risk-free rate on the day and lending_lag_days earlier, plus lending_spread_bps"},
	}
}

// StartUp checks every parameter resolves: the market must have spoken.
func (v version) StartUp(p gbp.ParameterReader, day time.Time) error {
	return gbp.CheckPolicy(v, p, day)
}

func lower(a, b int64) int64  { return min(a, b) }
func higher(a, b int64) int64 { return max(a, b) }

// lagged is the lag and take: the risk-free rate on the day and the lag's
// days earlier, the one take chooses, plus the spread. Before the market
// has published for a lag's length the series is taken as its first
// value: the earliest publication inside the lag stands for the days
// before it, so a change soon after the bank opens is lagged as any
// other; with no publication at all the rate on the day stands alone.
func lagged(spreadKey, lagKey string, take func(a, b int64) int64) gbp.Algorithm {
	return func(p gbp.ParameterReader, day time.Time) (string, error) {
		now, err := p.Parameter(gbp.RiskFreeRateKey, day)
		if err != nil {
			return "", err
		}
		lag, err := p.Parameter(lagKey, day)
		if err != nil {
			return "", err
		}
		spread, err := p.Parameter(spreadKey, day)
		if err != nil {
			return "", err
		}
		rate := now.Bps()
		for back := int(lag.Int()); back > 0; back-- {
			if then, err := p.Parameter(gbp.RiskFreeRateKey, day.AddDate(0, 0, -back)); err == nil {
				rate = take(now.Bps(), then.Bps())
				break
			}
		}
		return strconv.FormatInt(rate+spread.Bps(), 10), nil
	}
}
