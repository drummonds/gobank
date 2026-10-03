package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
)

// The account register records which ledger accounts a customer holds and
// the bank-facing details of each (product, sort code, account number). It
// is the demo's own table: customer identity lives in gobanks-customers,
// balances in the go-luca ledger, accrual in the products engine.
//
// idx is the account's position in the customer's account list; the bank
// app and admin pages address accounts by (customer, index).

var customersSchema = componentSchema{component: "customers", migrations: []migration{
	{1, []string{`CREATE TABLE IF NOT EXISTS customer_accounts (
		customer_id VARCHAR(20) NOT NULL,
		idx INTEGER NOT NULL,
		ledger_account_id VARCHAR(64) NOT NULL UNIQUE,
		product_id VARCHAR(50) NOT NULL,
		sort_code VARCHAR(8) NOT NULL,
		account_num VARCHAR(8) NOT NULL,
		opened TIMESTAMP NOT NULL,
		PRIMARY KEY (customer_id, idx)
	)`}},
}}

// createCustomerAccountsView (re)creates the contract view: the register
// as other components read it. Recreated at every start so a durable
// database picks up a changed definition.
func (ds *DemoState) createCustomerAccountsView() {
	for _, stmt := range []string{
		`DROP VIEW IF EXISTS contract_customer_accounts`,
		`CREATE VIEW contract_customer_accounts AS
			SELECT customer_id, idx, ledger_account_id, product_id, sort_code, account_num, opened FROM customer_accounts`,
	} {
		if _, err := ds.db.Exec(stmt); err != nil {
			log.Printf("initDB: contract_customer_accounts: %v", err)
		}
	}
}

// clearRegisterLocked empties the account register, e.g. on reset of a
// durable database. Must be called with ds.mu held.
func (ds *DemoState) clearRegisterLocked() {
	if ds.db == nil {
		return
	}
	if _, err := ds.db.Exec(`DELETE FROM customer_accounts`); err != nil {
		log.Printf("clearRegister: %v", err)
	}
}

// registeredAccount is one row of the register as a resume reads it: which
// ledger account, on which product, opened when.
type registeredAccount struct {
	CustomerID      string
	LedgerAccountID string
	ProductID       string
	Opened          time.Time
}

