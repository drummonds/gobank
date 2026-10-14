package bank

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/customers"
	"git.bytestone.uk/hum3/gobank/bank/events"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/bank/market"
	"git.bytestone.uk/hum3/gobank/bank/parameters"
	"git.bytestone.uk/hum3/gobank/bank/payments"
	"git.bytestone.uk/hum3/gobank/bank/products"
	"git.bytestone.uk/hum3/gobank/core"
)

// The commands (core.Commands) and the queries over the components
// (core.StaffQueries). A command that spans components — opening a
// customer, a transfer — is the bank's use case: it decides under the
// read model, writes in one transaction, and invalidates the read model
// after.

// --- Opening a customer ---

// funding is the opening deposit or loan disbursement for one of a new
// customer's accounts.
type funding struct {
	index   int
	payment payments.Payment
}

// OpenCustomer implements core.CustomerCommands: it plans the customer
// (customers.Plan), funds each account from the outside world (a deposit)
// or from the bank (a loan within its lending headroom, down to nothing)
// as a payment on record, persists everything in one transaction, and
// returns the record as the register now holds it. No accounts, or an
// opening amount that is negative, is core.ErrInvalidAmount; an unknown
// product is core.ErrNotFound.
func (b *Bank) OpenCustomer(ctx context.Context, c core.NewCustomer) (core.CustomerRecord, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	day := b.businessDay()
	plan, err := b.customers.Plan(day, c)
	if err != nil {
		return core.CustomerRecord{}, err
	}
	// Funding is decided on the book as the read model has it, plus what
	// this opening adds: a loan is kept within the headroom the deposits
	// before it open up (ADR-0004: a policy on a reading a second old).
	k := b.book.get(ctx)
	savings, lending := k.savings, k.lending
	now := b.clock.Now()
	var fundings []funding
	for i := range plan.Record.Accounts {
		a := &plan.Record.Accounts[i]
		amount := c.Accounts[i].Opening
		kind, from := payments.Deposit, "EXTERNAL"
		if a.Family == gbp.FamilyLending {
			amount = min(amount, lendingHeadroom(savings, lending, b.reserveRatioNow()))
			kind, from = payments.LoanDisbursement, "BANK"
		}
		if amount <= 0 {
			continue
		}
		a.Balance = amount
		if a.Family == gbp.FamilyLending {
			lending += amount
		} else {
			savings += amount
		}
		fundings = append(fundings, funding{index: i, payment: b.payments.New(kind, from, plan.Record.ID, amount, payments.Completed, now)})
	}
	if err := b.persistCustomer(ctx, day, &plan, fundings); err != nil {
		return core.CustomerRecord{}, err
	}
	b.book.invalidate()
	return b.CustomerRecord(ctx, plan.Record.ID)
}

func (b *Bank) reserveRatioNow() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reserveRatio
}

