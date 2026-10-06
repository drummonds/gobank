package main

import (
	"context"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
)

// The bank app's data: what the phone screens and the JSON API show, read
// through the core's customer-facing queries.

// --- API response types (shared by HTML views and JSON API) ---

type apiCustomer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type apiAccount struct {
	ProductName string      `json:"product_name"`
	Family      string      `json:"family"`
	Currency    string      `json:"currency"` // ISO 4217
	Rate        float64     `json:"rate"`
	Balance     luca.Amount `json:"balance_minor"` // integer minor units (pence)
	Interest    luca.Amount `json:"interest_minor"`
}

type apiAccountsResponse struct {
	CustomerID   string       `json:"customer_id"`
	CustomerName string       `json:"customer_name"`
	Accounts     []apiAccount `json:"accounts"`
	TotalSavings luca.Amount  `json:"total_savings_minor"`
	TotalLending luca.Amount  `json:"total_lending_minor"`
}

type apiTxEntry struct {
	ID          string      `json:"id"`
	Date        string      `json:"date"`
	ProductName string      `json:"product_name"`
	Type        string      `json:"type"`
	Currency    string      `json:"currency"` // ISO 4217
	Amount      luca.Amount `json:"amount_minor"`
	Balance     luca.Amount `json:"balance_minor"`
	Reference   string      `json:"reference"`
}

type apiTransactionsResponse struct {
	CustomerID string       `json:"customer_id"`
	Page       int          `json:"page"`
	TotalCount int          `json:"total_count"`
	PerPage    int          `json:"per_page"`
	Entries    []apiTxEntry `json:"entries"`
}

type apiProductDetail struct {
	CustomerID   string      `json:"customer_id"`
	CustomerName string      `json:"customer_name"`
	AccountIndex int         `json:"account_index"`
	ProductName  string      `json:"product_name"`
	Family       string      `json:"family"`
	Currency     string      `json:"currency"` // ISO 4217
	Rate         float64     `json:"rate"`
	Balance      luca.Amount `json:"balance_minor"`
	Interest     luca.Amount `json:"interest_minor"`
	SortCode     string      `json:"sort_code"`
	AccountNum   string      `json:"account_num"`
	OpenDate     string      `json:"open_date"`
}

// --- Service functions (shared by HTML views and JSON API) ---

const txPerPage = 20

// customerExists reports whether a customer with the given ID exists.
func customerExists(q core.CustomerQueries, id string) bool {
	_, err := q.Customer(context.Background(), id)
	return err == nil
}

func bankAppCustomerList(q core.StaffQueries) []apiCustomer {
	ctx := context.Background()
	page, _ := q.CustomerPage(ctx, 1)
	result := make([]apiCustomer, 0, len(page.Customers))
	for _, c := range page.Customers {
		name, _ := q.CustomerName(ctx, c.ID)
		result = append(result, apiCustomer{ID: c.ID, Name: name})
	}
	return result
}

func bankAppAccounts(q core.CustomerQueries, custID string) *apiAccountsResponse {
	ctx := context.Background()
	cust, err := q.Customer(ctx, custID)
	if err != nil {
		return nil
	}
	accounts, err := q.Accounts(ctx, custID)
	if err != nil {
		return nil
	}
	resp := &apiAccountsResponse{CustomerID: cust.ID, CustomerName: cust.Name}
	for _, a := range accounts {
		resp.Accounts = append(resp.Accounts, apiAccount{
			ProductName: a.ProductName, Family: a.Family, Currency: a.Currency, Rate: a.Rate, Balance: a.Balance, Interest: a.Interest,
		})
		if a.Family == string(gbp.FamilySavings) {
			resp.TotalSavings += a.Balance
		} else {
			resp.TotalLending += a.Balance
		}
	}
	return resp
}

func apiEntries(page core.TransactionPage) []apiTxEntry {
	out := make([]apiTxEntry, len(page.Entries))
	for i, tx := range page.Entries {
		out[i] = apiTxEntry{
			ID: tx.ID, Date: tx.Date, ProductName: tx.ProductName, Type: tx.Type, Currency: tx.Currency,
			Amount: tx.Amount, Balance: tx.Balance, Reference: tx.Reference,
		}
	}
	return out
}

func bankAppTransactions(q core.CustomerQueries, custID string, page int) apiTransactionsResponse {
	tp, _ := q.Transactions(context.Background(), custID, page)
	return apiTransactionsResponse{CustomerID: custID, Page: max(page, 1), TotalCount: tp.Total, PerPage: txPerPage, Entries: apiEntries(tp)}
}

func bankAppProductDetail(q core.CustomerQueries, custID string, accountIdx int) *apiProductDetail {
	ctx := context.Background()
	cust, err := q.Customer(ctx, custID)
	if err != nil {
		return nil
	}
	accounts, err := q.Accounts(ctx, custID)
	if err != nil || accountIdx < 0 || accountIdx >= len(accounts) {
		return nil
	}
	a := accounts[accountIdx]
	return &apiProductDetail{
		CustomerID: cust.ID, CustomerName: cust.Name, AccountIndex: accountIdx,
		ProductName: a.ProductName, Family: a.Family, Currency: a.Currency, Rate: a.Rate,
		Balance: a.Balance, Interest: a.Interest, SortCode: a.SortCode, AccountNum: a.AccountNum, OpenDate: a.OpenDate,
	}
}

func bankAppProductTransactions(q core.CustomerQueries, custID string, accountIdx, page int) apiTransactionsResponse {
	tp, _ := q.AccountTransactions(context.Background(), custID, accountIdx, page)
	return apiTransactionsResponse{CustomerID: custID, Page: max(page, 1), TotalCount: tp.Total, PerPage: txPerPage, Entries: apiEntries(tp)}
}
