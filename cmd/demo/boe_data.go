package main

import (
	_ "embed"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed boe_rates.csv
var boeRatesCSV string

// boeRateHistory is the parsed historical rate data, populated by init.
var boeRateHistory []RatePoint

func init() {
	boeRateHistory = parseBoeRates(boeRatesCSV)
}

func parseBoeRates(csv string) []RatePoint {
	var pts []RatePoint
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
		pts = append(pts, RatePoint{Date: t, Rate: rate})
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].Date.Before(pts[j].Date) })
	return pts
}

// lookupBoERate returns the BoE base rate in effect on the given day
// by finding the most recent rate change on or before that day.
func lookupBoERate(day time.Time) float64 {
	if len(boeRateHistory) == 0 {
		return 0.0525 // fallback
	}
	// Binary search: find last entry with Date <= day
	i := sort.Search(len(boeRateHistory), func(i int) bool {
		return boeRateHistory[i].Date.After(day)
	})
	if i == 0 {
		return boeRateHistory[0].Rate
	}
	return boeRateHistory[i-1].Rate
}