// persistCustomer writes a planned customer — the opening's root event,
// record, ledger accounts, register, funding payments and movements, the
// GL's control movements — in one transaction, so each customer is one
// commit and the opening's cascade is done when it commits (ADR-0011).
// A customer the database refuses is the error returned and nothing of
// theirs is written; later failures are logged and the customer kept.
// Funding rewrites the equity account's position and the new accounts'
// inside the transaction, so their locks are held until it commits:
// creators take turns on the equity account, and the daily pass waits
// for a new account's funding before projecting it.
func (b *Bank) persistCustomer(ctx context.Context, day time.Time, plan *customers.Plan, fundings []funding) error {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("bank: open customer: %w", err)
	}
	books := b.ledger.WithTx(tx)
	if err := b.customers.Persist(ctx, tx, tx, books, plan); err != nil {
		_ = tx.Rollback()
		return err
	}
	rec := &plan.Record
	root, err := b.events.Raise(ctx, tx, events.Opening, b.clock.Now(), rec.ID)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	cause := products.Cause{ID: root.ID, At: root.At}
	locked := []string{books.Chart.EquityCapital}
	for _, a := range rec.Accounts {
		locked = append(locked, a.LedgerAccountID)
	}
	unlock := books.Lock(locked...)
	defer unlock()
	general := b.gl.WithTx(tx)
	// An account has a position from the day it opens, funded or not: the
	// version's open event runs now (its settings, its first position), so
	// the day's pass has nothing left to do for it and a day is complete
	// when every registered account has its position.
	for _, a := range rec.Accounts {
		if err := b.products.Open(books, products.Account{ID: a.LedgerAccountID, Product: a.ProductID}, day, cause); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("bank: open customer %s: %s: %w", rec.ID, a.LedgerAccountID, err)
		}
	}
	for _, f := range fundings {
		f.payment.Cause = root.ID
		if err := b.payments.Insert(tx, f.payment); err != nil {
			log.Printf("bank: open customer %s: %v", rec.ID, err)
		}
		// The movement carries the payment reference: it is the statement
		// line the customer sees. Funding runs from cash to the account,
		// signed as the account's balance is: a deposit is negative, the
		// saver's liability growing; a drawdown positive (ADR-0008).
		a := rec.Accounts[f.index]
		amount := -f.payment.Amount
		if a.Family == gbp.FamilyLending {
			amount = f.payment.Amount
		}
		// The version's post-posting rule then projects the day on the
		// funded balance.
		funded := products.Account{ID: a.LedgerAccountID, Product: a.ProductID}
		if _, err := b.products.PostEvent(books, day, cause, books.Chart.Cash, a.LedgerAccountID, amount, luca.CodeBookTransfer, f.payment.Reference, funded); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("bank: open customer %s: %w", rec.ID, err)
		}
		// The GL sees the funding the same way, on the product's control.
		if err := general.Funding(day, a.ProductID, f.payment.Amount, f.payment.Reference); err != nil {
			log.Printf("bank: open customer %s: %v", rec.ID, err)
		}
	}
	if err := b.events.Done(ctx, tx, root.ID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("bank: open customer %s: commit: %w", rec.ID, err)
	}
	return nil
}

// --- Transfers ---

// Transfer implements core.PaymentCommands: it moves amount between the
// two customers' first savings accounts, posting the movement to the
// ledger with the day's rules rerun on both and to the GL's controls
// when the products differ, and records the payment as completed, all in
// one transaction under the payment's root event, raised and done in the
// same transaction (ADR-0011): the posting is the settlement, so the
// bank is at rest again when the command returns (ADR-0010), and a
// transfer that fails leaves nothing, not even its root. The two
// accounts' locks are held from the balance check to the commit, so two
// transfers from one account take turns.
func (b *Bank) Transfer(ctx context.Context, t core.Transfer) (core.Payment, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	if t.Amount <= 0 {
		return core.Payment{}, core.ErrInvalidAmount
	}
	if t.From == t.To {
		return core.Payment{}, core.ErrSameCustomer
	}
	from, err := b.customers.ByID(ctx, t.From)
	if err != nil {
		return core.Payment{}, err
	}
	to, err := b.customers.ByID(ctx, t.To)
	if err != nil {
		return core.Payment{}, err
	}
	fromAcc, toAcc := customers.FirstSavings(from.Accounts), customers.FirstSavings(to.Accounts)
	if fromAcc == nil || toAcc == nil {
		return core.Payment{}, core.ErrNotFound
	}
	// The two accounts' positions are rewritten: take their locks so the
	// daily pass and this event take turns on each of them, and the
	// balance is checked with them held.
	unlock := b.ledger.Lock(fromAcc.LedgerAccountID, toAcc.LedgerAccountID)
	defer unlock()
	live, err := b.ledger.LivePosition(ctx, fromAcc.LedgerAccountID)
	if err != nil {
		return core.Payment{}, err
	}
	if -live.Balance < t.Amount { // a savings balance is a liability: negative (ADR-0008)
		return core.Payment{}, core.ErrInsufficientFunds
	}
	now := b.clock.Now()
	pay := b.payments.New(payments.Transfer, t.From, t.To, t.Amount, payments.Completed, now)
	day := b.businessDay()
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return core.Payment{}, fmt.Errorf("bank: transfer: %w", err)
	}
	root, err := b.events.Raise(ctx, tx, events.Payment, now, t.From)
	if err != nil {
		_ = tx.Rollback()
		return core.Payment{}, err
	}
	pay.Cause = root.ID
	books := b.ledger.WithTx(tx)
	// A pays B: a negative amount from A's account to B's, B's balance
	// falling (the bank owes B more), A's rising (ADR-0008).
	if _, err := b.products.PostEvent(books, day, products.Cause{ID: root.ID, At: root.At}, fromAcc.LedgerAccountID, toAcc.LedgerAccountID, -t.Amount, luca.CodeBookTransfer, pay.Reference,
		products.Account{ID: fromAcc.LedgerAccountID, Product: fromAcc.ProductID},
		products.Account{ID: toAcc.LedgerAccountID, Product: toAcc.ProductID}); err != nil {
		_ = tx.Rollback()
		return core.Payment{}, err
	}
	if err := b.gl.WithTx(tx).Transfer(day, fromAcc.ProductID, toAcc.ProductID, t.Amount, pay.Reference); err != nil {
		_ = tx.Rollback()
		return core.Payment{}, err
	}
	if err := b.payments.Insert(tx, pay); err != nil {
		_ = tx.Rollback()
		return core.Payment{}, err
	}
	if err := b.events.Done(ctx, tx, root.ID); err != nil {
		_ = tx.Rollback()
		return core.Payment{}, err
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return core.Payment{}, fmt.Errorf("bank: transfer %s: commit: %w", pay.Reference, err)
	}
	b.book.invalidate()
	b.interest.invalidate() // the two positions were rewritten, accruals with them
	return pay.Core(), nil
}

