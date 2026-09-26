package main

import (
	"strings"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
)

// TxType classifies transaction log entries.
type TxType int

const (
	TxInterestCredit   TxType = iota // interest accrued on savings
	TxInterestDebit                  // interest accrued on loan (borrower owes more)
	TxDepositIn                      // external deposit into savings
	TxTransferOut                    // outgoing transfer to another customer
	TxTransferIn                     // incoming transfer from another customer
	TxLoanDisbursement               // loan funds disbursed to customer
)

func (t TxType) String() string {
	switch t {
	case TxInterestCredit:
		return "Interest"
	case TxInterestDebit:
		return "Loan Interest"
	case TxDepositIn:
		return "Deposit"
	case TxTransferOut:
		return "Transfer Out"
	case TxTransferIn:
		return "Transfer In"
	case TxLoanDisbursement:
		return "Loan"
	default:
		return "Unknown"
	}
}

// TxEntry is a single entry in the customer-facing transaction timeline.
type TxEntry struct {
	ID          int
	Date        time.Time
	CustomerID  string
	AccountID   string // ledger account the entry is for
	ProductName string // snapshot at time of entry
	Type        TxType
	Amount      luca.Amount // minor units, always positive; direction implied by Type
	Balance     luca.Amount // minor units; running balance after this entry
	Reference   string
}

// emitTx appends a transaction log entry. Must be called with ds.mu held.
func (ds *DemoState) emitTx(date time.Time, custID, accountID, productName string, txType TxType, amount, balance luca.Amount, ref string) {
	ds.appendTx(TxEntry{
		Date:        date,
		CustomerID:  custID,
		AccountID:   accountID,
		ProductName: productName,
		Type:        txType,
		Amount:      amount,
		Balance:     balance,
		Reference:   ref,
	})
}

// appendTx assigns the next ID to an entry and appends it. Must be called
// with ds.mu held.
func (ds *DemoState) appendTx(tx TxEntry) {
	ds.nextTxID++
	tx.ID = ds.nextTxID
	ds.txLog = append(ds.txLog, tx)
}

// interestTx is the customer-facing entry for interest the engine applied
// to a customer account. The customer and product are read from the
// account's ledger path (see addCustomerToLedger).
func (ds *DemoState) interestTx(date time.Time, ma *gbp.ManagedAccount, applied luca.Amount) TxEntry {
	txType := TxInterestCredit
	if ma.Family == gbp.FamilyLending {
		txType = TxInterestDebit
	}
	if applied < 0 {
		applied = -applied
	}
	custID, productID := customerAndProductFromPath(ma.Account.FullPath)
	productName := productID
	if p, ok := ds.productByID(productID); ok {
		productName = p.Name
	}
	return TxEntry{
		Date:        date,
		CustomerID:  custID,
		AccountID:   ma.Account.ID,
		ProductName: productName,
		Type:        txType,
		Amount:      applied,
		Balance:     ma.CachedBalance,
		Reference:   "INT",
	}
}

// ProductTransactions returns transaction entries for a specific customer account,
// newest first. page is 1-based; perPage entries per page.
func (ds *DemoState) ProductTransactions(custID string, accountIdx, page, perPage int) (entries []TxEntry, totalCount int) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	accounts := ds.accountsOf(custID)
	if accountIdx < 0 || accountIdx >= len(accounts) {
		return nil, 0
	}
	accountID := accounts[accountIdx].LedgerAccountID
	var matches []TxEntry
	for _, tx := range ds.txLog {
		if tx.CustomerID == custID && tx.AccountID == accountID {
			matches = append(matches, tx)
		}
	}

	totalCount = len(matches)
	if page < 1 {
		page = 1
	}

	// Reverse to newest-first
	for i, j := 0, len(matches)-1; i < j; i, j = i+1, j-1 {
		matches[i], matches[j] = matches[j], matches[i]
	}

	start := (page - 1) * perPage
	if start >= len(matches) {
		return nil, totalCount
	}
	end := min(start+perPage, len(matches))
	return matches[start:end], totalCount
}

// CustomerTransactions returns transaction entries for a given customer,
// newest first. page is 1-based; perPage entries per page.
func (ds *DemoState) CustomerTransactions(custID string, page, perPage int) (entries []TxEntry, totalCount int) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	// Collect matching entries (already in chronological order)
	var matches []TxEntry
	for _, tx := range ds.txLog {
		if tx.CustomerID == custID {
			matches = append(matches, tx)
		}
	}

	totalCount = len(matches)
	if page < 1 {
		page = 1
	}

	// Reverse to newest-first
	for i, j := 0, len(matches)-1; i < j; i, j = i+1, j-1 {
		matches[i], matches[j] = matches[j], matches[i]
	}

	start := (page - 1) * perPage
	if start >= len(matches) {
		return nil, totalCount
	}
	end := min(start+perPage, len(matches))
	return matches[start:end], totalCount
}

// customerAndProductFromPath splits a customer account path of the form
// <type>:<family>:<customer>:<product> as written by addCustomerToLedger.
func customerAndProductFromPath(fullPath string) (customerID, productID string) {
	parts := strings.Split(fullPath, ":")
	if len(parts) < 4 {
		return "", ""
	}
	return parts[2], parts[3]
}
