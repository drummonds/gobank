package main

import (
	"fmt"
	"strings"
)

// BuildPnLHTML renders a Profit & Loss statement derived from current state.
func (ds *DemoState) BuildPnLHTML() string {
	ds.mu.Lock()
	dayCount := ds.dayCount
	opCostPerDay := ds.opCostPerDay

	loanInterestIncome := 0.0
	depositInterestExpense := 0.0
	for _, c := range ds.customers {
		for _, a := range c.Accounts {
			if a.Family == FamilyLending {
				loanInterestIncome += a.Interest
			} else {
				depositInterestExpense += a.Interest
			}
		}
	}
	boeInterestIncome := ds.boeInterestAccum
	ds.mu.Unlock()

	opCosts := opCostPerDay * float64(dayCount)
	netInterest := loanInterestIncome + boeInterestIncome - depositInterestExpense
	profit := netInterest - opCosts

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Profit &amp; Loss</h2>`)
	s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">For %d simulated days</p>`, dayCount))

	s.WriteString(`<div class="box">`)
	s.WriteString(`<table class="table is-fullwidth">`)
	s.WriteString(`<tbody>`)
	s.WriteString(fmt.Sprintf(`<tr><td><strong>Loan Interest Income</strong></td><td class="has-text-right has-text-success-dark">%s</td></tr>`, fmtMoney(loanInterestIncome)))
	s.WriteString(fmt.Sprintf(`<tr><td><strong>BoE Interest Income</strong></td><td class="has-text-right has-text-success-dark">%s</td></tr>`, fmtMoney(boeInterestIncome)))
	s.WriteString(fmt.Sprintf(`<tr><td><strong>Deposit Interest Expense</strong></td><td class="has-text-right has-text-danger">(%s)</td></tr>`, fmtMoney(depositInterestExpense)))
	s.WriteString(fmt.Sprintf(`<tr><td class="has-text-weight-semibold">Net Interest Margin</td><td class="has-text-right has-text-weight-semibold">%s</td></tr>`, fmtMoney(netInterest)))
	s.WriteString(`<tr><td colspan="2"><hr class="my-1"></td></tr>`)
	s.WriteString(fmt.Sprintf(`<tr><td><strong>Operational Costs</strong></td><td class="has-text-right has-text-danger">(%s)</td></tr>`, fmtMoney(opCosts)))
	s.WriteString(`<tr><td colspan="2"><hr class="my-1"></td></tr>`)

	profitClass := "has-text-success-dark"
	if profit < 0 {
		profitClass = "has-text-danger"
	}
	s.WriteString(fmt.Sprintf(`<tr><td class="has-text-weight-bold is-size-5">Net Profit</td><td class="has-text-right has-text-weight-bold is-size-5 %s">%s</td></tr>`, profitClass, fmtMoney(profit)))
	s.WriteString(`</tbody></table></div>`)

	return s.String()
}

