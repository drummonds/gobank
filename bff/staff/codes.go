package staff

import "strings"

// A movement's code is an ISO 20022 bank transaction code (the external
// BankTransactionCode code set): a domain, a family and a sub-family,
// each four letters, written "PMNT:RCDT:BOOK" as go-luca holds it. The
// names here are the code set's, for the codes the bank writes.
var codeNames = map[string]string{
	// Domains
	"PMNT": "Payments",
	"LDAS": "Loans, Deposits and Syndications",
	"ACMT": "Account Management",
	// Families
	"RCDT": "Received Credit Transfers",
	"ICDT": "Issued Credit Transfers",
	"FTDP": "Fixed Term Deposits",
	"MDOP": "Miscellaneous Debit Operations",
	"MCOP": "Miscellaneous Credit Operations",
	// Sub-families
	"BOOK": "Internal Book Transfer",
	"DMCT": "Domestic Credit Transfer",
	"INTR": "Interest",
	"FEES": "Fees",
	"OTHR": "Other",
}

// DescribeCode expands a bank transaction code into its domain, family
// and sub-family names, "Payments / Received Credit Transfers / Internal
// Book Transfer"; a part the set does not name is kept as written.
func DescribeCode(code string) string {
	if code == "" {
		return ""
	}
	parts := strings.Split(code, ":")
	for i, p := range parts {
		if name, ok := codeNames[p]; ok {
			parts[i] = name
		}
	}
	return strings.Join(parts, " / ")
}
