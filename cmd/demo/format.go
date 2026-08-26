package main

import (
	"fmt"
	"strings"

	luca "git.bytestone.uk/hum3/go-luca"
)

// groupThousands inserts comma separators into a string of decimal digits.
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

// fmtMoney formats an amount in minor units (pence) as £1,234,567.89 with
// comma separators. Money is stored as integer minor units throughout the
// demo — float64 is prohibited for money storage.
func fmtMoney(v luca.Amount) string {
	prefix := "£"
	if v < 0 {
		prefix = "-£"
		v = -v
	}
	return fmt.Sprintf("%s%s.%02d", prefix, groupThousands(fmt.Sprintf("%d", v/100)), v%100)
}

// poundsE7 is a fixed-point amount in ten-millionths of a pound (7 decimal
// places), the demo's model for accrued interest below one penny. Integer
// arithmetic only — float64 is prohibited for money storage.
type poundsE7 int64

// poundsE7PerPenny is the number of 7dp-pound units in one penny.
const poundsE7PerPenny = 100_000

// accrualPoundsE7 converts an engine accrual numerator (minor units over
// gbp.AccrualDenominator = 3,650,000) into 7dp pounds, rounding to nearest:
// pounds_e7 = n * 10^7 / 365,000,000 = 2n/73.
func accrualPoundsE7(n int64) poundsE7 {
	if n < 0 {
		return -accrualPoundsE7(-n)
	}
	return poundsE7((2*n + 36) / 73) // +36 rounds to nearest (half is 36.5)
}

// Pence converts 7dp pounds to whole pence, truncating toward zero — the same
// conversion the interest engine performs when accrued amounts are applied as
// postable money (whole pence move, the remainder keeps accruing).
func (p poundsE7) Pence() luca.Amount {
	return luca.Amount(int64(p) / poundsE7PerPenny)
}

// String formats 7dp pounds as £1,234.0123456.
func (p poundsE7) String() string {
	prefix := "£"
	if p < 0 {
		prefix = "-£"
		p = -p
	}
	return fmt.Sprintf("%s%s.%07d", prefix, groupThousands(fmt.Sprintf("%d", int64(p)/10_000_000)), int64(p)%10_000_000)
}
