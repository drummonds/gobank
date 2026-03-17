//go:build !(js && wasm)

package main

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/drummonds/lofigui"
	"github.com/flosch/pongo2/v6"
)

// phoneHeader renders the green header bar inside the phone frame.
func phoneHeader(title, subtitle string) string {
	var s strings.Builder
	s.WriteString(`<div class="phone-header">`)
	fmt.Fprintf(&s, `<p class="title">%s</p>`, title)
	if subtitle != "" {
		fmt.Fprintf(&s, `<p class="subtitle">%s</p>`, subtitle)
	}
	s.WriteString(`</div>`)
	return s.String()
}

// phoneNav renders the bottom tab bar. activeTab: "home", "accounts", "transactions".
func phoneNav(custID, activeTab string) string {
	var s strings.Builder
	s.WriteString(`<div class="phone-nav">`)

	tabs := []struct {
		Icon, Label, Href, Key string
	}{
		{"&#127968;", "Home", "/app/", "home"},
		{"&#128179;", "Accounts", fmt.Sprintf("/app/customer/%s", custID), "accounts"},
		{"&#128196;", "Activity", fmt.Sprintf("/app/customer/%s/transactions", custID), "transactions"},
	}
	for _, t := range tabs {
		cls := ""
		if t.Key == activeTab {
			cls = ` class="is-active"`
		}
		fmt.Fprintf(&s, `<a href="%s"%s><span>%s</span>%s</a>`, t.Href, cls, t.Icon, t.Label)
	}
	s.WriteString(`</div>`)
	return s.String()
}

// --- Screen builders ---

func (ds *DemoState) buildAppLoginHTML() string {
	customers := ds.bankAppCustomerList()

	var s strings.Builder
	s.WriteString(`<div class="phone-header" style="padding:24px 16px 20px;text-align:center">`)
	s.WriteString(`<div style="width:56px;height:56px;border-radius:50%;background:rgba(255,255,255,0.2);margin:0 auto 8px;display:flex;align-items:center;justify-content:center;font-size:1.8rem">&#127974;</div>`)
	s.WriteString(`<p class="title" style="font-size:1.3rem">Model Bank</p>`)
	s.WriteString(`<p class="subtitle" style="font-size:0.75rem">Internet Banking</p>`)
	s.WriteString(`</div>`)
	s.WriteString(`<div class="phone-body">`)

	if len(customers) == 0 {
		s.WriteString(`<p style="color:#7a7a7a;text-align:center;margin-top:2rem">No customers yet. Start the simulation in the admin dashboard.</p>`)
	} else {
		// Login form
		s.WriteString(`<form method="post" action="/app/login" style="margin-bottom:20px">`)
		s.WriteString(`<label style="font-size:0.8rem;color:#4a4a4a;font-weight:600;display:block;margin-bottom:4px">Customer ID</label>`)
		s.WriteString(`<select name="customer_id" style="width:100%;padding:10px 12px;border:1px solid #dbdbdb;border-radius:8px;font-size:0.9rem;margin-bottom:12px;background:#fff">`)
		limit := len(customers)
		if limit > 50 {
			limit = 50
		}
		for _, c := range customers[:limit] {
			fmt.Fprintf(&s, `<option value="%s">%s</option>`, c.ID, c.Name)
		}
		s.WriteString(`</select>`)
		s.WriteString(`<label style="font-size:0.8rem;color:#4a4a4a;font-weight:600;display:block;margin-bottom:4px">PIN</label>`)
		s.WriteString(`<input type="password" name="pin" maxlength="6" placeholder="&#8226;&#8226;&#8226;&#8226;&#8226;&#8226;" style="width:100%;padding:10px 12px;border:1px solid #dbdbdb;border-radius:8px;font-size:1.1rem;letter-spacing:6px;text-align:center;margin-bottom:16px;box-sizing:border-box">`)
		s.WriteString(`<button type="submit" style="width:100%;padding:12px;background:#00947e;color:#fff;border:none;border-radius:8px;font-size:1rem;font-weight:600;cursor:pointer">Log In</button>`)
		s.WriteString(`</form>`)

		s.WriteString(`<p style="text-align:center;color:#b5b5b5;font-size:0.75rem;margin:12px 0">&#8212; or select a demo account &#8212;</p>`)

		// Quick-access customer cards
		for _, c := range customers[:limit] {
			fmt.Fprintf(&s, `<a href="/app/customer/%s" style="display:block;text-decoration:none;color:inherit;margin-bottom:8px">
  <div style="background:#f5f5f5;border-radius:10px;padding:12px 16px;display:flex;align-items:center">
    <div style="width:36px;height:36px;border-radius:50%%;background:#00947e;color:#fff;display:flex;align-items:center;justify-content:center;font-weight:bold;font-size:0.9rem;margin-right:12px">%s</div>
    <div style="flex:1"><strong>%s</strong><br><span style="font-size:0.75rem;color:#7a7a7a">%s</span></div>
    <span style="color:#b5b5b5;font-size:1.2rem">&#8250;</span>
  </div>
</a>`, c.ID, string(c.Name[0]), c.Name, c.ID)
		}
		if len(customers) > limit {
			fmt.Fprintf(&s, `<p style="color:#7a7a7a;text-align:center;font-size:0.8rem">Showing %d of %d customers</p>`, limit, len(customers))
		}
	}
	s.WriteString(`</div>`)
	return s.String()
}

