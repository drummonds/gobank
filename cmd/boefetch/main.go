// Command boefetch downloads BoE base rate history from the Bank of England
// IADB API and writes it as CSV. Falls back to hardcoded data if the API
// is unavailable.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// fallbackData contains hardcoded BoE base rate changes (2020-01-01 to 2025-12-18).
const fallbackData = `DATE,VALUE
11/Mar/2020,0.25
19/Mar/2020,0.10
16/Dec/2021,0.25
03/Feb/2022,0.50
17/Mar/2022,0.75
05/May/2022,1.00
16/Jun/2022,1.25
04/Aug/2022,1.75
22/Sep/2022,2.25
03/Nov/2022,3.00
15/Dec/2022,3.50
02/Feb/2023,4.00
23/Mar/2023,4.25
11/May/2023,4.50
22/Jun/2023,5.00
03/Aug/2023,5.25
01/Aug/2024,5.00
07/Nov/2024,4.75
06/Feb/2025,4.50
08/May/2025,4.25
19/Jun/2025,4.00
07/Aug/2025,3.75
`

type rateEntry struct {
	date time.Time
	rate float64
}

func main() {
	outFile := flag.String("o", "boe_rates.csv", "output CSV file path")
	flag.Parse()

	entries, err := fetchFromAPI()
	if err != nil {
		fmt.Fprintf(os.Stderr, "API fetch failed (%v), using fallback data\n", err)
		entries = parseFallback()
	}

	// Prepend the initial rate at 2020-01-01 (0.75%)
	start := rateEntry{
		date: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		rate: 0.75,
	}
	entries = append([]rateEntry{start}, entries...)

	// Sort by date
	sort.Slice(entries, func(i, j int) bool { return entries[i].date.Before(entries[j].date) })

	// Deduplicate by date
	var deduped []rateEntry
	seen := map[string]bool{}
	for _, e := range entries {
		key := e.date.Format("2006-01-02")
		if !seen[key] {
			seen[key] = true
			deduped = append(deduped, e)
		}
	}

	// Write CSV
	var sb strings.Builder
	for _, e := range deduped {
		fmt.Fprintf(&sb, "%s,%.4f\n", e.date.Format("2006-01-02"), e.rate/100.0)
	}

	if err := os.WriteFile(*outFile, []byte(sb.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write %s: %v\n", *outFile, err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %d rate entries to %s\n", len(deduped), *outFile)
}

func fetchFromAPI() ([]rateEntry, error) {
	url := "https://www.bankofengland.co.uk/boeapps/database/_iadb-fromshowcolumns.asp?SeriesCodes=IUDBEDR&CSVF=TN&Datefrom=01/Jan/2020&Dateto=01/Mar/2026&UsingCodes=Y&VPD=Y"

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseAPIResponse(string(body))
}

func parseAPIResponse(body string) ([]rateEntry, error) {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	var entries []rateEntry
	for i, line := range lines {
		if i == 0 {
			continue // skip header
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ",", 2)
		if len(parts) != 2 {
			continue
		}
		dateStr := strings.TrimSpace(parts[0])
		rateStr := strings.TrimSpace(parts[1])

		t, err := time.Parse("02/Jan/2006", dateStr)
		if err != nil {
			// Try alternative format
			t, err = time.Parse("02 Jan 2006", dateStr)
			if err != nil {
				continue
			}
		}

		var rate float64
		_, err = fmt.Sscanf(rateStr, "%f", &rate)
		if err != nil {
			continue
		}

		entries = append(entries, rateEntry{date: t, rate: rate})
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no valid entries parsed from API response")
	}
	return entries, nil
}

func parseFallback() []rateEntry {
	entries, _ := parseAPIResponse(fallbackData)
	return entries
}
