# The Model Bank

The Model Bank's definition (ADR-0007): what makes gobank's capabilities
this bank, as code. Another bank is another definition over the same
capabilities. The package is `modelbank`; its policy is `modelbank/policy/v1`.

## What a definition says

| Part | Where | Changed by |
|---|---|---|
| The bank's name | `Definition().Name` | a release of the definition |
| The products it opens with, each on a rules version it carries | `Definition().Products` | a release lists a new product or withdraws one; staff set a product's parameters |
| Its policy: how its own rates follow the market | `Definition().Policy` | a release for the rule; staff set the spreads and lags |

## Products

| Product | Rules | Rate | Notes |
|---|---|---|---|
| `easy-access` | `easyaccess/v1` | 1.50% published | instant access |
| `easy-access-tracker` | `easyaccess/v2` | the bank's savings rate − 15 bps, floor 0 | tracks the policy |
| `fixed-term` | `fixedterm/v1` | 4.00% published | 24 months from the open day |
| `isa` | `isa/v1` | 3.50% published | deposits capped at the allowance |
| `personal-loan` | `personalloan/v1` | 6.90% published | |
| `mortgage` | `mortgage/v1` | 4.50% published | |
| `overdraft` | `overdraft/v1` | 15.90% published | movements refused past the limit |

## Policy v1: the lag and take

The bank's rates derive from the pound's risk-free rate (ADR-0009) by
algorithm (ADR-0006 as amended). The spreads and lags are parameters at
the bank scope, published here and settable from the Bank tab.

| Parameter | Published | Meaning |
|---|---|---|
| `savings_spread_bps` | −50 | the savings rate's spread over the risk-free rate |
| `savings_lag_days` | 14 | days a rise takes to reach savers |
| `lending_spread_bps` | 300 | the lending rate's spread over the risk-free rate |
| `lending_lag_days` | 14 | days a cut takes to reach borrowers |
| `savings_rate_bps` | derived | min(rate on the day, rate `savings_lag_days` earlier) + `savings_spread_bps` |
| `lending_rate_bps` | derived | max(rate on the day, rate `lending_lag_days` earlier) + `lending_spread_bps` |

| Market moved | Savers see | Borrowers see |
|---|---|---|
| down on day d | the new rate from d | the old rate until d + lag, then the new |
| up on day d | the old rate until d + lag, then the new | the new rate from d |
| before the market has published for a lag's length | the series is taken as its first value: a change inside the first lag is lagged as any other | the same |

The live picture is the Bank tab of the running demo: the risk-free
rate against the bank's savings and lending rates over the run, the
lag visible as the bank's line moving after the market's.