func (ds *DemoState) buildAppBalanceHTML(custID string) string {
	resp := ds.bankAppAccounts(custID)
	if resp == nil {
		return phoneHeader("Model Bank", "") +
			`<div class="phone-body"><p style="color:#cc0000;text-align:center;margin-top:2rem">Customer not found.</p></div>`
	}

	var s strings.Builder
	s.WriteString(phoneHeader(resp.CustomerName, resp.CustomerID))
	s.WriteString(`<div class="phone-body">`)

	// Total balance card
	netBalance := resp.TotalSavings - resp.TotalLending
	balColor := "#00947e"
	if netBalance < 0 {
		balColor = "#cc0000"
	}
	fmt.Fprintf(&s, `<div style="background:linear-gradient(135deg,#00947e,#00b89c);border-radius:14px;padding:20px;color:#fff;margin-bottom:16px">
  <p style="font-size:0.8rem;opacity:0.8;margin:0">Net Balance</p>
  <p style="font-size:1.8rem;font-weight:bold;margin:4px 0">%s</p>
  <div style="display:flex;justify-content:space-between;font-size:0.75rem;opacity:0.85;margin-top:8px">
    <span>Savings: %s</span><span>Lending: %s</span>
  </div>
</div>`, fmtMoney(netBalance), fmtMoney(resp.TotalSavings), fmtMoney(resp.TotalLending))
	_ = balColor

	// Per-account cards (clickable to product detail)
	for i, a := range resp.Accounts {
		familyIcon := "&#128178;" // savings
		familyColor := "#48c78e"
		if a.Family == "Lending" {
			familyIcon = "&#128179;"
			familyColor = "#3e8ed0"
		}
		fmt.Fprintf(&s, `<a href="/app/customer/%s/product/%d" style="display:block;text-decoration:none;color:inherit;margin-bottom:8px">
<div style="background:#fafafa;border-radius:10px;padding:14px 16px;border-left:4px solid %s">
  <div style="display:flex;justify-content:space-between;align-items:center">
    <div><span>%s</span> <strong>%s</strong><br><span style="font-size:0.75rem;color:#7a7a7a">%.2f%% APR</span></div>
    <div style="display:flex;align-items:center">
      <div style="text-align:right;margin-right:8px"><strong>%s</strong><br><span style="font-size:0.7rem;color:#7a7a7a">Interest: %s</span></div>
      <span style="color:#b5b5b5;font-size:1.2rem">&#8250;</span>
    </div>
  </div>
</div>
</a>`, custID, i, familyColor, familyIcon, a.ProductName, a.Rate*100, fmtMoney(a.Balance), fmtMoney(a.Interest))
	}

	s.WriteString(`</div>`)
	s.WriteString(phoneNav(custID, "accounts"))
	return s.String()
}

