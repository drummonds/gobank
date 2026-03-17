package main

import "time"

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
	AccountIdx  int    // index into customer's Accounts slice
	ProductName string // snapshot at time of entry
	Type        TxType
	Amount      float64 // always positive; direction implied by Type
	Balance     float64 // running balance after this entry
	Reference   string
}

// emitTx appends a transaction log entry. Must be called with ds.mu held.
func (ds *DemoState) emitTx(date time.Time, custID string, accIdx int, productName string, txType TxType, amount, balance float64, ref string) {
	ds.nextTxID++
	ds.txLog = append(ds.txLog, TxEntry{
		ID:          ds.nextTxID,
		Date:        date,
		CustomerID:  custID,
		AccountIdx:  accIdx,
		ProductName: productName,
		Type:        txType,
		Amount:      amount,
		Balance:     balance,
		Reference:   ref,
	})
}

// ProductTransactions returns transaction entries for a specific customer account,
// newest first. page is 1-based; perPage entries per page.
func (ds *DemoState) ProductTransactions(custID string, accountIdx, page, perPage int) (entries []TxEntry, totalCount int) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	var matches []TxEntry
	for _, tx := range ds.txLog {
		if tx.CustomerID == custID && tx.AccountIdx == accountIdx {
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
	end := start + perPage
	if end > len(matches) {
		end = len(matches)
	}
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
	end := start + perPage
	if end > len(matches) {
		end = len(matches)
	}
	return matches[start:end], totalCount
}
