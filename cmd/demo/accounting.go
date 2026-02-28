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
	ds.mu.Unlock()

	opCosts := opCostPerDay * float64(dayCount)
	netInterest := loanInterestIncome - depositInterestExpense
	profit := netInterest - opCosts

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Profit &amp; Loss</h2>`)
	s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">For %d simulated days</p>`, dayCount))

	s.WriteString(`<div class="box">`)
	s.WriteString(`<table class="table is-fullwidth">`)
	s.WriteString(`<tbody>`)
	s.WriteString(fmt.Sprintf(`<tr><td><strong>Loan Interest Income</strong></td><td class="has-text-right has-text-success-dark">£%.2f</td></tr>`, loanInterestIncome))
	s.WriteString(fmt.Sprintf(`<tr><td><strong>Deposit Interest Expense</strong></td><td class="has-text-right has-text-danger">(£%.2f)</td></tr>`, depositInterestExpense))
	s.WriteString(fmt.Sprintf(`<tr><td class="has-text-weight-semibold">Net Interest Margin</td><td class="has-text-right has-text-weight-semibold">£%.2f</td></tr>`, netInterest))
	s.WriteString(`<tr><td colspan="2"><hr class="my-1"></td></tr>`)
	s.WriteString(fmt.Sprintf(`<tr><td><strong>Operational Costs</strong></td><td class="has-text-right has-text-danger">(£%.2f)</td></tr>`, opCosts))
	s.WriteString(`<tr><td colspan="2"><hr class="my-1"></td></tr>`)

	profitClass := "has-text-success-dark"
	profitSign := ""
	if profit < 0 {
		profitClass = "has-text-danger"
		profitSign = "-"
		profit = -profit
	}
	s.WriteString(fmt.Sprintf(`<tr><td class="has-text-weight-bold is-size-5">Net Profit</td><td class="has-text-right has-text-weight-bold is-size-5 %s">%s£%.2f</td></tr>`, profitClass, profitSign, profit))
	s.WriteString(`</tbody></table></div>`)

	return s.String()
}

// BuildBalanceSheetHTML renders a balance sheet derived from current state.
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
	ds.mu.Unlock()

	opCosts := opCostPerDay * float64(dayCount)
	retainedEarnings := (loanInterest - depositInterest) - opCosts
	cash := totalDeposits - totalLoans + retainedEarnings
	totalAssets := totalLoans + cash
	totalLiabilities := totalDeposits
	equity := retainedEarnings

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Balance Sheet</h2>`)
	s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">As at day %d</p>`, dayCount))

	s.WriteString(`<div class="columns">`)

	// Assets
	s.WriteString(`<div class="column"><div class="box">`)
	s.WriteString(`<h3 class="title is-5">Assets</h3>`)
	s.WriteString(`<table class="table is-fullwidth"><tbody>`)
	s.WriteString(fmt.Sprintf(`<tr><td>Loans to Customers</td><td class="has-text-right">£%.2f</td></tr>`, totalLoans))
	s.WriteString(fmt.Sprintf(`<tr><td>Cash &amp; Reserves</td><td class="has-text-right">£%.2f</td></tr>`, cash))
	s.WriteString(`<tr><td colspan="2"><hr class="my-1"></td></tr>`)
	s.WriteString(fmt.Sprintf(`<tr><td class="has-text-weight-bold">Total Assets</td><td class="has-text-right has-text-weight-bold">£%.2f</td></tr>`, totalAssets))
	s.WriteString(`</tbody></table></div></div>`)

	// Liabilities + Equity
	s.WriteString(`<div class="column"><div class="box">`)
	s.WriteString(`<h3 class="title is-5">Liabilities &amp; Equity</h3>`)
	s.WriteString(`<table class="table is-fullwidth"><tbody>`)
	s.WriteString(fmt.Sprintf(`<tr><td>Customer Deposits</td><td class="has-text-right">£%.2f</td></tr>`, totalLiabilities))
	eqClass := ""
	if equity < 0 {
		eqClass = ` class="has-text-danger"`
	}
	s.WriteString(fmt.Sprintf(`<tr><td>Retained Earnings</td><td class="has-text-right"%s>£%.2f</td></tr>`, eqClass, equity))
	s.WriteString(`<tr><td colspan="2"><hr class="my-1"></td></tr>`)
	s.WriteString(fmt.Sprintf(`<tr><td class="has-text-weight-bold">Total</td><td class="has-text-right has-text-weight-bold">£%.2f</td></tr>`, totalLiabilities+equity))
	s.WriteString(`</tbody></table></div></div>`)

	s.WriteString(`</div>`)

	return s.String()
}
