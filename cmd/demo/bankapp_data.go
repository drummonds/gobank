package main

import (
	gbp "codeberg.org/hum3/gobank-products"
)

// --- API response types (shared by HTML views and JSON API) ---

type apiCustomer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type apiAccount struct {
	ProductName string  `json:"product_name"`
	Family      string  `json:"family"`
	Rate        float64 `json:"rate"`
	Balance     float64 `json:"balance"`
	Interest    float64 `json:"interest"`
}

type apiAccountsResponse struct {
	CustomerID   string       `json:"customer_id"`
	CustomerName string       `json:"customer_name"`
	Accounts     []apiAccount `json:"accounts"`
	TotalSavings float64      `json:"total_savings"`
	TotalLending float64      `json:"total_lending"`
}

type apiTxEntry struct {
	ID          int     `json:"id"`
	Date        string  `json:"date"`
	ProductName string  `json:"product_name"`
	Type        string  `json:"type"`
	Amount      float64 `json:"amount"`
	Balance     float64 `json:"balance"`
	Reference   string  `json:"reference"`
}

type apiTransactionsResponse struct {
	CustomerID string       `json:"customer_id"`
	Page       int          `json:"page"`
	TotalCount int          `json:"total_count"`
	PerPage    int          `json:"per_page"`
	Entries    []apiTxEntry `json:"entries"`
}

type apiProductDetail struct {
	CustomerID   string  `json:"customer_id"`
	CustomerName string  `json:"customer_name"`
	AccountIndex int     `json:"account_index"`
	ProductName  string  `json:"product_name"`
	Family       string  `json:"family"`
	Rate         float64 `json:"rate"`
	Balance      float64 `json:"balance"`
	Interest     float64 `json:"interest"`
	SortCode     string  `json:"sort_code"`
	AccountNum   string  `json:"account_num"`
	OpenDate     string  `json:"open_date"`
}

// --- Service functions (shared by HTML views and JSON API) ---

const txPerPage = 20

func (ds *DemoState) bankAppCustomerList() []apiCustomer {
	ds.mu.Lock()
	customers := make([]CustomerRecord, len(ds.customers))
	copy(customers, ds.customers)
	ds.mu.Unlock()

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
	ds.mu.Lock()
	var cust *CustomerRecord
	for i := range ds.customers {
		if ds.customers[i].ID == custID {
			c := ds.customers[i]
			cust = &c
			break
		}
	}
	ds.mu.Unlock()

	if cust == nil {
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
	ds.mu.Lock()
	var cust *CustomerRecord
	for i := range ds.customers {
		if ds.customers[i].ID == custID {
			c := ds.customers[i]
			cust = &c
			break
		}
	}
	ds.mu.Unlock()

	if cust == nil || accountIdx < 0 || accountIdx >= len(cust.Accounts) {
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
