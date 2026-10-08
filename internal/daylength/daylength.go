// Package daylength reads the console's day-length setting: how long a
// simulated day takes on the wall clock, as GOBANK_DAY_LENGTH, the run
// row or the settings page give it.
package daylength

import (
	"fmt"
	"strings"
	"time"
)

// Parse reads a simulated day's wall-clock length: a Go duration such as
// "2h" or "90m". Empty or "0" is flat out.
func Parse(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("day length %q: want a duration like 2h or 90m, or 0 for flat out", s)
	}
	return d, nil
}
