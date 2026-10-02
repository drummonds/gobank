package bff

import (
	"fmt"
	"git.bytestone.uk/hum3/gobank/core"
	"strconv"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/screen"
)

// Screen paths served by the BFF. Clients never build these; they follow the
// actions in the screens they receive.
const (
	PathLogin         = "/v1/login"
	PathLogout        = "/v1/logout"
	PathScreenLogin   = "/v1/screen/login"
	PathScreenHome    = "/v1/screen/accounts"
	PathScreenTx      = "/v1/screen/activity"
	PathScreenProduct = "/v1/screen/product/" // + index
)

func productPath(index int) string { return PathScreenProduct + strconv.Itoa(index) }

func nav(active string) *screen.Nav {
	return &screen.Nav{Active: active, Tabs: []screen.Tab{
		{Key: "accounts", Label: "Accounts", Icon: screen.IconAccounts, Screen: PathScreenHome},
		{Key: "activity", Label: "Activity", Icon: screen.IconActivity, Screen: PathScreenTx},
	}}
}

// LoginScreen is the public front door. It shows no customer data. notice, if
// set, is shown as an error above the form.
func LoginScreen(notice string) screen.Screen {
	s := screen.New("login", PathScreenLogin, "Model Bank")
	s.Subtitle = "Internet Banking"
	if notice != "" {
		s.Add(screen.Text(notice, screen.ToneDanger))
	}
	s.Add(
		screen.Form("Log in", PathLogin,
			screen.Field{Name: "customer_id", Label: "Customer ID", Kind: "text", Placeholder: "e.g. cust-001", Required: true},
			screen.Field{Name: "password", Label: "Password", Kind: "password", Placeholder: "Password", Required: true},
		),
		screen.Text("Model bank. Customer IDs are listed on the admin Customers page.", screen.ToneMuted),
	)
	return s
}

// AccountsScreen is the home screen after login.
func AccountsScreen(c core.Customer, accounts []core.Account) screen.Screen {
	s := screen.New("accounts", PathScreenHome, c.Name)
	s.Subtitle = c.ID
	var savings, lending luca.Amount
	for _, a := range accounts {
		if a.Family == "Lending" {
			lending += a.Balance
		} else {
			savings += a.Balance
		}
	}
	s.Add(screen.Hero("Net Balance", FormatMoney(savings-lending), screen.ToneSavings,
		screen.Pair{Label: "Savings", Value: FormatMoney(savings)},
		screen.Pair{Label: "Lending", Value: FormatMoney(lending)},
	))
	for _, a := range accounts {
		icon, tone := screen.IconSavings, screen.ToneSavings
		if a.Family == "Lending" {
			icon, tone = screen.IconLending, screen.ToneLending
		}
		s.Add(screen.Row(icon, a.ProductName, fmt.Sprintf("%.2f%% APR", a.Rate*100),
			FormatMoney(a.Balance), "Interest: "+FormatMoney(a.Interest), tone, screen.Go(productPath(a.Index))))
	}
	s.Add(screen.Button("Log out", screen.ToneMuted, screen.Logout()))
	s.Nav = nav("accounts")
	return s
}

// ActivityScreen lists transactions across all of a customer's accounts.
func ActivityScreen(c core.Customer, page core.TransactionPage) screen.Screen {
	s := screen.New("activity", PathScreenTx, "Activity")
	s.Subtitle = c.Name
	addTransactions(&s, page, true)
	if page.HasMore() {
		s.Add(screen.Button("Load more", screen.ToneMuted, screen.Go(fmt.Sprintf("%s?page=%d", PathScreenTx, page.Page+1))))
	}
	s.Nav = nav("activity")
	return s
}

// ProductScreen shows one account with its details and recent activity.
func ProductScreen(c core.Customer, a core.Account, page core.TransactionPage) screen.Screen {
	s := screen.New("product", productPath(a.Index), a.ProductName)
	s.Subtitle = a.Family
	s.Back = screen.Go(PathScreenHome)
	tone := screen.ToneSavings
	if a.Family == "Lending" {
		tone = screen.ToneLending
	}
	s.Add(
		screen.Hero("Balance", FormatMoney(a.Balance), tone,
			screen.Pair{Label: "Interest", Value: FormatMoney(a.Interest)},
			screen.Pair{Label: "APR", Value: fmt.Sprintf("%.2f%%", a.Rate*100)},
		),
		screen.Details("Account Details",
			screen.Pair{Label: "Sort Code", Value: a.SortCode},
			screen.Pair{Label: "Account No.", Value: a.AccountNum},
			screen.Pair{Label: "Opened", Value: a.OpenDate},
			screen.Pair{Label: "Rate", Value: fmt.Sprintf("%.2f%%", a.Rate*100)},
		),
		screen.Heading("Recent Activity"),
	)
	addTransactions(&s, page, false)
	if page.HasMore() {
		s.Add(screen.Button("Load more", screen.ToneMuted, screen.Go(fmt.Sprintf("%s?page=%d", productPath(a.Index), page.Page+1))))
	}
	s.Nav = nav("accounts")
	return s
}

// addTransactions appends transaction rows grouped under date headings. The
// sign, colour and icon of each entry are decided here, not on the device.
func addTransactions(s *screen.Screen, page core.TransactionPage, showProduct bool) {
	if len(page.Entries) == 0 {
		s.Add(screen.Text("No transactions yet.", screen.ToneMuted))
		return
	}
	date := ""
	for _, tx := range page.Entries {
		if tx.Date != date {
			date = tx.Date
			s.Add(screen.Heading(date))
		}
		sign, tone, icon := "+", screen.TonePositive, screen.IconSavings
		switch tx.Type {
		case "Transfer Out":
			sign, tone, icon = "-", screen.ToneNegative, screen.IconOut
		case "Loan Interest":
			sign, tone, icon = "-", screen.ToneNegative, screen.IconInterest
		case "Interest":
			icon = screen.IconInterest
		case "Transfer In":
			icon = screen.IconIn
		case "Loan":
			tone, icon = screen.ToneLending, screen.IconLoan
		}
		sub := tx.Reference
		if showProduct {
			sub = tx.ProductName + " · " + tx.Reference
		}
		s.Add(screen.Tx(icon, tx.Type, sub, sign+FormatMoney(tx.Amount), "Bal: "+FormatMoney(tx.Balance), tone))
	}
}