func (ds *DemoState) buildAppTransactionsHTML(custID string, page int) string {
	resp := ds.bankAppAccounts(custID)
	if resp == nil {
		return phoneHeader("Model Bank", "") +
			`<div class="phone-body"><p style="color:#cc0000;text-align:center;margin-top:2rem">Customer not found.</p></div>`
	}

	txResp := ds.bankAppTransactions(custID, page)

	var s strings.Builder
	s.WriteString(phoneHeader("Activity", resp.CustomerName))
	s.WriteString(`<div class="phone-body" id="tx-body">`)

	if len(txResp.Entries) == 0 {
		s.WriteString(`<p style="color:#7a7a7a;text-align:center;margin-top:2rem">No transactions yet.</p>`)
	} else {
		currentDate := ""
		for _, tx := range txResp.Entries {
			if tx.Date != currentDate {
				currentDate = tx.Date
				fmt.Fprintf(&s, `<p style="font-size:0.75rem;color:#7a7a7a;margin:12px 0 4px;font-weight:bold">%s</p>`, tx.Date)
			}
			// Amount styling
			amtPrefix := "+"
			amtColor := "#48c78e"
			switch tx.Type {
			case "Transfer Out", "Loan Interest":
				amtPrefix = "-"
				amtColor = "#f14668"
			case "Interest", "Transfer In", "Deposit":
				amtPrefix = "+"
				amtColor = "#48c78e"
			case "Loan":
				amtPrefix = "+"
				amtColor = "#3e8ed0"
			}
			// Icon
			icon := "&#128178;"
			switch tx.Type {
			case "Transfer Out":
				icon = "&#8593;" // up arrow
			case "Transfer In":
				icon = "&#8595;" // down arrow
			case "Interest", "Loan Interest":
				icon = "&#9733;" // star
			case "Loan":
				icon = "&#127974;" // bank
			}
			fmt.Fprintf(&s, `<div style="display:flex;align-items:center;padding:8px 0;border-bottom:1px solid #f0f0f0">
  <div style="width:32px;height:32px;border-radius:50%%;background:#f0f0f0;display:flex;align-items:center;justify-content:center;margin-right:10px;font-size:0.9rem">%s</div>
  <div style="flex:1">
    <div style="display:flex;justify-content:space-between">
      <strong style="font-size:0.85rem">%s</strong>
      <span style="color:%s;font-weight:bold;font-size:0.85rem">%s%s</span>
    </div>
    <div style="display:flex;justify-content:space-between;font-size:0.7rem;color:#7a7a7a">
      <span>%s &middot; %s</span>
      <span>Bal: %s</span>
    </div>
  </div>
</div>`, icon, tx.Type, amtColor, amtPrefix, fmtMoney(tx.Amount), tx.ProductName, tx.Reference, fmtMoney(tx.Balance))
		}

		// Load more button via HTMX
		totalPages := (txResp.TotalCount + txPerPage - 1) / txPerPage
		if page < totalPages {
			fmt.Fprintf(&s, `<div style="text-align:center;margin:16px 0">
  <a href="/app/customer/%s/transactions?page=%d" style="color:#00947e;font-size:0.85rem;text-decoration:none">Load more</a>
</div>`, custID, page+1)
		}
	}

	s.WriteString(`</div>`)
	s.WriteString(phoneNav(custID, "transactions"))
	return s.String()
}