// --- Treasury ---

// BuyGilt implements core.TreasuryCommands: the holding is recorded and
// its face value moves from Asset:Cash to Asset:Gilts in one transaction.
func (b *Bank) BuyGilt(ctx context.Context, tenor string, faceValue luca.Amount) error {
	b.open.RLock()
	defer b.open.RUnlock()
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("bank: buy gilt: %w", err)
	}
	defer tx.Rollback()
	if err := b.treasury.WithTx(tx).Buy(ctx, tenor, faceValue); err != nil {
		return err
	}
	books := b.ledger.WithTx(tx)
	desc := fmt.Sprintf("Gilt purchase, %s tenor", tenor)
	if _, err := books.RecordMovement(books.Chart.Cash, books.Chart.Gilts, faceValue, luca.CodeBookTransfer, b.clock.Now(), desc); err != nil {
		return fmt.Errorf("bank: buy gilt: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("bank: buy gilt: commit: %w", err)
	}
	b.book.invalidate() // cash moved
	return nil
}

// GiltYields implements core.TreasuryQueries.
func (b *Bank) GiltYields(ctx context.Context) ([]core.GiltYield, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	return b.treasury.Yields(ctx)
}

// GiltHoldings implements core.TreasuryQueries.
func (b *Bank) GiltHoldings(ctx context.Context) ([]core.GiltHolding, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	return b.treasury.Holdings(ctx)
}

// --- Customers ---

// CustomersPerPage and PaymentsPerPage are the pages the register and the
// payments list are read in.
const (
	CustomersPerPage    = 50
	PaymentsPerPage     = 20
	TransactionsPerPage = 20
)

// Customer implements core.CustomerQueries.
func (b *Bank) Customer(ctx context.Context, customerID string) (core.Customer, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	if !b.customers.Exists(ctx, customerID) {
		return core.Customer{}, core.ErrNotFound
	}
	return core.Customer{ID: customerID, Name: b.customers.Name(ctx, customerID)}, nil
}

// Accounts implements core.CustomerQueries, in register order.
func (b *Bank) Accounts(ctx context.Context, customerID string) ([]core.Account, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	cust, err := b.customers.ByID(ctx, customerID)
	if err != nil {
		return nil, err
	}
	return customers.CoreAccounts(cust.Accounts), nil
}

