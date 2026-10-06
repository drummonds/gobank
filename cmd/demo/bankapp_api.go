//go:build !(js && wasm)

package main

import (
	"encoding/json"
	"git.bytestone.uk/hum3/gobank/core"
	"net/http"
	"strconv"
	"strings"
)

// --- JSON HTTP handlers ---

func registerBankAppAPI(bank core.StaffQueries) {
	http.HandleFunc("/api/customers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bankAppCustomerList(bank))
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
		switch {
		case sub == "accounts":
			resp := bankAppAccounts(bank, custID)
			if resp == nil {
				http.NotFound(w, r)
				return
			}
			json.NewEncoder(w).Encode(resp)
		case sub == "transactions":
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page < 1 {
				page = 1
			}
			json.NewEncoder(w).Encode(bankAppTransactions(bank, custID, page))
		case strings.HasPrefix(sub, "product/"):
			productRest := strings.TrimPrefix(sub, "product/")
			productParts := strings.SplitN(productRest, "/", 2)
			idx, err := strconv.Atoi(productParts[0])
			if err != nil {
				http.NotFound(w, r)
				return
			}
			productSub := ""
			if len(productParts) > 1 {
				productSub = productParts[1]
			}
			switch productSub {
			case "":
				resp := bankAppProductDetail(bank, custID, idx)
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
				json.NewEncoder(w).Encode(bankAppProductTransactions(bank, custID, idx, page))
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	})
}
