package staff

import "testing"

// A movement's code is an ISO 20022 bank transaction code, domain,
// family and sub-family; the page expands each part it knows and keeps
// one it does not as written.
func TestDescribeCode(t *testing.T) {
	for code, want := range map[string]string{
		"PMNT:RCDT:BOOK": "Payments / Received Credit Transfers / Internal Book Transfer",
		"PMNT:ICDT:DMCT": "Payments / Issued Credit Transfers / Domestic Credit Transfer",
		"LDAS:FTDP:INTR": "Loans, Deposits and Syndications / Fixed Term Deposits / Interest",
		"ACMT:MDOP:FEES": "Account Management / Miscellaneous Debit Operations / Fees",
		"ACMT:MCOP:OTHR": "Account Management / Miscellaneous Credit Operations / Other",
		"LDAS:FTDP:ACRU": "Loans, Deposits and Syndications / Fixed Term Deposits / ACRU",
		"XXXX":           "XXXX",
		"":               "",
	} {
		if got := DescribeCode(code); got != want {
			t.Errorf("DescribeCode(%q) = %q, want %q", code, got, want)
		}
	}
}