// registeredAccounts is every account on the register, in customer and
// index order.
func registeredAccounts(db *sql.DB) ([]registeredAccount, error) {
	rows, err := db.Query(`SELECT customer_id, ledger_account_id, product_id, opened FROM customer_accounts ORDER BY customer_id, idx`)
	if err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}
	defer rows.Close()
	var out []registeredAccount
	for rows.Next() {
		var a registeredAccount
		if err := rows.Scan(&a.CustomerID, &a.LedgerAccountID, &a.ProductID, &a.Opened); err != nil {
			return nil, fmt.Errorf("register: scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// registeredBalances is the ledger balance of every registered account, in
// one pass over the ledger's movements rather than a query per account.
func registeredBalances(db *sql.DB) (map[string]luca.Amount, error) {
	rows, err := db.Query(`SELECT ca.ledger_account_id, COALESCE(SUM(m.delta), 0) FROM contract_customer_accounts ca
		LEFT JOIN (
			SELECT to_account_id AS id, amount AS delta FROM contract_ledger_movements
			UNION ALL
			SELECT from_account_id AS id, -amount AS delta FROM contract_ledger_movements
		) m ON m.id = ca.ledger_account_id
		GROUP BY ca.ledger_account_id`)
	if err != nil {
		return nil, fmt.Errorf("register balances: %w", err)
	}
	defer rows.Close()
	out := map[string]luca.Amount{}
	for rows.Next() {
		var id string
		var bal luca.Amount
		if err := rows.Scan(&id, &bal); err != nil {
			return nil, fmt.Errorf("register balances: scan: %w", err)
		}
		out[id] = bal
	}
	return out, rows.Err()
}

// lastCustomerSeq is the sequence number of the newest customer (IDs are
// cust-<seq>), zero when there are none: a resumed run numbers its next
// customer after it.
func (ds *DemoState) lastCustomerSeq() int {
	n := ds.custStoreCount()
	if n == 0 {
		return 0
	}
	recs, _, err := ds.custStore.List(context.Background(), n-1, 1)
	if err != nil || len(recs) == 0 {
		log.Printf("lastCustomerSeq: %v", err)
		return 0
	}
	var seq int
	if _, err := fmt.Sscanf(recs[0].ID, "cust-%d", &seq); err != nil {
		log.Printf("lastCustomerSeq: customer ID %q: %v", recs[0].ID, err)
	}
	return seq
}

// execer is *sql.DB or *sql.Tx.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// registerAccounts writes a customer's accounts to the register. The
// accounts must already have ledger IDs (see addCustomerToLedger).
func registerAccounts(q execer, cust *CustomerRecord) error {
	for i, a := range cust.Accounts {
		if a.LedgerAccountID == "" {
			continue
		}
		_, err := q.Exec(`INSERT INTO customer_accounts
			(customer_id, idx, ledger_account_id, product_id, sort_code, account_num, opened)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			cust.ID, i, a.LedgerAccountID, a.ProductID, a.SortCode, a.AccountNum, a.OpenDate.UTC())
		if err != nil {
			return fmt.Errorf("register account %d of %s: %w", i, cust.ID, err)
		}
	}
	return nil
}

// productByID looks a product up in the catalogue.
func (ds *DemoState) productByID(id string) (Product, bool) {
	for _, p := range ds.products {
		if p.ID == id {
			return p, true
		}
	}
	return Product{}, false
}

// customerCount is the number of customers on the books.
func (ds *DemoState) customerCount() int {
	return ds.custStoreCount()
}

// customerByID loads one customer with their accounts. Balances and accrual
// come from the products engine, applied interest from the ledger.
func (ds *DemoState) customerByID(id string) (*CustomerRecord, bool) {
	if ds.custStore == nil {
		return nil, false
	}
	rec, err := ds.custStore.GetByID(context.Background(), id)
	if err != nil || rec == nil {
		return nil, false
	}
	cust := CustomerRecord{
		ID:       rec.ID,
		JoinDate: rec.JoinDate,
		KYCStatus: KYCStatus{
			Verified:      rec.KYCVerified,
			LastCheckDate: rec.KYCLastCheck,
			RiskRating:    rec.KYCRiskRating,
		},
	}
	cust.Accounts = ds.accountsOf(id)
	return &cust, true
}

// customerPage returns one page of customers (customersPerPage each, oldest
// first) with their accounts, and the total number of customers.
func (ds *DemoState) customerPage(page int) ([]CustomerRecord, int) {
	if ds.custStore == nil {
		return nil, 0
	}
	if page < 1 {
		page = 1
	}
	recs, total, err := ds.custStore.List(context.Background(), (page-1)*customersPerPage, customersPerPage)
	if err != nil {
		log.Printf("customerPage: %v", err)
		return nil, 0
	}
	out := make([]CustomerRecord, 0, len(recs))
	for _, rec := range recs {
		out = append(out, CustomerRecord{
			ID:       rec.ID,
			JoinDate: rec.JoinDate,
			KYCStatus: KYCStatus{
				Verified:      rec.KYCVerified,
				LastCheckDate: rec.KYCLastCheck,
				RiskRating:    rec.KYCRiskRating,
			},
			Accounts: ds.accountsOf(rec.ID),
		})
	}
	return out, total
}

// accountsOf reads a customer's accounts from the register in index order
// and fills in balances, accrual and applied interest.
func (ds *DemoState) accountsOf(customerID string) []CustomerAccount {
	if ds.db == nil {
		return nil
	}
	rows, err := ds.db.Query(`SELECT ledger_account_id, product_id, sort_code, account_num, opened
		FROM customer_accounts WHERE customer_id = $1 ORDER BY idx`, customerID)
	if err != nil {
		log.Printf("accountsOf %s: %v", customerID, err)
		return nil
	}
	defer rows.Close()
	var accounts []CustomerAccount
	for rows.Next() {
		var a CustomerAccount
		if err := rows.Scan(&a.LedgerAccountID, &a.ProductID, &a.SortCode, &a.AccountNum, &a.OpenDate); err != nil {
			log.Printf("accountsOf %s: scan: %v", customerID, err)
			return nil
		}
		if p, ok := ds.productByID(a.ProductID); ok {
			a.ProductName, a.Family, a.Rate = p.Name, p.Family, p.Rate
		}
		accounts = append(accounts, a)
	}
	for i := range accounts {
		ds.fillAccountFigures(&accounts[i])
	}
	return accounts
}

// fillAccountFigures sets an account's balance and accrual from the engine
// and its lifetime applied interest from the ledger.
func (ds *DemoState) fillAccountFigures(a *CustomerAccount) {
	if ds.sim != nil {
		ds.simMu.Lock()
		if ma, ok := ds.sim.GetManagedAccount(a.LedgerAccountID); ok {
			a.Balance = ma.CachedBalance
			a.Accrued = ma.AccruedInterest()
			a.AccruedNumerator = ma.AccruedNumerator
		}
		ds.simMu.Unlock()
	}
	a.Interest = ds.appliedInterest(a.LedgerAccountID)
}

// appliedInterest is the interest the engine has applied to an account:
// the sum of its month-end application movements.
func (ds *DemoState) appliedInterest(ledgerAccountID string) luca.Amount {
	var applied luca.Amount
	err := ds.db.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM contract_ledger_movements WHERE to_account_id = $1 AND code = $2`,
		ledgerAccountID, luca.CodeInterestAccrual).Scan(&applied)
	if err != nil {
		log.Printf("appliedInterest %s: %v", ledgerAccountID, err)
	}
	return applied
}

// customerInterest is the gross savings interest a customer has earned to
// date: applied plus accrued-but-unapplied whole pence.
type customerInterest struct {
	CustomerID string
	Interest   luca.Amount
}

// savingsInterestByCustomer lists every customer with savings interest to
// date, oldest customer first, for the BBSI return.
func (ds *DemoState) savingsInterestByCustomer() []customerInterest {
	if ds.db == nil {
		return nil
	}
	rows, err := ds.db.Query(`SELECT customer_id, ledger_account_id, product_id FROM customer_accounts ORDER BY customer_id, idx`)
	if err != nil {
		log.Printf("savingsInterestByCustomer: %v", err)
		return nil
	}
	defer rows.Close()
	var out []customerInterest
	for rows.Next() {
		var custID, acctID, productID string
		if err := rows.Scan(&custID, &acctID, &productID); err != nil {
			log.Printf("savingsInterestByCustomer: scan: %v", err)
			return nil
		}
		if p, ok := ds.productByID(productID); !ok || p.Family != gbp.FamilySavings {
			continue
		}
		a := CustomerAccount{LedgerAccountID: acctID}
		ds.fillAccountFigures(&a)
		if interest := a.Interest + a.Accrued; interest > 0 {
			if n := len(out); n > 0 && out[n-1].CustomerID == custID {
				out[n-1].Interest += interest
			} else {
				out = append(out, customerInterest{CustomerID: custID, Interest: interest})
			}
		}
	}
	return out
}
