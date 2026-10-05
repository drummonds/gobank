package main

import (
	"context"
	"crypto/subtle"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
)

// coreAdapter is the demo's implementation of the core contracts
// (ADR-0002 stage 1): the BFF and the staff web app reach DemoState only
// through it. Figures come from where the demo keeps them today — the
// register and the products engine for accounts, the ledger for
// transactions, in-memory series for histories.
type coreAdapter struct {
	ds *DemoState
	// password is the one app password every customer logs in with
	// (GOBANK_APP_PASSWORD); empty means app login is off. Generated
	// customers have no credentials of their own until the identity
	// component arrives with stored sessions in stage 2.
	password string
}

func newCoreAdapter(ds *DemoState, password string) *coreAdapter {
	return &coreAdapter{ds: ds, password: password}
}

// --- Customer side ---

// Authenticate implements core.Authenticator: the shared password, for a
// customer that exists. Unknown customers and wrong passwords fail alike.
func (a *coreAdapter) Authenticate(_ context.Context, customerID, password string) (core.Customer, error) {
	ok := a.password != "" && subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) == 1
	cust, exists := a.ds.customerByID(customerID)
	if !ok || !exists {
		return core.Customer{}, core.ErrBadCredentials
	}
	return core.Customer{ID: cust.ID, Name: a.ds.lookupName(cust.ID)}, nil
}

// Customer implements core.CustomerQueries.
func (a *coreAdapter) Customer(_ context.Context, customerID string) (core.Customer, error) {
	cust, ok := a.ds.customerByID(customerID)
	if !ok {
		return core.Customer{}, core.ErrNotFound
	}
	return core.Customer{ID: cust.ID, Name: a.ds.lookupName(cust.ID)}, nil
}

// Accounts implements core.CustomerQueries, in register order.
func (a *coreAdapter) Accounts(_ context.Context, customerID string) ([]core.Account, error) {
	cust, ok := a.ds.customerByID(customerID)
	if !ok {
		return nil, core.ErrNotFound
	}
	return coreAccounts(cust.Accounts), nil
}

func coreAccounts(accounts []CustomerAccount) []core.Account {
	accts := make([]core.Account, len(accounts))
	for i, acc := range accounts {
		accts[i] = core.Account{
			Index:       i,
			ProductName: acc.ProductName,
			Family:      string(acc.Family),
			Currency:    acc.Currency,
			Rate:        acc.Rate,
			Balance:     acc.Balance,
			Interest:    acc.Interest,
			AccruedE7:   acc.AccruedE7,
			SortCode:    acc.SortCode,
			AccountNum:  acc.AccountNum,
			OpenDate:    acc.OpenDate.Format("2006-01-02"),
		}
	}
	return accts
}

// Transactions implements core.CustomerQueries from the ledger, newest first.
func (a *coreAdapter) Transactions(_ context.Context, customerID string, page int) (core.TransactionPage, error) {
	if !a.ds.customerExists(customerID) {
		return core.TransactionPage{}, core.ErrNotFound
	}
	page = max(page, 1)
	entries, total := a.ds.CustomerTransactions(customerID, page, txPerPage)
	return txPage(entries, page, total), nil
}

// AccountTransactions implements core.CustomerQueries for one account by
// its register index.
func (a *coreAdapter) AccountTransactions(_ context.Context, customerID string, index, page int) (core.TransactionPage, error) {
	cust, ok := a.ds.customerByID(customerID)
	if !ok || index < 0 || index >= len(cust.Accounts) {
		return core.TransactionPage{}, core.ErrNotFound
	}
	page = max(page, 1)
	entries, total := a.ds.ProductTransactions(customerID, index, page, txPerPage)
	return txPage(entries, page, total), nil
}

