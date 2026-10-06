package customers

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/core"
)

// A customer's transactions are a projection of the ledger (ADR-0002
// stage 2): every line is one movement on one of the customer's ledger
// accounts, read from the ledger's contract view, with the balance the
// movement left the account at. Nothing is kept in memory and nothing is
// written twice, so a restart shows the same lines, because the ledger is
// the record. The daily accrual postings move between the interest P&L
// and holding accounts, never the customer's, so a customer account's
// movements are exactly its statement lines.

// TxType is how a movement reads on the customer's statement.
type TxType int

const (
	TxInterestCredit   TxType = iota // interest applied to savings
	TxInterestDebit                  // interest applied to a loan (borrower owes more)
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

// Line is one line of the customer's statement: a ledger movement on one
// of their accounts.
type Line struct {
	ID          string // the ledger movement's ID
	Date        time.Time
	AccountID   string // ledger account the line is for
	ProductName string
	Type        TxType
	Currency    string      // the account's, ISO 4217
	Amount      luca.Amount // minor units, always positive; direction implied by Type
	Balance     luca.Amount // minor units; the account's balance after this line
	Reference   string      // the movement's description: a payment reference, or what interest was applied for
	known       time.Time   // when the ledger recorded it; orders lines within a day
}

// TxTypeOf reads a movement on a customer account as a statement line:
//
//	| code             | counterparty   | direction | family  | reads as      |
//	|------------------|----------------|-----------|---------|---------------|
//	| interest accrual | any            | any       | savings | Interest      |
//	| interest accrual | any            | any       | lending | Loan Interest |
//	| book transfer    | Equity:Capital | in        | savings | Deposit       |
//	| book transfer    | Equity:Capital | in        | lending | Loan          |
//	| book transfer    | other          | in        | any     | Transfer In   |
//	| book transfer    | other          | out       | any     | Transfer Out  |
func TxTypeOf(code, counterparty string, in bool, family gbp.ProductFamily) TxType {
	lending := family == gbp.FamilyLending
	switch {
	case code == luca.CodeInterestAccrual:
		if lending {
			return TxInterestDebit
		}
		return TxInterestCredit
	case counterparty == ledger.EquityCapital:
		if lending {
			return TxLoanDisbursement
		}
		return TxDepositIn
	case in:
		return TxTransferIn
	default:
		return TxTransferOut
	}
}

// accountLines is every movement on one customer account as statement
// lines, oldest first, each carrying the balance it left.
func (c *Customers) accountLines(ctx context.Context, a Account) ([]Line, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT id, to_account_id, from_path, to_path, amount, code, value_time, knowledge_time, description
		FROM contract_ledger_movements WHERE from_account_id = $1 OR to_account_id = $2
		ORDER BY value_time, knowledge_time`, a.LedgerAccountID, a.LedgerAccountID)
	if err != nil {
		return nil, fmt.Errorf("customers: statement of %s: %w", a.LedgerAccountID, err)
	}
	defer rows.Close()
	var lines []Line
	var balance luca.Amount
	for rows.Next() {
		var id, toID, fromPath, toPath, code, description string
		var amount luca.Amount
		var valueTime, knowledgeTime time.Time
		if err := rows.Scan(&id, &toID, &fromPath, &toPath, &amount, &code, &valueTime, &knowledgeTime, &description); err != nil {
			return nil, fmt.Errorf("customers: statement of %s: %w", a.LedgerAccountID, err)
		}
		in := toID == a.LedgerAccountID
		counterparty := fromPath
		if !in {
			counterparty = toPath
		}
		if in {
			balance += amount
		} else {
			balance -= amount
		}
		lines = append(lines, Line{
			ID: id, Date: valueTime, AccountID: a.LedgerAccountID, ProductName: a.ProductName,
			Type: TxTypeOf(code, counterparty, in, a.Family), Currency: a.Currency,
			Amount: amount, Balance: balance, Reference: description, known: knowledgeTime,
		})
	}
	return lines, rows.Err()
}

// linesOf is the statement lines of the given accounts, newest first: by
// value date, then by when the ledger learnt of them. pglike records that
// to the second (its NOW() is SQLite's datetime), so lines the same
// second apart fall back to their reference, and payment references are
// sequential.
func (c *Customers) linesOf(ctx context.Context, accounts []Account) ([]Line, error) {
	var lines []Line
	for _, a := range accounts {
		l, err := c.accountLines(ctx, a)
		if err != nil {
			return nil, err
		}
		lines = append(lines, l...)
	}
	slices.SortStableFunc(lines, func(a, b Line) int {
		if c := b.Date.Compare(a.Date); c != 0 {
			return c
		}
		if c := b.known.Compare(a.known); c != 0 {
			return c
		}
		return strings.Compare(b.Reference, a.Reference)
	})
	return lines, nil
}

// page is one page of lines as the core publishes them; page is 1-based.
func pageOf(lines []Line, page, perPage int) core.TransactionPage {
	page = max(page, 1)
	out := core.TransactionPage{Page: page, PerPage: perPage, Total: len(lines)}
	start := (page - 1) * perPage
	if start >= len(lines) {
		return out
	}
	for _, l := range lines[start:min(start+perPage, len(lines))] {
		out.Entries = append(out.Entries, core.Transaction{
			ID: l.ID, Date: l.Date.Format(time.DateOnly), ProductName: l.ProductName, Type: l.Type.String(),
			Currency: l.Currency, Reference: l.Reference, Amount: l.Amount, Balance: l.Balance,
		})
	}
	return out
}

// Transactions is one page of a customer's statement across all their
// accounts, newest first. An unknown customer is core.ErrNotFound.
func (c *Customers) Transactions(ctx context.Context, customerID string, page, perPage int) (core.TransactionPage, error) {
	cust, err := c.ByID(ctx, customerID)
	if err != nil {
		return core.TransactionPage{}, err
	}
	lines, err := c.linesOf(ctx, cust.Accounts)
	if err != nil {
		return core.TransactionPage{}, err
	}
	return pageOf(lines, page, perPage), nil
}

// AccountTransactions is one page of the statement of one of a customer's
// accounts, by its register index, newest first. An unknown customer or
// index is core.ErrNotFound.
func (c *Customers) AccountTransactions(ctx context.Context, customerID string, index, page, perPage int) (core.TransactionPage, error) {
	cust, err := c.ByID(ctx, customerID)
	if err != nil {
		return core.TransactionPage{}, err
	}
	if index < 0 || index >= len(cust.Accounts) {
		return core.TransactionPage{}, core.ErrNotFound
	}
	lines, err := c.linesOf(ctx, cust.Accounts[index:index+1])
	if err != nil {
		return core.TransactionPage{}, err
	}
	return pageOf(lines, page, perPage), nil
}
