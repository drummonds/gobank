package main

import (
	"fmt"
	"log"
	"strings"

	luca "git.bytestone.uk/hum3/go-luca"
)

// The ledger component is go-luca: its tables are the books of account
// and it publishes its own contract views (contract_ledger_movements,
// contract_ledger_eod_positions, contract_ledger_live_positions) when its
// schema runs. This file is the demo's reading of those views.

// ledgerExponent is the exponent of every account the demo opens: GBP in
// pence. The position views publish money in major units at the
// commodity's exponent, so reading them back needs it.
const ledgerExponent = -2

// positionScale is the decimal places the views give accrued interest.
const positionScale = 7

// livePosition is an account's position as the ledger publishes it now:
// the latest projection plus the movements since.
type livePosition struct {
	balance   luca.Amount // minor units
	accruedE7 int64       // ten-millionths of the major unit
}

// livePosition reads one account from contract_ledger_live_positions.
// ok is false when there is no database or the account has no row.
func (ds *DemoState) livePosition(accountID string) (livePosition, bool) {
	if ds.db == nil || accountID == "" {
		return livePosition{}, false
	}
	var balance, accrued string
	err := ds.db.QueryRow(`SELECT balance, accrued FROM contract_ledger_live_positions WHERE account_id = $1`, accountID).
		Scan(&balance, &accrued)
	if err != nil {
		log.Printf("livePosition %s: %v", accountID, err)
		return livePosition{}, false
	}
	minor, err := parseScaled(balance, -ledgerExponent)
	if err != nil {
		log.Printf("livePosition %s: balance %q: %v", accountID, balance, err)
		return livePosition{}, false
	}
	e7, err := parseScaled(accrued, positionScale)
	if err != nil {
		log.Printf("livePosition %s: accrued %q: %v", accountID, accrued, err)
		return livePosition{}, false
	}
	return livePosition{balance: luca.Amount(minor), accruedE7: e7}, true
}

// parseScaled reads a NUMERIC rendered as text ("1001.27", "-0.4109589")
// as an integer at scale decimal places, exactly: no floating point, and
// an error if the text carries more non-zero places than scale.
func parseScaled(s string, scale int) (int64, error) {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	whole, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		whole, frac = s[:i], s[i+1:]
	}
	if len(frac) > scale {
		if strings.Trim(frac[scale:], "0") != "" {
			return 0, fmt.Errorf("more than %d decimal places", scale)
		}
		frac = frac[:scale]
	}
	frac += strings.Repeat("0", scale-len(frac))
	digits := strings.TrimLeft(whole+frac, "0")
	if digits == "" {
		return 0, nil
	}
	var n int64
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int64(c-'0')
	}
	if neg {
		n = -n
	}
	return n, nil
}