func txPage(entries []TxEntry, page, total int) core.TransactionPage {
	p := core.TransactionPage{Page: page, PerPage: txPerPage, Total: total}
	for _, tx := range entries {
		p.Entries = append(p.Entries, core.Transaction{
			ID:          tx.ID,
			Date:        tx.Date.Format("2006-01-02"),
			ProductName: tx.ProductName,
			Type:        tx.Type.String(),
			Currency:    tx.Currency,
			Reference:   tx.Reference,
			Amount:      tx.Amount,
			Balance:     tx.Balance,
		})
	}
	return p
}

// --- Book ---

// Position implements core.BookQueries.
func (a *coreAdapter) Position(context.Context) (core.Position, error) {
	return a.ds.position(), nil
}

// ProfitAndLoss implements core.BookQueries.
func (a *coreAdapter) ProfitAndLoss(context.Context) (core.ProfitAndLoss, error) {
	return a.ds.profitAndLoss(), nil
}

// BalanceSheet implements core.BookQueries.
func (a *coreAdapter) BalanceSheet(ctx context.Context) (core.BalanceSheet, error) {
	pos, pl := a.ds.position(), a.ds.profitAndLoss()
	var gilts luca.Amount
	for _, h := range a.ds.getGiltHoldings() {
		gilts += h.FaceValue
	}
	retained := pl.NetProfit()
	return core.BalanceSheet{
		DayCount:           pos.DayCount,
		Loans:              pos.Lending,
		Gilts:              gilts,
		CashAtBoE:          max(pos.Savings-pos.Lending+retained-gilts, 0), // gilts are bought from cash
		Deposits:           pos.Savings,
		RetainedEarnings:   retained,
		RiskWeightedAssets: pos.Lending,
	}, nil
}

// History implements core.BookQueries.
func (a *coreAdapter) History(context.Context) (core.History, error) {
	return a.ds.history(), nil
}

// --- Customer register ---

// CustomerPage implements core.CustomerRegister.
func (a *coreAdapter) CustomerPage(_ context.Context, page int) (core.CustomerPage, error) {
	page = max(page, 1)
	recs, total := a.ds.customerPage(page)
	p := core.CustomerPage{Page: page, PerPage: customersPerPage, Total: total}
	for _, c := range recs {
		row := core.CustomerSummary{ID: c.ID, Accounts: len(c.Accounts)}
		for _, acc := range c.Accounts {
			if acc.Family == gbp.FamilySavings {
				row.Savings += acc.Balance
			} else {
				row.Lending += acc.Balance
			}
		}
		p.Customers = append(p.Customers, row)
	}
	return p, nil
}

// CustomerRecord implements core.CustomerRegister.
func (a *coreAdapter) CustomerRecord(_ context.Context, customerID string) (core.CustomerRecord, error) {
	cust, ok := a.ds.customerByID(customerID)
	if !ok {
		return core.CustomerRecord{}, core.ErrNotFound
	}
	return core.CustomerRecord{
		ID:       cust.ID,
		JoinDate: cust.JoinDate,
		KYC: core.KYC{
			Verified:   cust.KYCStatus.Verified,
			LastCheck:  cust.KYCStatus.LastCheckDate,
			RiskRating: cust.KYCStatus.RiskRating,
		},
		Accounts: coreAccounts(cust.Accounts),
	}, nil
}

// CustomerName implements core.CustomerRegister.
func (a *coreAdapter) CustomerName(_ context.Context, customerID string) (string, error) {
	if !a.ds.customerExists(customerID) {
		return "", core.ErrNotFound
	}
	return a.ds.lookupName(customerID), nil
}

// CustomerPII implements core.CustomerRegister.
func (a *coreAdapter) CustomerPII(_ context.Context, customerID string) (core.PII, error) {
	if !a.ds.customerExists(customerID) {
		return core.PII{}, core.ErrNotFound
	}
	pii := a.ds.lookupPII(customerID)
	return core.PII{Name: a.ds.lookupName(customerID), NI: pii.NI, DOB: pii.DOB, Address: pii.Address, Email: pii.Email, Phone: pii.Phone}, nil
}