// Transactions implements core.CustomerQueries, newest first.
func (b *Bank) Transactions(ctx context.Context, customerID string, page int) (core.TransactionPage, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	return b.customers.Transactions(ctx, customerID, page, TransactionsPerPage)
}

// AccountTransactions implements core.CustomerQueries for one account by
// its register index.
func (b *Bank) AccountTransactions(ctx context.Context, customerID string, index, page int) (core.TransactionPage, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	return b.customers.AccountTransactions(ctx, customerID, index, page, TransactionsPerPage)
}

// CustomerPage implements core.CustomerRegister.
func (b *Bank) CustomerPage(ctx context.Context, page int) (core.CustomerPage, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	page = max(page, 1)
	recs, total, err := b.customers.Page(ctx, page, CustomersPerPage)
	if err != nil {
		return core.CustomerPage{}, err
	}
	p := core.CustomerPage{Page: page, PerPage: CustomersPerPage, Total: total}
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
func (b *Bank) CustomerRecord(ctx context.Context, customerID string) (core.CustomerRecord, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	cust, err := b.customers.ByID(ctx, customerID)
	if err != nil {
		return core.CustomerRecord{}, err
	}
	return cust.Core(), nil
}

// CustomerName implements core.CustomerRegister.
func (b *Bank) CustomerName(ctx context.Context, customerID string) (string, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	if !b.customers.Exists(ctx, customerID) {
		return "", core.ErrNotFound
	}
	return b.customers.Name(ctx, customerID), nil
}

// CustomerPII implements core.CustomerRegister.
func (b *Bank) CustomerPII(ctx context.Context, customerID string) (core.PII, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	return b.customers.PII(ctx, customerID)
}

// SavingsInterestByCustomer implements core.CustomerRegister.
func (b *Bank) SavingsInterestByCustomer(ctx context.Context) ([]core.CustomerInterest, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	return b.customers.SavingsInterest(ctx)
}

// --- Payments ---

// PaymentPage implements core.PaymentQueries.
func (b *Bank) PaymentPage(ctx context.Context, page int) (core.PaymentPage, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	page = max(page, 1)
	pays, total, err := b.payments.Page(ctx, page, PaymentsPerPage)
	if err != nil {
		return core.PaymentPage{}, err
	}
	out := core.PaymentPage{Page: page, PerPage: PaymentsPerPage, Total: total}
	for _, p := range pays {
		out.Payments = append(out.Payments, p.Core())
	}
	return out, nil
}

// Payment implements core.PaymentQueries.
func (b *Bank) Payment(ctx context.Context, id int) (core.Payment, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	p, err := b.payments.ByID(ctx, id)
	if err != nil {
		return core.Payment{}, err
	}
	return p.Core(), nil
}

// PaymentsOf implements core.PaymentQueries.
func (b *Bank) PaymentsOf(ctx context.Context, customerID string) ([]core.Payment, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	if !b.customers.Exists(ctx, customerID) {
		return nil, core.ErrNotFound
	}
	pays, err := b.payments.Of(ctx, customerID)
	if err != nil {
		return nil, err
	}
	out := make([]core.Payment, len(pays))
	for i, p := range pays {
		out[i] = p.Core()
	}
	return out, nil
}

// ClearPayments removes every payment and restarts their numbering: the
// console's reset of the payments page.
func (b *Bank) ClearPayments(ctx context.Context) error {
	b.open.RLock()
	defer b.open.RUnlock()
	return b.payments.Clear(ctx)
}

// --- Products ---

// Products implements core.ProductQueries: the catalogue, a row per
// product on record, with the rules it runs on, the product's share of
// the book — its balance, from the same reading as the position — and
// the accounts holding it.
func (b *Bank) Products(ctx context.Context) ([]core.Product, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	k := b.book.get(ctx)
	now := b.clock.Now()
	byProduct, err := b.products.AccountsByProduct(ctx)
	if err != nil {
		return nil, err
	}
	catalogue := b.products.All()
	out := make([]core.Product, 0, len(catalogue))
	for _, prod := range catalogue {
		var params []core.Parameter
		var rate int64
		if prod.Carried() {
			for _, d := range prod.Rules.Parameters() {
				params = append(params, core.Parameter{Key: d.Key, Scope: string(d.Scope), Kind: string(d.Kind), Value: publishedOrDerived(d), Derived: d.IsDerived()})
			}
			// The rate the product pays now: resolved, so a tracker's is
			// the bank's rate through its derivation.
			if p, err := b.products.ProductParameter(ctx, nil, prod, "rate_bps", now, now); err == nil {
				rate = p.Bps()
			}
		}
		out = append(out, core.Product{
			ID: prod.ID, Name: prod.Name, Rules: prod.RulesID, RulesVersion: prod.RulesVersion, Carried: prod.Carried(),
			Family: string(prod.Family()), Currency: prod.Currency,
			RateBps: rate, Parameters: params, Description: prod.Description,
			Adopted: prod.Adopted, ModuleVersion: prod.ModuleVersion, OnSale: prod.OnSale(),
			Accounts: byProduct[prod.ID], Balance: k.products[prod.ID].balance,
		})
	}
	return out, nil
}

// UpcomingSettings implements core.ProductQueries: every setting effective
// after the clock's instant, soonest first.
func (b *Bank) UpcomingSettings(ctx context.Context) ([]core.Setting, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	upcoming, err := b.settings.Upcoming(ctx, nil, b.clock.Now())
	if err != nil {
		return nil, err
	}
	out := make([]core.Setting, len(upcoming))
	for i, u := range upcoming {
		out[i] = core.Setting{Scope: string(u.Scope), Owner: u.Owner, Key: u.Key, Value: u.Value, Effective: u.Effective, Decided: u.Decided, By: u.By}
	}
	return out, nil
}

// Parameter implements core.ProductQueries: one parameter by its owner
// and key. A product's is as its rules declare it, the value in force
// resolved at the clock's instant (a setting, the published value, or a
// derivation); a currency's is its market's risk-free rate, which the feed
// writes and nobody sets; the bank's own are its policy's, settable where
// the policy publishes a value and derived where it does not.
func (b *Bank) Parameter(ctx context.Context, owner, key string) (core.ParameterDetail, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	now := b.clock.Now()
	var d gbp.Declaration
	detail := core.ParameterDetail{Owner: owner, Key: key}
	var resolve func() (gbp.Param, error) // the value in force, by whose parameter it is
	if prod, ok := b.products.ByID(owner); ok {
		if !prod.Carried() {
			return core.ParameterDetail{}, core.ErrNotFound
		}
		if d, ok = gbp.Declared(prod.Rules, key); !ok {
			return core.ParameterDetail{}, core.ErrNotFound
		}
		detail.Family = string(prod.Family())
		detail.Settable = d.Scope == gbp.ScopeVersion && !d.IsDerived()
		resolve = func() (gbp.Param, error) { return b.products.ProductParameter(ctx, nil, prod, key, now, now) }
	} else if src, ok := market.ByCurrency(owner); ok && key == gbp.RiskFreeRateKey {
		d = gbp.Declaration{Key: key, Scope: gbp.ScopeMarket, Kind: gbp.KindBps, Description: src.Rate + ", as " + src.Name + " publishes it"}
		resolve = func() (gbp.Param, error) {
			text, found, err := b.settings.Resolve(ctx, nil, d.Scope, owner, key, now, now)
			if err != nil || !found {
				return gbp.Param{}, err
			}
			return d.Parse(text)
		}
	} else if pol := b.products.Policy(); owner == core.BankOwner && pol != nil {
		if d, ok = policyDeclaration(pol, key); !ok || d.Scope != gbp.ScopeBank {
			return core.ParameterDetail{}, core.ErrNotFound
		}
		detail.Settable = d.Published != "" && !d.IsDerived()
		resolve = func() (gbp.Param, error) { return b.products.BankParameter(ctx, nil, key, now, now) }
	} else {
		return core.ParameterDetail{}, core.ErrNotFound
	}
	detail.Scope, detail.Kind, detail.Description = string(d.Scope), string(d.Kind), d.Description
	detail.Derived, detail.Derivation = d.IsDerived(), describeDeclaration(d)
	if d.Scope != gbp.ScopeAccount {
		p, err := resolve()
		if err != nil {
			log.Printf("bank: parameter %s/%s in force: %v", owner, key, err)
		}
		detail.InForce = p.Text
	}
	upcoming, err := b.settings.Upcoming(ctx, nil, now)
	if err != nil {
		return core.ParameterDetail{}, err
	}
	for _, u := range upcoming {
		if u.Scope == d.Scope && u.Owner == owner && u.Key == key {
			detail.Upcoming = append(detail.Upcoming, core.Setting{Scope: string(u.Scope), Owner: u.Owner, Key: u.Key, Value: u.Value, Effective: u.Effective, Decided: u.Decided, By: u.By})
		}
	}
	return detail, nil
}

// policyDeclaration finds the policy's declaration of key.
func policyDeclaration(pol gbp.Policy, key string) (gbp.Declaration, bool) {
	for _, d := range pol.Parameters() {
		if d.Key == key {
			return d, true
		}
	}
	return gbp.Declaration{}, false
}

// Policy implements core.PolicyQueries: the bank's policy and its
// bank-scoped parameters, each with the value in force at the clock's
// instant.
func (b *Bank) Policy(ctx context.Context) (core.Policy, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	pol := b.products.Policy()
	if pol == nil {
		return core.Policy{}, core.ErrNotFound
	}
	now := b.clock.Now()
	out := core.Policy{Name: pol.Name(), Version: pol.Version()}
	for _, d := range pol.Parameters() {
		if d.Scope != gbp.ScopeBank {
			continue
		}
		p := core.PolicyParameter{Parameter: core.Parameter{Key: d.Key, Scope: string(d.Scope), Kind: string(d.Kind), Value: publishedOrDerived(d), Derived: d.IsDerived()}}
		if v, err := b.products.BankParameter(ctx, nil, d.Key, now, now); err == nil {
			p.InForce = v.Text
		}
		out.Parameters = append(out.Parameters, p)
	}
	return out, nil
}

// SetParameter implements core.ProductCommands: a staff member's setting
// of one of a product's own parameters, or one the bank's policy
// publishes, decided at the clock's instant and effective from the
// instant given, not before now, written under a root event raised at
// that instant (ADR-0011): the decision is on the setting, the event is
// the bank's work when the clock gets there.
func (b *Bank) SetParameter(ctx context.Context, s core.ParameterSetting) error {
	b.open.RLock()
	defer b.open.RUnlock()
	var d gbp.Declaration
	var scope gbp.Scope
	if prod, ok := b.products.ByID(s.Owner); ok {
		if !prod.Carried() {
			return fmt.Errorf("%w: %s: this build does not carry its rules", core.ErrInvalidSetting, s.Owner)
		}
		d, ok = gbp.Declared(prod.Rules, s.Key)
		switch {
		case !ok:
			return fmt.Errorf("%w: %s v%d declares no %s", core.ErrInvalidSetting, prod.RulesID, prod.RulesVersion, s.Key)
		case d.IsDerived():
			return fmt.Errorf("%w: %s is derived, not set", core.ErrInvalidSetting, s.Key)
		case d.Scope != gbp.ScopeVersion:
			return fmt.Errorf("%w: %s is at the %s scope, not the product's", core.ErrInvalidSetting, s.Key, d.Scope)
		}
		scope = gbp.ScopeVersion
	} else if pol := b.products.Policy(); s.Owner == core.BankOwner && pol != nil {
		d, ok = policyDeclaration(pol, s.Key)
		switch {
		case !ok || d.Scope != gbp.ScopeBank:
			return fmt.Errorf("%w: %s v%d declares no %s at the bank scope", core.ErrInvalidSetting, pol.Name(), pol.Version(), s.Key)
		case d.IsDerived():
			return fmt.Errorf("%w: %s is derived by the policy, not set", core.ErrInvalidSetting, s.Key)
		case d.Published == "":
			return fmt.Errorf("%w: %s is not the bank's to set", core.ErrInvalidSetting, s.Key)
		}
		scope = gbp.ScopeBank
	} else {
		return core.ErrNotFound
	}
	param, err := d.Parse(s.Value)
	if err != nil {
		return fmt.Errorf("%w: %v", core.ErrInvalidSetting, err)
	}
	now := b.clock.Now()
	if s.Effective.Before(now) {
		return fmt.Errorf("%w: effective %s is before the bank's clock, %s", core.ErrInvalidSetting, s.Effective.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339))
	}
	if s.By == "" {
		return fmt.Errorf("%w: no one decided it", core.ErrInvalidSetting)
	}
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("bank: set parameter: %w", err)
	}
	root, err := b.events.Raise(ctx, tx, events.Setting, s.Effective, s.By)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := b.settings.Set(ctx, tx, parameters.Setting{Scope: scope, Owner: s.Owner, Key: d.Key, Value: param.Text, Effective: s.Effective, Decided: now, By: s.By, Cause: root.ID}); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("bank: set parameter: commit: %w", err)
	}
	return nil
}