func (ds *DemoState) buildAppProductHTML(custID string, accountIdx, txPage int) string {
	detail := ds.bankAppProductDetail(custID, accountIdx)
	if detail == nil {
		return phoneHeader("Model Bank", "") +
			`<div class="phone-body"><p style="color:#cc0000;text-align:center;margin-top:2rem">Product not found.</p></div>`
	}

	var s strings.Builder

	// Header with back link
	s.WriteString(`<div class="phone-header" style="text-align:left;padding:8px 16px 12px">`)
	fmt.Fprintf(&s, `<a href="/app/customer/%s" style="color:rgba(255,255,255,0.8);text-decoration:none;font-size:0.8rem">&#8249; Back</a>`, custID)
	fmt.Fprintf(&s, `<p class="title" style="margin-top:4px">%s</p>`, detail.ProductName)
	fmt.Fprintf(&s, `<p class="subtitle">%s</p>`, detail.Family)
	s.WriteString(`</div>`)
	s.WriteString(`<div class="phone-body">`)

	// Balance hero
	gradStart, gradEnd := "#00947e", "#00b89c"
	if detail.Family == "Lending" {
		gradStart, gradEnd = "#3e8ed0", "#5ea8e5"
	}
	fmt.Fprintf(&s, `<div style="background:linear-gradient(135deg,%s,%s);border-radius:14px;padding:20px;color:#fff;margin-bottom:16px">
  <p style="font-size:0.8rem;opacity:0.8;margin:0">Balance</p>
  <p style="font-size:1.8rem;font-weight:bold;margin:4px 0">%s</p>
  <div style="display:flex;justify-content:space-between;font-size:0.75rem;opacity:0.85;margin-top:8px">
    <span>Interest: %s</span><span>%.2f%% APR</span>
  </div>
</div>`, gradStart, gradEnd, fmtMoney(detail.Balance), fmtMoney(detail.Interest), detail.Rate*100)

	// Account details card
	s.WriteString(`<div style="background:#fafafa;border-radius:10px;padding:14px 16px;margin-bottom:16px">`)
	s.WriteString(`<p style="font-size:0.85rem;font-weight:600;margin-bottom:8px">Account Details</p>`)
	fmt.Fprintf(&s, `<div style="display:flex;justify-content:space-between;font-size:0.8rem;padding:6px 0;border-bottom:1px solid #eee"><span style="color:#7a7a7a">Sort Code</span><span style="font-family:monospace">%s</span></div>`, detail.SortCode)
	fmt.Fprintf(&s, `<div style="display:flex;justify-content:space-between;font-size:0.8rem;padding:6px 0;border-bottom:1px solid #eee"><span style="color:#7a7a7a">Account No.</span><span style="font-family:monospace">%s</span></div>`, detail.AccountNum)
	fmt.Fprintf(&s, `<div style="display:flex;justify-content:space-between;font-size:0.8rem;padding:6px 0;border-bottom:1px solid #eee"><span style="color:#7a7a7a">Opened</span><span>%s</span></div>`, detail.OpenDate)
	fmt.Fprintf(&s, `<div style="display:flex;justify-content:space-between;font-size:0.8rem;padding:6px 0"><span style="color:#7a7a7a">Rate</span><span>%.2f%%</span></div>`, detail.Rate*100)
	s.WriteString(`</div>`)

	// Transactions for this product
	if txPage < 1 {
		txPage = 1
	}
	txResp := ds.bankAppProductTransactions(custID, accountIdx, txPage)

	s.WriteString(`<p style="font-size:0.85rem;font-weight:600;margin-bottom:8px">Recent Activity</p>`)

	if len(txResp.Entries) == 0 {
		s.WriteString(`<p style="color:#7a7a7a;text-align:center;margin-top:1rem;font-size:0.85rem">No transactions yet.</p>`)
	} else {
		currentDate := ""
		for _, tx := range txResp.Entries {
			if tx.Date != currentDate {
				currentDate = tx.Date
				fmt.Fprintf(&s, `<p style="font-size:0.75rem;color:#7a7a7a;margin:12px 0 4px;font-weight:bold">%s</p>`, tx.Date)
			}
			amtPrefix := "+"
			amtColor := "#48c78e"
			switch tx.Type {
			case "Transfer Out", "Loan Interest":
				amtPrefix = "-"
				amtColor = "#f14668"
			case "Interest", "Transfer In", "Deposit":
				amtPrefix = "+"
				amtColor = "#48c78e"
			case "Loan":
				amtPrefix = "+"
				amtColor = "#3e8ed0"
			}
			icon := "&#128178;"
			switch tx.Type {
			case "Transfer Out":
				icon = "&#8593;"
			case "Transfer In":
				icon = "&#8595;"
			case "Interest", "Loan Interest":
				icon = "&#9733;"
			case "Loan":
				icon = "&#127974;"
			}
			fmt.Fprintf(&s, `<div style="display:flex;align-items:center;padding:8px 0;border-bottom:1px solid #f0f0f0">
  <div style="width:32px;height:32px;border-radius:50%%;background:#f0f0f0;display:flex;align-items:center;justify-content:center;margin-right:10px;font-size:0.9rem">%s</div>
  <div style="flex:1">
    <div style="display:flex;justify-content:space-between">
      <strong style="font-size:0.85rem">%s</strong>
      <span style="color:%s;font-weight:bold;font-size:0.85rem">%s%s</span>
    </div>
    <div style="display:flex;justify-content:space-between;font-size:0.7rem;color:#7a7a7a">
      <span>%s</span>
      <span>Bal: %s</span>
    </div>
  </div>
</div>`, icon, tx.Type, amtColor, amtPrefix, fmtMoney(tx.Amount), tx.Reference, fmtMoney(tx.Balance))
		}

		totalPages := (txResp.TotalCount + txPerPage - 1) / txPerPage
		if txPage < totalPages {
			fmt.Fprintf(&s, `<div style="text-align:center;margin:16px 0">
  <a href="/app/customer/%s/product/%d?page=%d" style="color:#00947e;font-size:0.85rem;text-decoration:none">Load more</a>
</div>`, custID, accountIdx, txPage+1)
		}
	}

	s.WriteString(`</div>`)
	s.WriteString(phoneNav(custID, "accounts"))
	return s.String()
}

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