// SavingsInterestByCustomer implements core.CustomerRegister.
func (a *coreAdapter) SavingsInterestByCustomer(context.Context) ([]core.CustomerInterest, error) {
	rows := a.ds.savingsInterestByCustomer()
	out := make([]core.CustomerInterest, len(rows))
	for i, r := range rows {
		out[i] = core.CustomerInterest{CustomerID: r.CustomerID, Interest: r.Interest}
	}
	return out, nil
}

// --- Payments ---

// PaymentPage implements core.PaymentQueries.
func (a *coreAdapter) PaymentPage(_ context.Context, page int) (core.PaymentPage, error) {
	page = max(page, 1)
	payments, total := a.ds.paymentPage(page)
	return core.PaymentPage{Payments: corePayments(payments), Page: page, PerPage: paymentsPerPage, Total: total}, nil
}

// Payment implements core.PaymentQueries.
func (a *coreAdapter) Payment(_ context.Context, id int) (core.Payment, error) {
	p, ok := a.ds.paymentByID(id)
	if !ok {
		return core.Payment{}, core.ErrNotFound
	}
	return corePayment(p), nil
}

// PaymentsOf implements core.PaymentQueries.
func (a *coreAdapter) PaymentsOf(_ context.Context, customerID string) ([]core.Payment, error) {
	if !a.ds.customerExists(customerID) {
		return nil, core.ErrNotFound
	}
	return corePayments(a.ds.paymentsOf(customerID)), nil
}

func corePayments(payments []Payment) []core.Payment {
	out := make([]core.Payment, len(payments))
	for i, p := range payments {
		out[i] = corePayment(p)
	}
	return out
}

func corePayment(p Payment) core.Payment {
	return core.Payment{
		ID:        p.ID,
		Reference: p.Reference,
		Type:      core.PaymentType(p.Type.String()),
		From:      p.FromID,
		To:        p.ToID,
		Amount:    p.Amount,
		Status:    core.PaymentStatus(p.Status.String()),
		CreatedAt: p.CreatedAt,
		SettledAt: p.SettledAt,
	}
}

// --- Products ---

// Products implements core.ProductQueries: the catalogue with each
// product's open accounts and their balance.
func (a *coreAdapter) Products(context.Context) ([]core.Product, error) {
	totals := a.ds.productTotals()
	var out []core.Product
	for _, p := range a.ds.products {
		out = append(out, core.Product{
			ID:          p.ID,
			Name:        p.Name,
			Family:      string(p.Family),
			Currency:    p.Currency,
			Rate:        p.Rate,
			Terms:       p.Terms,
			Description: p.Description,
			Accounts:    totals[p.ID].Accounts,
			Balance:     totals[p.ID].Balance,
		})
	}
	return out, nil
}

// --- Treasury ---

// GiltYields implements core.TreasuryQueries.
func (a *coreAdapter) GiltYields(context.Context) ([]core.GiltYield, error) {
	return a.ds.getGiltYields(), nil
}

// GiltHoldings implements core.TreasuryQueries.
func (a *coreAdapter) GiltHoldings(context.Context) ([]core.GiltHolding, error) {
	return a.ds.getGiltHoldings(), nil
}

// --- Commands ---

// Transfer implements core.PaymentCommands.
func (a *coreAdapter) Transfer(_ context.Context, t core.Transfer) (core.Payment, error) {
	p, err := a.ds.transfer(t)
	if err != nil {
		return core.Payment{}, err
	}
	return corePayment(p), nil
}

// BuyGilt implements core.TreasuryCommands.
func (a *coreAdapter) BuyGilt(_ context.Context, tenor string, faceValue luca.Amount) error {
	return a.ds.BuyGilt(tenor, faceValue)
}

var (
	_ core.CustomerQueries = (*coreAdapter)(nil)
	_ core.Authenticator   = (*coreAdapter)(nil)
	_ core.StaffQueries    = (*coreAdapter)(nil)
	_ core.Commands        = (*coreAdapter)(nil)
)
