package staff

import (
	"context"
	"fmt"
	"html"
	"strings"

	luca "git.bytestone.uk/hum3/go-luca"

	"git.bytestone.uk/hum3/gobank/core"
)

// BuildPnLHTML renders a Profit & Loss statement from the core's figures.
func BuildPnLHTML(q core.BookQueries) string {
	pl, _ := q.ProfitAndLoss(context.Background())
	dayCount := pl.DayCount
	loanInterestIncome, boeInterestIncome, depositInterestExpense := pl.LoanInterestIncome, pl.BoEInterestIncome, pl.DepositInterestExpense
	opCosts := pl.OperatingCosts
	netInterest := pl.NetInterest()
	profit := pl.NetProfit()

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Profit &amp; Loss</h2>`)
	period := fmt.Sprintf("For the period %s to %s (%d days)", pl.From.Format("2 January 2006"), pl.To.Format("2 January 2006"), dayCount)
	if !pl.YearEnd.IsZero() {
		period += fmt.Sprintf(", in the financial year ending %s", pl.YearEnd.Format("2 January 2006"))
	}
	s.WriteString(`<p class="subtitle is-6 has-text-grey">` + period + `</p>`)

	s.WriteString(`<div class="box">`)
	s.WriteString(`<table class="table is-fullwidth">`)
	s.WriteString(`<tbody>`)
	s.WriteString(fmt.Sprintf(`<tr><td><strong>Loan Interest Income</strong></td><td class="has-text-right has-text-success-dark">%s</td></tr>`, FormatMoney(loanInterestIncome)))
	s.WriteString(fmt.Sprintf(`<tr><td><strong>BoE Interest Income</strong></td><td class="has-text-right has-text-success-dark">%s</td></tr>`, FormatMoney(boeInterestIncome)))
	s.WriteString(fmt.Sprintf(`<tr><td><strong>Deposit Interest Expense</strong></td><td class="has-text-right has-text-danger">(%s)</td></tr>`, FormatMoney(depositInterestExpense)))
	s.WriteString(fmt.Sprintf(`<tr><td class="has-text-weight-semibold">Net Interest Margin</td><td class="has-text-right has-text-weight-semibold">%s</td></tr>`, FormatMoney(netInterest)))
	s.WriteString(`<tr><td colspan="2"><hr class="my-1"></td></tr>`)
	s.WriteString(fmt.Sprintf(`<tr><td><strong>Operational Costs</strong></td><td class="has-text-right has-text-danger">(%s)</td></tr>`, FormatMoney(opCosts)))
	s.WriteString(`<tr><td colspan="2"><hr class="my-1"></td></tr>`)

	profitClass := "has-text-success-dark"
	if profit < 0 {
		profitClass = "has-text-danger"
	}
	s.WriteString(fmt.Sprintf(`<tr><td class="has-text-weight-bold is-size-5">Net Profit</td><td class="has-text-right has-text-weight-bold is-size-5 %s">%s</td></tr>`, profitClass, FormatMoney(profit)))
	s.WriteString(`</tbody></table></div>`)

	return s.String()
}

// BuildBalanceSheetHTML renders the balance sheet as the core reads it
// from the ledger: assets against liabilities and equity, the customers'
// accrued interest on both sides, and the regulatory capital (CET1)
// section. It balances; the page shows that it does.
func BuildBalanceSheetHTML(q core.BookQueries) string {
	bs, err := q.BalanceSheet(context.Background())
	if err != nil {
		return `<p class="has-text-danger">The balance sheet could not be read: ` + html.EscapeString(err.Error()) + `</p>`
	}
	rwa := bs.RiskWeightedAssets
	cet1Ratio := bs.CET1Ratio()

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Balance Sheet</h2>`)
	s.WriteString(fmt.Sprintf(`<p class="subtitle is-6 has-text-grey">As at %s (day %d), read from the ledger</p>`, bs.AsAt.Format("2 January 2006"), bs.DayCount))

	line := func(label string, v luca.Amount, weight string) string {
		return fmt.Sprintf(`<tr><td>%s</td><td class="has-text-right">%s</td><td class="has-text-right has-text-grey is-size-7">%s</td></tr>`, label, FormatMoney(v), weight)
	}
	total := func(label string, v luca.Amount) string {
		return fmt.Sprintf(`<tr><td class="has-text-weight-bold">%s</td><td class="has-text-right has-text-weight-bold">%s</td><td></td></tr>`, label, FormatMoney(v))
	}
	rule := `<tr><td colspan="3"><hr class="my-1"></td></tr>`

	s.WriteString(`<div class="columns">`)

	// Assets
	s.WriteString(`<div class="column"><div class="box">`)
	s.WriteString(`<h3 class="title is-5">Assets</h3>`)
	s.WriteString(`<table class="table is-fullwidth"><tbody>`)
	s.WriteString(line("Loans to customers", bs.Loans, "100% RW"))
	s.WriteString(line("Accrued interest receivable", bs.AccruedInterestReceivable, ""))
	if bs.Gilts != 0 {
		s.WriteString(line("Government securities (gilts)", bs.Gilts, "0% RW"))
	}
	s.WriteString(line("Cash and reserves at the BoE", bs.CashAtBoE, "0% RW"))
	s.WriteString(rule)
	s.WriteString(total("Total assets", bs.TotalAssets()))
	s.WriteString(`</tbody></table></div></div>`)

	// Liabilities and equity
	s.WriteString(`<div class="column"><div class="box">`)
	s.WriteString(`<h3 class="title is-5">Liabilities &amp; Equity</h3>`)
	s.WriteString(`<table class="table is-fullwidth"><tbody>`)
	s.WriteString(line("Customer deposits", bs.Deposits, ""))
	s.WriteString(line("Accrued interest payable", bs.AccruedInterestPayable, ""))
	s.WriteString(rule)
	s.WriteString(total("Total liabilities", bs.TotalLiabilities()))
	s.WriteString(line("Share capital", bs.Capital, ""))
	eq := "Retained earnings"
	if bs.RetainedEarnings < 0 {
		eq = `<span class="has-text-danger">Retained earnings (a loss)</span>`
	}
	s.WriteString(line(eq, bs.RetainedEarnings, ""))
	s.WriteString(rule)
	s.WriteString(total("Total equity (CET1 capital)", bs.Equity()))
	s.WriteString(rule)
	s.WriteString(total("Total liabilities and equity", bs.TotalLiabilities()+bs.Equity()))
	if !bs.Balances() {
		s.WriteString(fmt.Sprintf(`<tr><td colspan="3" class="has-text-danger">Does not balance: assets differ by %s. The books were read while the run was writing; reload.</td></tr>`, FormatMoney(bs.TotalAssets()-bs.TotalLiabilities()-bs.Equity())))
	}
	s.WriteString(`</tbody></table></div></div>`)

	s.WriteString(`</div>`) // end columns

	equity := bs.Equity()
	// Regulatory Capital section
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Regulatory Capital</h3>`)

	// CET1 compliance tag
	const minCET1Ratio = core.MinCET1Ratio
	if cet1Ratio >= minCET1Ratio {
		s.WriteString(`<span class="tag is-success is-medium mb-3">CET1 Compliant</span>`)
	} else {
		s.WriteString(`<span class="tag is-danger is-medium mb-3">CET1 Non-Compliant</span>`)
	}

	s.WriteString(`<table class="table is-fullwidth"><tbody>`)
	s.WriteString(fmt.Sprintf(`<tr><td><strong>Common Equity Tier 1 (CET1) Capital</strong></td><td class="has-text-right">%s</td></tr>`, FormatMoney(equity)))
	s.WriteString(fmt.Sprintf(`<tr><td>Risk-Weighted Assets (RWA)</td><td class="has-text-right">%s</td></tr>`, FormatMoney(rwa)))
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
	s.WriteString(`In this model, CET1 capital is the bank's equity: its share capital and its retained earnings. `)
	s.WriteString(`See <a href="https://en.wikipedia.org/wiki/Tier_1_capital" target="_blank">Tier&nbsp;1&nbsp;capital</a>.</li>`)
	s.WriteString(`<li><strong>Capital Requirement</strong> (Basel III) sets minimum capital ratios banks must maintain against risk-weighted assets. `)
	s.WriteString(`The CET1 minimum is 4.5% of RWA. `)
	s.WriteString(`See <a href="https://en.wikipedia.org/wiki/Capital_requirement" target="_blank">Capital&nbsp;requirement</a>.</li>`)
	s.WriteString(`<li><strong>Risk Weights:</strong> Loans to customers carry a 100% risk weight. `)
	s.WriteString(`Government securities (gilts) and cash at the BoE carry a 0% risk weight, `)
	s.WriteString(`meaning bank capital invested in sovereign debt does not consume regulatory capital. `)
	s.WriteString(`This is why gilts do not appear in the RWA figure above.</li>`)
	s.WriteString(`<li><strong>Read from the ledger</strong> (ADR-0008): every line is an account's balance or a sum of them. `)
	s.WriteString(`Operating costs leave cash each day, a gilt is bought out of cash, and BoE interest is earned into the reserves. `)
	s.WriteString(`The customers' accrued interest, which the products hold as positions until it is applied, is shown on both sides as the P&amp;L counts it.</li>`)
	s.WriteString(`</ol>`)
	s.WriteString(`</div></div>`)

	return s.String()
}
