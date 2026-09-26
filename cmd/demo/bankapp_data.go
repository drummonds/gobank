package main

import (
	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
)

// --- API response types (shared by HTML views and JSON API) ---

type apiCustomer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type apiAccount struct {
	ProductName string      `json:"product_name"`
	Family      string      `json:"family"`
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
	ID          int         `json:"id"`
	Date        string      `json:"date"`
	ProductName string      `json:"product_name"`
	Type        string      `json:"type"`
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
func (ds *DemoState) customerExists(id string) bool {
	_, ok := ds.customerByID(id)
	return ok
}

func (ds *DemoState) bankAppCustomerList() []apiCustomer {
	customers, _ := ds.customerPage(1)

	result := make([]apiCustomer, len(customers))
	for i, c := range customers {
		result[i] = apiCustomer{
			ID:   c.ID,
			Name: ds.lookupName(c.ID),
		}
	}
	return result
}

func (ds *DemoState) bankAppAccounts(custID string) *apiAccountsResponse {
	cust, ok := ds.customerByID(custID)
	if !ok {
		return nil
	}

	resp := &apiAccountsResponse{
		CustomerID:   cust.ID,
		CustomerName: ds.lookupName(cust.ID),
	}
	for _, a := range cust.Accounts {
		resp.Accounts = append(resp.Accounts, apiAccount{
			ProductName: a.ProductName,
			Family:      string(a.Family),
			Rate:        a.Rate,
			Balance:     a.Balance,
			Interest:    a.Interest,
		})
		if a.Family == gbp.FamilySavings {
			resp.TotalSavings += a.Balance
		} else {
			resp.TotalLending += a.Balance
		}
	}
	return resp
}

func (ds *DemoState) bankAppTransactions(custID string, page int) apiTransactionsResponse {
	entries, total := ds.CustomerTransactions(custID, page, txPerPage)
	apiEntries := make([]apiTxEntry, len(entries))
	for i, tx := range entries {
		apiEntries[i] = apiTxEntry{
			ID:          tx.ID,
			Date:        tx.Date.Format("2006-01-02"),
			ProductName: tx.ProductName,
			Type:        tx.Type.String(),
			Amount:      tx.Amount,
			Balance:     tx.Balance,
			Reference:   tx.Reference,
		}
	}
	return apiTransactionsResponse{
		CustomerID: custID,
		Page:       page,
		TotalCount: total,
		PerPage:    txPerPage,
		Entries:    apiEntries,
	}
}

func (ds *DemoState) bankAppProductDetail(custID string, accountIdx int) *apiProductDetail {
	cust, ok := ds.customerByID(custID)
	if !ok || accountIdx < 0 || accountIdx >= len(cust.Accounts) {
		return nil
	}

	a := cust.Accounts[accountIdx]
	return &apiProductDetail{
		CustomerID:   cust.ID,
		CustomerName: ds.lookupName(cust.ID),
		AccountIndex: accountIdx,
		ProductName:  a.ProductName,
		Family:       string(a.Family),
		Rate:         a.Rate,
		Balance:      a.Balance,
		Interest:     a.Interest,
		SortCode:     a.SortCode,
		AccountNum:   a.AccountNum,
		OpenDate:     a.OpenDate.Format("2006-01-02"),
	}
}

func (ds *DemoState) bankAppProductTransactions(custID string, accountIdx, page int) apiTransactionsResponse {
	entries, total := ds.ProductTransactions(custID, accountIdx, page, txPerPage)
	apiEntries := make([]apiTxEntry, len(entries))
	for i, tx := range entries {
		apiEntries[i] = apiTxEntry{
			ID:          tx.ID,
			Date:        tx.Date.Format("2006-01-02"),
			ProductName: tx.ProductName,
			Type:        tx.Type.String(),
			Amount:      tx.Amount,
			Balance:     tx.Balance,
			Reference:   tx.Reference,
		}
	}
	return apiTransactionsResponse{
		CustomerID: custID,
		Page:       page,
		TotalCount: total,
		PerPage:    txPerPage,
		Entries:    apiEntries,
	}
}