// publishedOrDerived is a declaration's value for a listing: what it
// publishes, or its derivation in words.
func publishedOrDerived(d gbp.Declaration) string {
	if d.IsDerived() {
		return describeDeclaration(d)
	}
	return d.Published
}

// describeDeclaration is a derived parameter's derivation in words: a
// formula from its parts, an algorithm as its declarer describes it.
func describeDeclaration(d gbp.Declaration) string {
	switch {
	case d.Derived != nil:
		return describeDerivation(*d.Derived)
	case d.Algorithm != nil:
		return d.Description
	}
	return ""
}

// describeDerivation is a derived parameter in words: "boe.base_rate_bps − 15 bps, floor 0".
func describeDerivation(d gbp.Derivation) string {
	s := d.Source
	switch {
	case d.SpreadBps > 0:
		s += fmt.Sprintf(" + %d bps", d.SpreadBps)
	case d.SpreadBps < 0:
		s += fmt.Sprintf(" − %d bps", -d.SpreadBps)
	}
	if d.Floor != nil {
		s += fmt.Sprintf(", floor %d", *d.Floor)
	}
	if d.Cap != nil {
		s += fmt.Sprintf(", cap %d", *d.Cap)
	}
	return s
}

var (
	_ core.StaffQueries = (*Bank)(nil)
	_ core.Commands     = (*Bank)(nil)
)