// BuildBalanceSheetHTML renders a balance sheet derived from current state,
// including gilt holdings and a regulatory capital (Tier 1) section.
func (ds *DemoState) BuildBalanceSheetHTML() string {
	ds.mu.Lock()
	dayCount := ds.dayCount
	opCostPerDay := ds.opCostPerDay

	totalLoans := 0.0
	totalDeposits := 0.0
	loanInterest := 0.0
	depositInterest := 0.0
	for _, c := range ds.customers {
		for _, a := range c.Accounts {
			if a.Family == FamilyLending {
				totalLoans += a.Balance
				loanInterest += a.Interest
			} else {
				totalDeposits += a.Balance
				depositInterest += a.Interest
			}
		}
	}
	boeInterest := ds.boeInterestAccum
	ds.mu.Unlock()

	// Gilt holdings (DB query, outside lock)
	holdings := ds.getGiltHoldings()
	totalGilts := 0.0
	for _, h := range holdings {
		totalGilts += h.FaceValue
	}

	opCosts := opCostPerDay * float64(dayCount)
	retainedEarnings := (loanInterest + boeInterest - depositInterest) - opCosts
	cashTotal := totalDeposits - totalLoans + retainedEarnings
	cashAtBoE := cashTotal - totalGilts // gilts purchased from cash
	if cashAtBoE < 0 {
		cashAtBoE = 0
	}
	totalAssets := totalLoans + totalGilts + cashAtBoE
	totalLiabilities := totalDeposits
	equity := retainedEarnings

	// Risk-weighted assets (Basel simplified):
	//   Loans = 100% risk weight, Gilts (sovereign) = 0%, Cash at BoE = 0%
	rwa := totalLoans
	cet1Ratio := 0.0
	if rwa > 0 {
		cet1Ratio = equity / rwa
	}

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Balance Sheet</h2>`)
	s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">As at day %d</p>`, dayCount))

	s.WriteString(`<div class="columns">`)

	// Assets
	s.WriteString(`<div class="column"><div class="box">`)
	s.WriteString(`<h3 class="title is-5">Assets</h3>`)
	s.WriteString(`<table class="table is-fullwidth"><tbody>`)
	s.WriteString(fmt.Sprintf(`<tr><td>Loans to Customers</td><td class="has-text-right">%s</td><td class="has-text-right has-text-grey is-size-7">100%% RW</td></tr>`, fmtMoney(totalLoans)))
	if totalGilts > 0 {
		s.WriteString(fmt.Sprintf(`<tr><td>Government Securities (Gilts)</td><td class="has-text-right">%s</td><td class="has-text-right has-text-grey is-size-7">0%% RW</td></tr>`, fmtMoney(totalGilts)))
	}
	s.WriteString(fmt.Sprintf(`<tr><td>Cash &amp; Reserves at BoE</td><td class="has-text-right">%s</td><td class="has-text-right has-text-grey is-size-7">0%% RW</td></tr>`, fmtMoney(cashAtBoE)))
	s.WriteString(`<tr><td colspan="3"><hr class="my-1"></td></tr>`)
	s.WriteString(fmt.Sprintf(`<tr><td class="has-text-weight-bold">Total Assets</td><td class="has-text-right has-text-weight-bold">%s</td><td></td></tr>`, fmtMoney(totalAssets)))
	s.WriteString(`</tbody></table></div></div>`)

	// Liabilities + Equity
	s.WriteString(`<div class="column"><div class="box">`)
	s.WriteString(`<h3 class="title is-5">Liabilities &amp; Equity</h3>`)
	s.WriteString(`<table class="table is-fullwidth"><tbody>`)
	s.WriteString(fmt.Sprintf(`<tr><td>Customer Deposits</td><td class="has-text-right">%s</td></tr>`, fmtMoney(totalLiabilities)))
	eqClass := ""
	if equity < 0 {
		eqClass = ` class="has-text-danger"`
	}
	s.WriteString(fmt.Sprintf(`<tr><td>Retained Earnings (CET1 Capital)</td><td class="has-text-right"%s>%s</td></tr>`, eqClass, fmtMoney(equity)))
	s.WriteString(`<tr><td colspan="2"><hr class="my-1"></td></tr>`)
	s.WriteString(fmt.Sprintf(`<tr><td class="has-text-weight-bold">Total</td><td class="has-text-right has-text-weight-bold">%s</td></tr>`, fmtMoney(totalLiabilities+equity)))
	s.WriteString(`</tbody></table></div></div>`)

	s.WriteString(`</div>`) // end columns

	// Regulatory Capital section
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Regulatory Capital</h3>`)

	// CET1 compliance tag
	const minCET1Ratio = 0.045 // Basel III minimum 4.5%
	if cet1Ratio >= minCET1Ratio {
		s.WriteString(`<span class="tag is-success is-medium mb-3">CET1 Compliant</span>`)
	} else {
		s.WriteString(`<span class="tag is-danger is-medium mb-3">CET1 Non-Compliant</span>`)
	}

	s.WriteString(`<table class="table is-fullwidth"><tbody>`)
	s.WriteString(fmt.Sprintf(`<tr><td><strong>Common Equity Tier 1 (CET1) Capital</strong></td><td class="has-text-right">%s</td></tr>`, fmtMoney(equity)))
	s.WriteString(fmt.Sprintf(`<tr><td>Risk-Weighted Assets (RWA)</td><td class="has-text-right">%s</td></tr>`, fmtMoney(rwa)))
	s.WriteString(`<tr><td colspan="2"><hr class="my-1"></td></tr>`)

	ratioClass := "has-text-success-dark"
	if cet1Ratio < minCET1Ratio {
		ratioClass = "has-text-danger"
	}
	s.WriteString(fmt.Sprintf(`<tr><td class="has-text-weight-bold">CET1 Ratio</td><td class="has-text-right has-text-weight-bold %s">%.2f%%</td></tr>`, ratioClass, cet1Ratio*100))
	s.WriteString(fmt.Sprintf(`<tr><td>Minimum Required (Basel III)</td><td class="has-text-right">%.1f%%</td></tr>`, minCET1Ratio*100))
	s.WriteString(`</tbody></table>`)
	s.WriteString(`</div>`)

	// Notes
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-6 has-text-grey">Notes</h3>`)
	s.WriteString(`<div class="content is-small">`)
	s.WriteString(`<ol>`)
	s.WriteString(`<li><strong>Tier 1 Capital</strong> is the core measure of a bank's financial strength from a regulator's point of view. `)
	s.WriteString(`In this model, CET1 capital equals retained earnings &mdash; the bank's only equity. `)
	s.WriteString(`See <a href="https://en.wikipedia.org/wiki/Tier_1_capital" target="_blank">Tier&nbsp;1&nbsp;capital</a>.</li>`)
	s.WriteString(`<li><strong>Capital Requirement</strong> (Basel III) sets minimum capital ratios banks must maintain against risk-weighted assets. `)
	s.WriteString(`The CET1 minimum is 4.5% of RWA. `)
	s.WriteString(`See <a href="https://en.wikipedia.org/wiki/Capital_requirement" target="_blank">Capital&nbsp;requirement</a>.</li>`)
	s.WriteString(`<li><strong>Risk Weights:</strong> Loans to customers carry a 100% risk weight. `)
	s.WriteString(`Government securities (gilts) and cash at the BoE carry a 0% risk weight, `)
	s.WriteString(`meaning bank capital invested in sovereign debt does not consume regulatory capital. `)
	s.WriteString(`This is why gilts do not appear in the RWA figure above.</li>`)
	s.WriteString(`</ol>`)
	s.WriteString(`</div></div>`)

	return s.String()
}
