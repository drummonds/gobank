package coretest

import (
	"context"
	"errors"
	"testing"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/core"
)

// runStaff checks the StaffQueries contract: the staff view of the bank
// agrees with itself and with what each customer sees.
func runStaff(t *testing.T, f Fixture) {
	ctx := context.Background()
	s := f.Staff

	t.Run("register pages cover every customer", func(t *testing.T) {
		first, err := s.CustomerPage(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if first.Page != 1 || first.PerPage <= 0 || first.Total <= 0 {
			t.Fatalf("page 1 = {Page %d PerPage %d Total %d}", first.Page, first.PerPage, first.Total)
		}
		seen, found := 0, false
		for n, page := 1, first; ; n++ {
			for _, c := range page.Customers {
				seen++
				if c.ID == f.CustomerID {
					found = true
				}
				accts, err := s.Accounts(ctx, c.ID)
				if err != nil {
					t.Fatalf("Accounts(%s): %v", c.ID, err)
				}
				var savings, lending luca.Amount
				for _, a := range accts {
					if a.Family == "Savings" {
						savings += a.Balance
					} else {
						lending += a.Balance
					}
				}
				if c.Accounts != len(accts) || c.Savings != savings || c.Lending != lending {
					t.Errorf("register row %+v disagrees with the customer's accounts (%d, %d, %d)", c, len(accts), savings, lending)
				}
			}
			if n >= page.TotalPages() {
				break
			}
			if page, err = s.CustomerPage(ctx, n+1); err != nil {
				t.Fatal(err)
			}
		}
		if seen != first.Total || !found {
			t.Errorf("pages hold %d customers, Total says %d; fixture customer listed: %v", seen, first.Total, found)
		}
		beyond, err := s.CustomerPage(ctx, first.TotalPages()+1)
		if err != nil || len(beyond.Customers) != 0 {
			t.Errorf("page beyond the last = %+v, %v; want empty", beyond, err)
		}
	})

	t.Run("record agrees with the customer view", func(t *testing.T) {
		rec, err := s.CustomerRecord(ctx, f.CustomerID)
		if err != nil {
			t.Fatal(err)
		}
		if rec.ID != f.CustomerID || rec.JoinDate.IsZero() || rec.KYC.RiskRating == "" {
			t.Errorf("CustomerRecord = %+v, want ID, join date and a KYC rating", rec)
		}
		accts, err := s.Accounts(ctx, f.CustomerID)
		if err != nil {
			t.Fatal(err)
		}
		if len(rec.Accounts) != len(accts) {
			t.Fatalf("record lists %d accounts, customer sees %d", len(rec.Accounts), len(accts))
		}
		for i := range accts {
			if rec.Accounts[i] != accts[i] {
				t.Errorf("account %d: record %+v, customer %+v", i, rec.Accounts[i], accts[i])
			}
		}
		name, err := s.CustomerName(ctx, f.CustomerID)
		cust, _ := s.Customer(ctx, f.CustomerID)
		if err != nil || name != cust.Name {
			t.Errorf("CustomerName = %q, %v; Customer().Name = %q", name, err, cust.Name)
		}
		pii, err := s.CustomerPII(ctx, f.CustomerID)
		if err != nil || pii.Name != name {
			t.Errorf("CustomerPII = %+v, %v; want the same name", pii, err)
		}
	})

	t.Run("unknown customer is ErrNotFound", func(t *testing.T) {
		_, err := s.CustomerRecord(ctx, UnknownCustomerID)
		wantNotFound(t, "CustomerRecord", err)
		_, err = s.CustomerName(ctx, UnknownCustomerID)
		wantNotFound(t, "CustomerName", err)
		_, err = s.CustomerPII(ctx, UnknownCustomerID)
		wantNotFound(t, "CustomerPII", err)
		_, err = s.PaymentsOf(ctx, UnknownCustomerID)
		wantNotFound(t, "PaymentsOf", err)
	})

	t.Run("position is the sum of the register", func(t *testing.T) {
		pos, err := s.Position(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var savings, lending luca.Amount
		total := 0
		for n := 1; ; n++ {
			page, err := s.CustomerPage(ctx, n)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range page.Customers {
				savings += c.Savings
				lending += c.Lending
			}
			total = page.Total
			if n >= page.TotalPages() {
				break
			}
		}
		if pos.Customers != total || pos.Savings != savings || pos.Lending != lending {
			t.Errorf("Position = %+v; register totals %d customers, savings %d, lending %d", pos, total, savings, lending)
		}
		if pos.Cash != pos.Savings-pos.Lending || pos.Day.IsZero() || pos.ReserveRatio <= 0 {
			t.Errorf("Position = %+v: cash must be deposits less loans, with a day and a reserve ratio", pos)
		}
	})

	t.Run("statements balance", func(t *testing.T) {
		pos, _ := s.Position(ctx)
		pl, err := s.ProfitAndLoss(ctx)
		if err != nil || pl.DayCount != pos.DayCount {
			t.Errorf("ProfitAndLoss = %+v, %v; want day %d", pl, err, pos.DayCount)
		}
		bs, err := s.BalanceSheet(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if bs.Deposits != pos.Savings || bs.Loans != pos.Lending || bs.RetainedEarnings != pl.NetProfit() {
			t.Errorf("BalanceSheet = %+v disagrees with position %+v and P&L %+v", bs, pos, pl)
		}
		if bs.CashAtBoE > 0 && bs.TotalAssets() != bs.Deposits+bs.RetainedEarnings {
			t.Errorf("assets %d != liabilities %d + equity %d", bs.TotalAssets(), bs.Deposits, bs.RetainedEarnings)
		}
	})

	t.Run("history is in date order", func(t *testing.T) {
		h, err := s.History(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(h.Balances) == 0 || len(h.Customers) == 0 || len(h.BoERate) == 0 {
			t.Errorf("History has empty series: %d balances, %d customers, %d rates", len(h.Balances), len(h.Customers), len(h.BoERate))
		}
		for i := 1; i < len(h.Balances); i++ {
			if h.Balances[i].Date.Before(h.Balances[i-1].Date) {
				t.Errorf("balances out of order at %d", i)
			}
		}
		for i := 1; i < len(h.BoERate); i++ {
			if h.BoERate[i].Date.Before(h.BoERate[i-1].Date) {
				t.Errorf("rates out of order at %d", i)
			}
		}
	})

	t.Run("products carry the book", func(t *testing.T) {
		products, err := s.Products(ctx)
		if err != nil || len(products) == 0 {
			t.Fatalf("Products = %v, %v", products, err)
		}
		pos, _ := s.Position(ctx)
		var savings, lending luca.Amount
		accounts := 0
		for _, p := range products {
			if p.ID == "" || p.Name == "" || (p.Family != "Savings" && p.Family != "Lending") {
				t.Errorf("product %+v needs an ID, a name and a family", p)
			}
			accounts += p.Accounts
			if p.Family == "Savings" {
				savings += p.Balance
			} else {
				lending += p.Balance
			}
		}
		if savings != pos.Savings || lending != pos.Lending {
			t.Errorf("products total savings %d, lending %d; position %+v", savings, lending, pos)
		}
		var registered int
		for n := 1; ; n++ {
			page, _ := s.CustomerPage(ctx, n)
			for _, c := range page.Customers {
				registered += c.Accounts
			}
			if n >= page.TotalPages() {
				break
			}
		}
		if accounts != registered {
			t.Errorf("products count %d accounts, register %d", accounts, registered)
		}
	})

	t.Run("payments page newest first", func(t *testing.T) {
		page, err := s.PaymentPage(ctx, 1)
		if err != nil || page.Page != 1 || page.PerPage <= 0 {
			t.Fatalf("PaymentPage(1) = %+v, %v", page, err)
		}
		for i, p := range page.Payments {
			if i > 0 && p.ID > page.Payments[i-1].ID {
				t.Errorf("payments not newest first at %d", i)
			}
			got, err := s.Payment(ctx, p.ID)
			if err != nil || got != p {
				t.Errorf("Payment(%d) = %+v, %v; page has %+v", p.ID, got, err, p)
			}
			if p.Reference == "" || p.Type == "" || p.Status == "" || p.CreatedAt.IsZero() {
				t.Errorf("payment %+v is missing a field", p)
			}
		}
		if _, err := s.Payment(ctx, -1); !errors.Is(err, core.ErrNotFound) {
			t.Errorf("Payment(-1) err = %v, want ErrNotFound", err)
		}
		of, err := s.PaymentsOf(ctx, f.CustomerID)
		if err != nil {
			t.Fatal(err)
		}
		for i, p := range of {
			if p.From != f.CustomerID && p.To != f.CustomerID {
				t.Errorf("PaymentsOf lists %+v, which is not the customer's", p)
			}
			if i > 0 && p.ID < of[i-1].ID {
				t.Errorf("PaymentsOf not oldest first at %d", i)
			}
		}
	})

	t.Run("savings interest names known customers", func(t *testing.T) {
		rows, err := s.SavingsInterestByCustomer(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.Interest <= 0 {
				t.Errorf("row %+v: interest must be positive", r)
			}
			if _, err := s.CustomerRecord(ctx, r.CustomerID); err != nil {
				t.Errorf("row %+v names an unknown customer: %v", r, err)
			}
		}
	})

	t.Run("gilt yields are quoted", func(t *testing.T) {
		yields, err := s.GiltYields(ctx)
		if err != nil || len(yields) == 0 {
			t.Fatalf("GiltYields = %v, %v", yields, err)
		}
		for _, y := range yields {
			if y.Tenor == "" || y.Rate <= 0 {
				t.Errorf("yield %+v needs a tenor and a positive rate", y)
			}
		}
		if _, err := s.GiltHoldings(ctx); err != nil {
			t.Errorf("GiltHoldings: %v", err)
		}
	})
}

// runCommands checks the Commands contract: what a command changes is
// what the queries then show.
func runCommands(t *testing.T, f Fixture) {
	ctx := context.Background()
	c, s := f.Commands, f.Staff
	if s == nil {
		t.Fatal("Commands need Staff to observe their effect")
	}
	if f.OtherCustomerID == "" {
		t.Fatal("Commands need OtherCustomerID")
	}

	firstSavings := func(t *testing.T, id string) core.Account {
		t.Helper()
		accts, err := s.Accounts(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range accts {
			if a.Family == "Savings" {
				return a
			}
		}
		t.Fatalf("fixture customer %s holds no savings account", id)
		return core.Account{}
	}

	t.Run("transfer moves the money and is on record", func(t *testing.T) {
		from, to := firstSavings(t, f.CustomerID), firstSavings(t, f.OtherCustomerID)
		if from.Balance <= 0 {
			t.Fatalf("fixture sender %s has no savings balance", f.CustomerID)
		}
		amount := min(from.Balance, 12_34)
		p, err := c.Transfer(ctx, core.Transfer{From: f.CustomerID, To: f.OtherCustomerID, Amount: amount})
		if err != nil {
			t.Fatal(err)
		}
		if p.Reference == "" || p.Type != core.PaymentTransfer || p.From != f.CustomerID || p.To != f.OtherCustomerID || p.Amount != amount {
			t.Errorf("Transfer returned %+v", p)
		}
		if got, err := s.Payment(ctx, p.ID); err != nil || got.Reference != p.Reference {
			t.Errorf("Payment(%d) = %+v, %v; want the transfer", p.ID, got, err)
		}
		if after := firstSavings(t, f.CustomerID); after.Balance != from.Balance-amount {
			t.Errorf("sender balance %d, want %d", after.Balance, from.Balance-amount)
		}
		if after := firstSavings(t, f.OtherCustomerID); after.Balance != to.Balance+amount {
			t.Errorf("recipient balance %d, want %d", after.Balance, to.Balance+amount)
		}
		page, err := s.Transactions(ctx, f.CustomerID, 1)
		if err != nil || len(page.Entries) == 0 {
			t.Fatalf("Transactions after transfer = %+v, %v", page, err)
		}
		if tx := page.Entries[0]; tx.Type != "Transfer Out" || tx.Reference != p.Reference || tx.Amount != amount {
			t.Errorf("newest sender transaction %+v, want the transfer out", tx)
		}
		page, _ = s.Transactions(ctx, f.OtherCustomerID, 1)
		if len(page.Entries) == 0 || page.Entries[0].Type != "Transfer In" || page.Entries[0].Reference != p.Reference {
			t.Errorf("newest recipient transaction %+v, want the transfer in", page.Entries)
		}
	})

	t.Run("transfer refuses what the bank cannot do", func(t *testing.T) {
		from := firstSavings(t, f.CustomerID)
		for _, tc := range []struct {
			name string
			t    core.Transfer
			err  error
		}{
			{"zero", core.Transfer{From: f.CustomerID, To: f.OtherCustomerID, Amount: 0}, core.ErrInvalidAmount},
			{"negative", core.Transfer{From: f.CustomerID, To: f.OtherCustomerID, Amount: -1}, core.ErrInvalidAmount},
			{"same customer", core.Transfer{From: f.CustomerID, To: f.CustomerID, Amount: 1}, core.ErrSameCustomer},
			{"unknown sender", core.Transfer{From: UnknownCustomerID, To: f.OtherCustomerID, Amount: 1}, core.ErrNotFound},
			{"unknown recipient", core.Transfer{From: f.CustomerID, To: UnknownCustomerID, Amount: 1}, core.ErrNotFound},
			{"over the balance", core.Transfer{From: f.CustomerID, To: f.OtherCustomerID, Amount: from.Balance + 1}, core.ErrInsufficientFunds},
		} {
			if _, err := c.Transfer(ctx, tc.t); !errors.Is(err, tc.err) {
				t.Errorf("%s: err = %v, want %v", tc.name, err, tc.err)
			}
		}
		if after := firstSavings(t, f.CustomerID); after.Balance != from.Balance {
			t.Errorf("a refused transfer moved money: %d -> %d", from.Balance, after.Balance)
		}
	})

	t.Run("open customer puts them on the register, funded", func(t *testing.T) {
		products, err := s.Products(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var savings, lending core.Product
		for _, p := range products {
			switch {
			case p.Family == "Savings" && savings.ID == "":
				savings = p
			case p.Family == "Lending" && lending.ID == "":
				lending = p
			}
		}
		if savings.ID == "" || lending.ID == "" {
			t.Fatalf("fixture catalogue needs a savings and a lending product, got %+v", products)
		}
		before, _ := s.Position(ctx)
		register, _ := s.CustomerPage(ctx, 1)

		const deposit, loan luca.Amount = 500_00, 1_000_00
		rec, err := c.OpenCustomer(ctx, core.NewCustomer{
			KYC:      core.KYC{Verified: true, LastCheck: before.Day, RiskRating: "Low"},
			PII:      core.PII{Name: "Opened By Contract", NI: "QQ123456C", DOB: "1980-01-02", Address: "1 Test Street", Email: "opened@example.com", Phone: "07000 000000"},
			Accounts: []core.NewAccount{{ProductID: savings.ID, Opening: deposit}, {ProductID: lending.ID, Opening: loan}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if rec.ID == "" || !rec.JoinDate.Equal(before.Day) || !rec.KYC.Verified || rec.KYC.RiskRating != "Low" {
			t.Errorf("OpenCustomer returned %+v; want an ID, joined on the business day %s, with the KYC given", rec, before.Day.Format("2006-01-02"))
		}
		if len(rec.Accounts) != 2 {
			t.Fatalf("OpenCustomer returned %d accounts, want 2", len(rec.Accounts))
		}
		sv, ln := rec.Accounts[0], rec.Accounts[1]
		// The day's interest accrues on the balance from the day the account
		// opens; nothing has been applied yet.
		if sv.ProductName != savings.Name || sv.Balance != deposit || sv.Interest != 0 || sv.AccruedE7 < 0 || sv.SortCode == "" || sv.AccountNum == "" || sv.OpenDate != before.Day.Format("2006-01-02") {
			t.Errorf("savings account %+v; want %s holding the deposit %d, nothing applied, opened today", sv, savings.Name, deposit)
		}
		if ln.ProductName != lending.Name || ln.Balance < 0 || ln.Balance > loan || ln.Interest != 0 {
			t.Errorf("lending account %+v; want %s lent at most %d", ln, lending.Name, loan)
		}
		got, err := s.CustomerRecord(ctx, rec.ID)
		if err != nil || got.ID != rec.ID || len(got.Accounts) != 2 || got.Accounts[0].Balance != deposit || got.Accounts[1].Balance != ln.Balance {
			t.Errorf("CustomerRecord(%s) = %+v, %v; want what OpenCustomer returned", rec.ID, got, err)
		}
		if pii, err := s.CustomerPII(ctx, rec.ID); err != nil || pii.Name != "Opened By Contract" || pii.NI != "QQ123456C" {
			t.Errorf("CustomerPII(%s) = %+v, %v; want the PII given", rec.ID, pii, err)
		}
		if name, _ := s.CustomerName(ctx, rec.ID); name != "Opened By Contract" {
			t.Errorf("CustomerName(%s) = %q", rec.ID, name)
		}
		payments, _ := s.PaymentsOf(ctx, rec.ID)
		var deposits, loans int
		for _, p := range payments {
			switch {
			case p.Type == core.PaymentDeposit && p.To == rec.ID && p.Amount == deposit && p.Status == core.PaymentCompleted:
				deposits++
			case p.Type == core.PaymentLoan && p.To == rec.ID && p.Amount == ln.Balance && p.Status == core.PaymentCompleted:
				loans++
			}
		}
		wantLoans := 0
		if ln.Balance > 0 {
			wantLoans = 1
		}
		if deposits != 1 || loans != wantLoans || len(payments) != 1+wantLoans {
			t.Errorf("PaymentsOf(%s) = %+v; want one completed deposit of %d and %d loan payment(s)", rec.ID, payments, deposit, wantLoans)
		}
		page, err := s.Transactions(ctx, rec.ID, 1)
		if err != nil || len(page.Entries) != 1+wantLoans {
			t.Fatalf("Transactions(%s) = %+v, %v; want the opening fundings", rec.ID, page, err)
		}
		var sawDeposit bool
		for _, tx := range page.Entries {
			if tx.Type == "Deposit" && tx.Amount == deposit && tx.ProductName == savings.Name && tx.Balance == deposit {
				sawDeposit = true
			}
		}
		if !sawDeposit {
			t.Errorf("Transactions(%s) = %+v; want the deposit on the savings account", rec.ID, page.Entries)
		}
		after, _ := s.Position(ctx)
		if after.Customers != before.Customers+1 || after.Savings != before.Savings+deposit || after.Lending != before.Lending+ln.Balance {
			t.Errorf("position after opening = %+v; want one more customer, savings +%d, lending +%d on %+v", after, deposit, ln.Balance, before)
		}
		if again, _ := s.CustomerPage(ctx, 1); again.Total != register.Total+1 {
			t.Errorf("register total %d after opening, want %d", again.Total, register.Total+1)
		}
	})

	t.Run("open customer refuses what the bank cannot do", func(t *testing.T) {
		products, _ := s.Products(ctx)
		register, _ := s.CustomerPage(ctx, 1)
		pii := core.PII{Name: "Refused"}
		for _, tc := range []struct {
			name string
			c    core.NewCustomer
			err  error
		}{
			{"no accounts", core.NewCustomer{PII: pii}, core.ErrInvalidAmount},
			{"negative opening", core.NewCustomer{PII: pii, Accounts: []core.NewAccount{{ProductID: products[0].ID, Opening: -1}}}, core.ErrInvalidAmount},
			{"unknown product", core.NewCustomer{PII: pii, Accounts: []core.NewAccount{{ProductID: "no-such-product", Opening: 1_00}}}, core.ErrNotFound},
		} {
			if _, err := c.OpenCustomer(ctx, tc.c); !errors.Is(err, tc.err) {
				t.Errorf("%s: err = %v, want %v", tc.name, err, tc.err)
			}
		}
		if again, _ := s.CustomerPage(ctx, 1); again.Total != register.Total {
			t.Errorf("a refused customer is on the register: total %d -> %d", register.Total, again.Total)
		}
	})

	t.Run("start day moves the bank on a business day", func(t *testing.T) {
		before, _ := s.Position(ctx)
		day, err := c.StartDay(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if want := before.Day.AddDate(0, 0, 1); !day.Equal(want) {
			t.Errorf("StartDay returned %s, want %s", day.Format("2006-01-02"), want.Format("2006-01-02"))
		}
		after, _ := s.Position(ctx)
		if !after.Day.Equal(day) || after.DayCount != before.DayCount+1 || after.Customers != before.Customers {
			t.Errorf("position after StartDay = %+v; want day %s, count %d, the same customers as %+v", after, day.Format("2006-01-02"), before.DayCount+1, before)
		}
	})

	t.Run("buy gilt adds a holding", func(t *testing.T) {
		yields, _ := s.GiltYields(ctx)
		before, _ := s.GiltHoldings(ctx)
		tenor := yields[0].Tenor
		if err := c.BuyGilt(ctx, tenor, core.MinGiltPurchase); err != nil {
			t.Fatal(err)
		}
		after, err := s.GiltHoldings(ctx)
		if err != nil || len(after) != len(before)+1 {
			t.Fatalf("holdings after purchase: %d, want %d (%v)", len(after), len(before)+1, err)
		}
		h := after[len(after)-1]
		if h.Tenor != tenor || h.FaceValue != core.MinGiltPurchase || h.Yield != yields[0].Rate || h.PurchaseDate.IsZero() {
			t.Errorf("new holding %+v, want %s at %v", h, tenor, yields[0].Rate)
		}
		if err := c.BuyGilt(ctx, "no-such-tenor", core.MinGiltPurchase); !errors.Is(err, core.ErrNotFound) {
			t.Errorf("unknown tenor: err = %v, want ErrNotFound", err)
		}
		if err := c.BuyGilt(ctx, tenor, core.MinGiltPurchase-1); !errors.Is(err, core.ErrInvalidAmount) {
			t.Errorf("below the minimum: err = %v, want ErrInvalidAmount", err)
		}
		if again, _ := s.GiltHoldings(ctx); len(again) != len(after) {
			t.Errorf("a refused purchase added a holding")
		}
	})
}