// --- The ledger ---

// Movement implements core.LedgerQueries: one movement as the journal
// entry it is, in the bank's view (ADR-0008).
func (b *Bank) Movement(ctx context.Context, id string) (core.Movement, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	m, err := b.ledger.MovementByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Movement{}, core.ErrNotFound
	}
	if err != nil {
		return core.Movement{}, err
	}
	leg := func(l ledger.MovementLeg) core.Leg {
		kind, _, _ := strings.Cut(l.Path, ":")
		return core.Leg{AccountID: l.AccountID, Path: l.Path, Type: kind, Balance: l.Balance}
	}
	out := core.Movement{
		ID: m.ID, From: leg(m.From), To: leg(m.To), Amount: m.Amount, Currency: ledger.Currency,
		Code: m.Code, ValueTime: m.ValueTime, KnowledgeTime: m.KnowledgeTime, Description: m.Description,
	}
	// A rule's posting names its cause and step (ADR-0011).
	rec, ok, err := b.products.PostingByMovement(ctx, id)
	if err != nil {
		return core.Movement{}, err
	}
	if ok {
		out.Step = rec.Step
		if rec.Cause != "" {
			if ev, found, err := b.events.ByID(ctx, rec.Cause); err != nil {
				return core.Movement{}, err
			} else if found {
				cause := eventRecord(ev)
				out.Cause = &cause
			}
		}
	}
	return out, nil
}
