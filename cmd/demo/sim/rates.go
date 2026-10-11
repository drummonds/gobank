package sim

import (
	_ "embed"
	"sort"
	"strconv"
	"strings"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
)

// The Bank of England's Bank Rate as the simulation replays it (ADR-0009):
// the pound's market, publishing at noon London time on a decision day.
// The series is historical, fetched by cmd/boefetch from the market's
// feed into boe_rates.csv; the simulation is the feed, publishing each
// change through the core when its clock reaches it. What the feed knows
// of the market, the currency and the publication time, it knows as a
// feed does; the bank's own record of the source is bank/market.

const currency = "GBP"

// london is where the Bank publishes; a missing zone database falls back
// to UTC, an hour out in summer and no worse.
var london = func() *time.Location {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// publishedAt is noon London time on the decision day.
func publishedAt(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, london)
}

//go:embed boe_rates.csv
var boeRatesCSV string

// Publications is a market's history, oldest first.
type Publications []core.Publication

// BoERates is the historical series, parsed at start.
var BoERates Publications

func init() {
	BoERates = parsePublications(boeRatesCSV)
}

// parsePublications reads "date,decimal rate" lines as GBP publications at
// the market's publication time on the day, in basis points.
func parsePublications(csv string) Publications {
	var pubs Publications
	for line := range strings.SplitSeq(strings.TrimSpace(csv), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ",", 2)
		if len(parts) != 2 {
			continue
		}
		day, err := time.Parse("2006-01-02", parts[0])
		if err != nil {
			continue
		}
		rate, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			continue
		}
		pubs = append(pubs, core.Publication{Currency: currency, Bps: int64(rate*10000 + 0.5), At: publishedAt(day)})
	}
	sort.Slice(pubs, func(i, j int) bool { return pubs[i].At.Before(pubs[j].At) })
	return pubs
}

// InForce is the rate the last publication at or before the instant set;
// false before the series begins.
func (p Publications) InForce(at time.Time) (int64, bool) {
	i := sort.Search(len(p), func(i int) bool { return p[i].At.After(at) })
	if i == 0 {
		return 0, false
	}
	return p[i-1].Bps, true
}
