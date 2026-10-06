package sim

import (
	_ "embed"
	"sort"
	"strconv"
	"strings"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
)

// The Bank of England base rate as the simulation replays it (ADR-0002
// stage 4): market data the bank reads through core.BaseRateSource, which
// in a real deployment is a feed. The series is historical, fetched by
// cmd/boefetch into boe_rates.csv.

//go:embed boe_rates.csv
var boeRatesCSV string

// RateSeries is a base-rate history, oldest first: a core.BaseRateSource.
type RateSeries []core.RatePoint

// BoERates is the historical series, parsed at start.
var BoERates RateSeries

func init() {
	BoERates = parseBoeRates(boeRatesCSV)
}

func parseBoeRates(csv string) RateSeries {
	var pts RateSeries
	for line := range strings.SplitSeq(strings.TrimSpace(csv), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ",", 2)
		if len(parts) != 2 {
			continue
		}
		t, err := time.Parse("2006-01-02", parts[0])
		if err != nil {
			continue
		}
		rate, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			continue
		}
		pts = append(pts, core.RatePoint{Date: t, Rate: rate})
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].Date.Before(pts[j].Date) })
	return pts
}

// BaseRate implements core.BaseRateSource: the rate in effect on day is
// the most recent change on or before it.
func (s RateSeries) BaseRate(day time.Time) float64 {
	if len(s) == 0 {
		return 0.0525 // fallback
	}
	i := sort.Search(len(s), func(i int) bool { return s[i].Date.After(day) })
	if i == 0 {
		return s[0].Rate
	}
	return s[i-1].Rate
}
