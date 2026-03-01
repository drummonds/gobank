package main

import (
	"fmt"
	"strings"
)

// fmtMoney formats a float as £1,234,567.89 with comma separators.
func fmtMoney(v float64) string {
	negative := v < 0
	if negative {
		v = -v
	}
	whole := int64(v)
	frac := v - float64(whole)

	// Format integer part with commas
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

	prefix := "£"
	if negative {
		prefix = "-£"
	}
	return fmt.Sprintf("%s%s.%02d", prefix, s, int64(frac*100+0.5))
}
