package main

import (
	"fmt"
	"strings"

	luca "git.bytestone.uk/hum3/go-luca"
)

// fmtMoney formats an amount in minor units (pence) as £1,234,567.89 with
// comma separators. Money is stored as integer minor units throughout the
// demo — float64 is prohibited for money storage.
func fmtMoney(v luca.Amount) string {
	prefix := "£"
	if v < 0 {
		prefix = "-£"
		v = -v
	}
	whole := v / 100
	frac := v % 100

	s := fmt.Sprintf("%d", whole)
	if len(s) > 3 {
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
		s = b.String()
	}
	return fmt.Sprintf("%s%s.%02d", prefix, s, frac)
}
