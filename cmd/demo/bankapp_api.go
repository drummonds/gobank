//go:build !(js && wasm)

package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	gbp "codeberg.org/hum3/gobank-products"
)

// --- API response types ---

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

// --- Service functions (shared by HTML views and JSON API) ---

const txPerPage = 20

func (ds *DemoState) bankAppCustomerList() []apiCustomer {
	ds.mu.Lock()
	customers := make([]CustomerRecord, len(ds.customers))
	copy(customers, ds.customers)
	piiStore := ds.piiStore
	ds.mu.Unlock()

	result := make([]apiCustomer, len(customers))
	for i, c := range customers {
		result[i] = apiCustomer{
			ID:   c.ID,
			Name: piiStore.RetrieveName(c.ID),
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
	piiStore := ds.piiStore
	ds.mu.Unlock()

	if cust == nil {
		return nil
	}

	resp := &apiAccountsResponse{
		CustomerID:   cust.ID,
		CustomerName: piiStore.RetrieveName(cust.ID),
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

// --- JSON HTTP handlers ---

func registerBankAppAPI(state *DemoState) {
	http.HandleFunc("/api/customers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state.bankAppCustomerList())
	})

	http.HandleFunc("/api/customer/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Parse: /api/customer/{id}/accounts or /api/customer/{id}/transactions
		path := strings.TrimPrefix(r.URL.Path, "/api/customer/")
		parts := strings.SplitN(path, "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}
		custID := parts[0]
		sub := ""
		if len(parts) > 1 {
			sub = parts[1]
		}

		w.Header().Set("Content-Type", "application/json")
		switch sub {
		case "accounts":
			resp := state.bankAppAccounts(custID)
			if resp == nil {
				http.NotFound(w, r)
				return
			}
			json.NewEncoder(w).Encode(resp)
		case "transactions":
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page < 1 {
				page = 1
			}
			json.NewEncoder(w).Encode(state.bankAppTransactions(custID, page))
		default:
			http.NotFound(w, r)
		}
	})
}
