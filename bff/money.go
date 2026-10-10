package bff

import (
	"fmt"
	"strings"

	luca "git.bytestone.uk/hum3/go-luca"
)

// FormatMoney formats minor units as £1,234,567.89. Negative amounts render
// as -£1.00. This is the only place money becomes text for the clients.
func FormatMoney(v luca.Amount) string {
	prefix := "£"
	if v < 0 {
		prefix = "-£"
		v = -v
	}
	return fmt.Sprintf("%s%s.%02d", prefix, groupThousands(fmt.Sprintf("%d", v/100)), v%100)
}

func groupThousands(s string) string {
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	offset := len(s) % 3
	if offset > 0 {
		b.WriteString(s[:offset])
	}
	for i := offset; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// FormatRateBps renders an annual rate held in basis points as a
// percentage to two places: 150 is "1.50%".
func FormatRateBps(bps int64) string {
	sign := ""
	if bps < 0 {
		sign, bps = "-", -bps
	}
	return fmt.Sprintf("%s%d.%02d%%", sign, bps/100, bps%100)
}
