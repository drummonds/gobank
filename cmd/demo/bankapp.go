//go:build !(js && wasm)

package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/drummonds/lofigui"
	"github.com/flosch/pongo2/v6"
)

// registerBankAppRoutes wires up the phone UI HTML routes.
func registerBankAppRoutes(state *DemoState, appCtrl *lofigui.Controller) {
	appPage := func(w http.ResponseWriter, content string) {
		appCtrl.RenderTemplate(w, pongo2.Context{
			"results": content,
		})
	}

	http.HandleFunc("/app/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/app/")

		// POST /app/login — simulated login redirect
		if path == "login" && r.Method == "POST" {
			r.ParseForm()
			custID := r.FormValue("customer_id")
			if custID == "" {
				http.Redirect(w, r, "/app/", http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/app/customer/"+custID, http.StatusSeeOther)
			return
		}

		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// /app/ — login screen
		if path == "" {
			appPage(w, state.buildAppLoginHTML())
			return
		}

		// /app/customer/{id}[/sub...]
		if !strings.HasPrefix(path, "customer/") {
			http.NotFound(w, r)
			return
		}
		rest := strings.TrimPrefix(path, "customer/")
		parts := strings.SplitN(rest, "/", 2)
		custID := parts[0]
		if custID == "" {
			http.NotFound(w, r)
			return
		}

		sub := ""
		if len(parts) > 1 {
			sub = parts[1]
		}

		switch {
		case sub == "":
			appPage(w, state.buildAppBalanceHTML(custID))
		case sub == "transactions":
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page < 1 {
				page = 1
			}
			appPage(w, state.buildAppTransactionsHTML(custID, page))
		case strings.HasPrefix(sub, "product/"):
			productRest := strings.TrimPrefix(sub, "product/")
			idx, err := strconv.Atoi(productRest)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			txPage, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if txPage < 1 {
				txPage = 1
			}
			appPage(w, state.buildAppProductHTML(custID, idx, txPage))
		default:
			http.NotFound(w, r)
		}
	})
}
