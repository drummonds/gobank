package main

import (
	"log"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
)

// The bank's book: totals over every customer account, derived from the
// ledger rather than from a customer list held in memory.

// bookTotals is the balance of all customer savings and lending accounts.
type bookTotals struct {
	Savings luca.Amount
	Lending luca.Amount
}

// bookTotals returns the running totals. They are maintained as balances
// change (funding, interest application) and re-derived from the ledger by
// refreshBookTotals. Must be called with ds.mu held.
func (ds *DemoState) bookTotals() bookTotals {
	return ds.book
}

// refreshBookTotals re-derives the totals from the ledger, e.g. after an
// import wrote movements directly. Must be called with ds.mu held.
func (ds *DemoState) refreshBookTotals() {
	if ds.ledger == nil {
		return
	}
	at := ds.currentDay.AddDate(0, 0, 1) // everything valued up to the end of today
	savings, _, err := ds.ledger.BalanceByPath("Liability:Savings", at)
	if err != nil {
		log.Printf("refreshBookTotals: savings: %v", err)
		return
	}
	lending, _, err := ds.ledger.BalanceByPath("Asset:Loans", at)
	if err != nil {
		log.Printf("refreshBookTotals: lending: %v", err)
		return
	}
	ds.book = bookTotals{Savings: savings, Lending: lending}
}

// addToBook records a balance change on a customer account in the running
// totals. Must be called with ds.mu held.
func (ds *DemoState) addToBook(family gbp.ProductFamily, delta luca.Amount) {
	if family == gbp.FamilySavings {
		ds.book.Savings += delta
	} else {
		ds.book.Lending += delta
	}
}

// interestTotals is the bank's customer interest to date on an accrual
// basis: loan interest income and deposit interest expense, each applied
// plus accrued-but-unapplied whole pence. Both are read from the ledger P&L
// accounts, which the engine credits on application and the daily accrual
// movements credit as interest accrues (balance = in - out, so the P&L
// accounts run negative as interest is recognised).
func (ds *DemoState) interestTotals() (loanIncome, depositExpense luca.Amount) {
	if ds.ledger == nil {
		return 0, 0
	}
	return -ds.pathBalance("Income:Interest"), -ds.pathBalance("Expense:Interest")
}

// pathBalance is the balance of the account at path, or zero if it has no
// account yet.
func (ds *DemoState) pathBalance(path string) luca.Amount {
	acct, err := ds.ledger.GetAccount(path)
	if err != nil || acct == nil {
		return 0
	}
	bal, err := ds.ledger.Balance(acct.ID)
	if err != nil {
		log.Printf("pathBalance %s: %v", path, err)
		return 0
	}
	return bal
}

// productTotal is the number of accounts on a product and their combined
// balance.
type productTotal struct {
	Accounts int
	Balance  luca.Amount
}

// productTotals reads account counts and balances per product through the
// customers and ledger contract views.
func (ds *DemoState) productTotals() map[string]productTotal {
	totals := map[string]productTotal{}
	if ds.db == nil {
		return totals
	}
	rows, err := ds.db.Query(`SELECT product_id, COUNT(*) FROM contract_customer_accounts GROUP BY product_id`)
	if err != nil {
		log.Printf("productTotals: count: %v", err)
		return totals
	}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			log.Printf("productTotals: scan count: %v", err)
			break
		}
		totals[id] = productTotal{Accounts: n}
	}
	rows.Close()

	rows, err = ds.db.Query(`SELECT product_id, COALESCE(SUM(delta), 0) FROM (
			SELECT ca.product_id, m.amount AS delta
			  FROM contract_customer_accounts ca JOIN contract_ledger_movements m ON m.to_account_id = ca.ledger_account_id
			UNION ALL
			SELECT ca.product_id, -m.amount AS delta
			  FROM contract_customer_accounts ca JOIN contract_ledger_movements m ON m.from_account_id = ca.ledger_account_id
		) d GROUP BY product_id`)
	if err != nil {
		log.Printf("productTotals: balances: %v", err)
		return totals
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var bal luca.Amount
		if err := rows.Scan(&id, &bal); err != nil {
			log.Printf("productTotals: scan balance: %v", err)
			break
		}
		pt := totals[id]
		pt.Balance = bal
		totals[id] = pt
	}
	return totals
}
