// Package customers is who the bank's customers are and the accounts each
// holds (ADR-0002 stage 5, story 1.5.4): customer identity and PII in
// gobanks-customers' encrypted store, the account register (which ledger
// accounts a customer holds and the bank-facing details of each) in the
// component's own table, and the read model over them, with balances and
// accrued interest from the ledger's positions. Customer numbers are the
// component's own (ADR-0004).
package customers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/bank/products"
	"git.bytestone.uk/hum3/gobank/bank/refs"
	"git.bytestone.uk/hum3/gobank/bank/schema"
	"git.bytestone.uk/hum3/gobank/core"
	store "git.bytestone.uk/hum3/gobanks-customers"
)

// Schema: the account register. idx is the account's position in the
// customer's account list; the bank app and the staff pages address
// accounts by (customer, index).
var Schema = schema.Component{Name: "customers", Migrations: []schema.Migration{
	{Version: 1, Statements: []string{`CREATE TABLE IF NOT EXISTS customer_accounts (
		customer_id VARCHAR(20) NOT NULL,
		idx INTEGER NOT NULL,
		ledger_account_id VARCHAR(64) NOT NULL UNIQUE,
		product_id VARCHAR(50) NOT NULL,
		sort_code VARCHAR(8) NOT NULL,
		account_num VARCHAR(8) NOT NULL,
		opened TIMESTAMP NOT NULL,
		PRIMARY KEY (customer_id, idx)
	)`}},
}}

// CreateView (re)creates the contract view: the register as other
// components read it. Recreated at every start so a durable database
// picks up a changed definition.
func CreateView(db *sql.DB) error {
	for _, stmt := range []string{
		`DROP VIEW IF EXISTS contract_customer_accounts`,
		`CREATE VIEW contract_customer_accounts AS
			SELECT customer_id, idx, ledger_account_id, product_id, sort_code, account_num, opened FROM customer_accounts`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("customers: contract_customer_accounts: %w", err)
		}
	}
	return nil
}

// SortCode is the bank's one sort code: the branch every account is at.
const SortCode = "30-90-01"

// Account is one product held by a customer, as the register and the
// ledger show it.
type Account struct {
	ProductID       string
	ProductName     string
	Family          gbp.ProductFamily
	Currency        string      // ISO 4217, from the product
	Balance         luca.Amount // minor units (pence); principal plus applied interest
	Rate            float64     // annual rate (a rate, not money)
	Interest        luca.Amount // minor units; lifetime interest applied to the balance
	Accrued         luca.Amount // minor units; accrued-but-unapplied interest, whole pence
	AccruedE7       int64       // the same accrual to 7 decimal places of a pound, from the ledger's live position
	OpenDate        time.Time
	SortCode        string
	AccountNum      string
	LedgerAccountID string
}

// Record is a customer as the register holds them, without PII.
type Record struct {
	ID       string
	JoinDate time.Time
	KYC      core.KYC
	Accounts []Account
}

// Core is the record as the core publishes it.
func (r Record) Core() core.CustomerRecord {
	return core.CustomerRecord{ID: r.ID, JoinDate: r.JoinDate, KYC: r.KYC, Accounts: CoreAccounts(r.Accounts)}
}

// CoreAccounts is a customer's accounts as the core publishes them, in
// register order.
func CoreAccounts(accounts []Account) []core.Account {
	out := make([]core.Account, len(accounts))
	for i, a := range accounts {
		out[i] = core.Account{
			Index: i, ProductName: a.ProductName, Family: string(a.Family), Currency: a.Currency, Rate: a.Rate,
			Balance: a.Balance, Interest: a.Interest, AccruedE7: a.AccruedE7,
			SortCode: a.SortCode, AccountNum: a.AccountNum, OpenDate: a.OpenDate.Format(time.DateOnly),
		}
	}
	return out
}

// FirstSavings is the customer's first savings account, or nil.
func FirstSavings(accounts []Account) *Account {
	for i := range accounts {
		if accounts[i].Family == gbp.FamilySavings {
			return &accounts[i]
		}
	}
	return nil
}

// Customers is the component over the bank's database.
type Customers struct {
	db       *sql.DB
	store    *store.SQLCustomerStore
	books    *ledger.Ledger
	products *products.Products
	seq      *refs.Sequence
	mu       sync.Mutex
	rng      *rand.Rand // account numbers (mu)
}

// Open is the component on db, with the register's view in place and
// the customer numbering resumed after the newest customer on record.
// seed is the account numbers' randomness.
func Open(db *sql.DB, key store.KeyProvider, books *ledger.Ledger, catalogue *products.Products, seed int64) (*Customers, error) {
	if err := CreateView(db); err != nil {
		return nil, err
	}
	s, err := store.NewSQLCustomerStore(db, key)
	if err != nil {
		return nil, fmt.Errorf("customers: store: %w", err)
	}
	c := &Customers{db: db, store: s, books: books, products: catalogue, rng: rand.New(rand.NewSource(seed))}
	last, err := c.lastSeq(context.Background())
	if err != nil {
		return nil, err
	}
	c.seq = refs.Resume(last)
	return c, nil
}

// Store is the customer store itself, for the wiring and for tests.
func (c *Customers) Store() *store.SQLCustomerStore { return c.store }

// lastSeq is the sequence number of the newest customer (IDs are
// cust-<seq>), zero when there are none.
func (c *Customers) lastSeq(ctx context.Context) (int, error) {
	n, err := c.store.Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("customers: count: %w", err)
	}
	if n == 0 {
		return 0, nil
	}
	recs, _, err := c.store.List(ctx, n-1, 1)
	if err != nil || len(recs) == 0 {
		return 0, fmt.Errorf("customers: newest customer: %w", err)
	}
	var seq int
	if _, err := fmt.Sscanf(recs[0].ID, "cust-%d", &seq); err != nil {
		return 0, fmt.Errorf("customers: customer ID %q: %w", recs[0].ID, err)
	}
	return seq, nil
}

// Count is the number of customers on the books.
func (c *Customers) Count(ctx context.Context) (int, error) {
	n, err := c.store.Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("customers: count: %w", err)
	}
	return n, nil
}

// Exists reports whether a customer is on the books.
func (c *Customers) Exists(ctx context.Context, id string) bool {
	rec, err := c.store.GetByID(ctx, id)
	return err == nil && rec != nil
}

// ByID is one customer with their accounts; an unknown customer is
// core.ErrNotFound.
func (c *Customers) ByID(ctx context.Context, id string) (Record, error) {
	rec, err := c.store.GetByID(ctx, id)
	if err != nil {
		return Record{}, fmt.Errorf("customers: %s: %w", id, err)
	}
	if rec == nil {
		return Record{}, core.ErrNotFound
	}
	r := recordOf(rec)
	r.Accounts, err = c.accountsOf(ctx, id)
	return r, err
}

func recordOf(rec *store.CustomerRecord) Record {
	return Record{
		ID: rec.ID, JoinDate: rec.JoinDate,
		KYC: core.KYC{Verified: rec.KYCVerified, LastCheck: rec.KYCLastCheck, RiskRating: rec.KYCRiskRating},
	}
}

// Page is one page of customers, oldest first, with their accounts, and
// the total number of customers.
func (c *Customers) Page(ctx context.Context, page, perPage int) ([]Record, int, error) {
	page = max(page, 1)
	recs, total, err := c.store.List(ctx, (page-1)*perPage, perPage)
	if err != nil {
		return nil, 0, fmt.Errorf("customers: page %d: %w", page, err)
	}
	out := make([]Record, 0, len(recs))
	for _, rec := range recs {
		r := recordOf(&rec)
		if r.Accounts, err = c.accountsOf(ctx, rec.ID); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, nil
}

// Name is the customer's name, or the ID when it cannot be read.
func (c *Customers) Name(ctx context.Context, id string) string {
	if name, err := c.store.GetNameByID(ctx, id); err == nil && name != "" {
		return name
	}
	return id
}

// PII is the customer's personal information, decrypted; an unknown
// customer is core.ErrNotFound.
func (c *Customers) PII(ctx context.Context, id string) (core.PII, error) {
	pii, err := c.store.GetPIIByID(ctx, id)
	if err != nil {
		return core.PII{}, fmt.Errorf("customers: PII of %s: %w", id, err)
	}
	if pii == nil {
		return core.PII{}, core.ErrNotFound
	}
	return core.PII{Name: pii.Name, NI: pii.NI, DOB: pii.DOB, Address: pii.Address, Email: pii.Email, Phone: pii.Phone}, nil
}

// accountsOf reads a customer's accounts from the register in index order
// and fills in balances, accrual and applied interest.
func (c *Customers) accountsOf(ctx context.Context, customerID string) ([]Account, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT ledger_account_id, product_id, sort_code, account_num, opened
		FROM customer_accounts WHERE customer_id = $1 ORDER BY idx`, customerID)
	if err != nil {
		return nil, fmt.Errorf("customers: accounts of %s: %w", customerID, err)
	}
	defer rows.Close()
	var accounts []Account
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.LedgerAccountID, &a.ProductID, &a.SortCode, &a.AccountNum, &a.OpenDate); err != nil {
			return nil, fmt.Errorf("customers: accounts of %s: %w", customerID, err)
		}
		if p, ok := c.products.ByID(a.ProductID); ok {
			a.ProductName, a.Family, a.Rate, a.Currency = p.Name, p.Family, p.Rate, p.Currency
		}
		accounts = append(accounts, a)
	}
	for i := range accounts {
		c.fillFigures(ctx, &accounts[i])
	}
	return accounts, rows.Err()
}

// fillFigures sets an account's balance and accrual from the ledger's
// live position (the latest projection plus the day's movements since)
// and its lifetime applied interest from the ledger's movements.
func (c *Customers) fillFigures(ctx context.Context, a *Account) {
	if p, err := c.books.LivePosition(ctx, a.LedgerAccountID); err != nil {
		log.Print(err)
	} else {
		a.Balance, a.AccruedE7, a.Accrued = p.Balance, p.AccruedE7, pence(p.AccruedE7)
	}
	a.Interest = c.appliedInterest(ctx, a.LedgerAccountID)
}

// poundsE7PerPenny is the number of 7dp-pound units in one penny.
const poundsE7PerPenny = 100_000

// pence truncates an accrual at 7dp of a pound to whole pence, toward
// zero: what the rules apply as postable money.
func pence(e7 int64) luca.Amount { return luca.Amount(e7 / poundsE7PerPenny) }

// appliedInterest is the interest the rules have applied to an account:
// the sum of its application movements.
func (c *Customers) appliedInterest(ctx context.Context, ledgerAccountID string) luca.Amount {
	var applied luca.Amount
	err := c.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount), 0) FROM contract_ledger_movements WHERE to_account_id = $1 AND code = $2`,
		ledgerAccountID, luca.CodeInterestAccrual).Scan(&applied)
	if err != nil {
		log.Printf("customers: applied interest of %s: %v", ledgerAccountID, err)
	}
	return applied
}

// SavingsInterest lists every customer with savings interest to date,
// applied plus accrued whole pence, oldest customer first (the BBSI
// return).
func (c *Customers) SavingsInterest(ctx context.Context) ([]core.CustomerInterest, error) {
	rows, err := c.db.QueryContext(ctx, `SELECT customer_id, ledger_account_id, product_id FROM customer_accounts ORDER BY customer_id, idx`)
	if err != nil {
		return nil, fmt.Errorf("customers: savings interest: %w", err)
	}
	defer rows.Close()
	var out []core.CustomerInterest
	for rows.Next() {
		var custID, acctID, productID string
		if err := rows.Scan(&custID, &acctID, &productID); err != nil {
			return nil, fmt.Errorf("customers: savings interest: %w", err)
		}
		if p, ok := c.products.ByID(productID); !ok || p.Family != gbp.FamilySavings {
			continue
		}
		a := Account{LedgerAccountID: acctID}
		c.fillFigures(ctx, &a)
		if interest := a.Interest + a.Accrued; interest > 0 {
			if n := len(out); n > 0 && out[n-1].CustomerID == custID {
				out[n-1].Interest += interest
			} else {
				out = append(out, core.CustomerInterest{CustomerID: custID, Interest: interest})
			}
		}
	}
	return out, rows.Err()
}

// --- Opening a customer ---

// Plan is a customer decided but not yet written: the record with its
// number, dates and account details, and the PII to store with it. What
// funds each account is the bank's decision, made on the plan.
type Plan struct {
	Record Record
	PII    core.PII
}

// Plan gives a new customer the next number, dates them on day, and gives
// each account its bank details. No accounts, or an opening amount that is
// negative, is core.ErrInvalidAmount; an unknown product is
// core.ErrNotFound. The number is used whether or not the customer is
// then persisted.
func (c *Customers) Plan(day time.Time, n core.NewCustomer) (Plan, error) {
	if len(n.Accounts) == 0 {
		return Plan{}, core.ErrInvalidAmount
	}
	for _, a := range n.Accounts {
		if a.Opening < 0 {
			return Plan{}, core.ErrInvalidAmount
		}
		if _, ok := c.products.ByID(a.ProductID); !ok {
			return Plan{}, core.ErrNotFound
		}
	}
	lastCheck := n.KYC.LastCheck
	if lastCheck.IsZero() {
		lastCheck = day
	}
	rec := Record{
		ID:       core.CustomerID(c.seq.Next()),
		JoinDate: day,
		KYC:      core.KYC{Verified: n.KYC.Verified, LastCheck: lastCheck, RiskRating: n.KYC.RiskRating},
	}
	c.mu.Lock()
	for _, a := range n.Accounts {
		product, _ := c.products.ByID(a.ProductID)
		rec.Accounts = append(rec.Accounts, Account{
			ProductID: product.ID, ProductName: product.Name, Family: product.Family, Currency: product.Currency, Rate: product.Rate,
			OpenDate: day, SortCode: SortCode, AccountNum: fmt.Sprintf("%08d", c.rng.Intn(100000000)),
		})
	}
	c.mu.Unlock()
	return Plan{Record: rec, PII: n.PII}, nil
}

// Execer is *sql.DB or *sql.Tx.
type Execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// Persist writes a planned customer: the record and PII to the store, an
// account on the ledger's chart for each of theirs, and the register
// rows. q and books are the caller's transaction (or the database); the
// store is bound to tx when one is given. A customer the store refuses is
// the error returned, and nothing of theirs is written after it.
func (c *Customers) Persist(ctx context.Context, tx *sql.Tx, q Execer, books *ledger.Ledger, plan *Plan) error {
	s := c.store
	if tx != nil {
		s = s.WithTx(tx)
	}
	rec := &plan.Record
	err := s.Create(ctx, store.CustomerRecord{
		ID: rec.ID, Ref: rec.ID, JoinDate: rec.JoinDate,
		KYCVerified: rec.KYC.Verified, KYCLastCheck: rec.KYC.LastCheck, KYCRiskRating: rec.KYC.RiskRating,
	}, store.PIIInput{Name: plan.PII.Name, NI: plan.PII.NI, DOB: plan.PII.DOB, Address: plan.PII.Address, Email: plan.PII.Email, Phone: plan.PII.Phone})
	if err != nil {
		return fmt.Errorf("customers: persist %s: %w", rec.ID, err)
	}
	for i := range rec.Accounts {
		a := &rec.Accounts[i]
		id, err := books.OpenCustomerAccount(rec.ID, a.ProductID, a.Family)
		if err != nil {
			return err
		}
		a.LedgerAccountID = id
		if _, err := q.Exec(`INSERT INTO customer_accounts
			(customer_id, idx, ledger_account_id, product_id, sort_code, account_num, opened)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			rec.ID, i, a.LedgerAccountID, a.ProductID, a.SortCode, a.AccountNum, a.OpenDate.UTC()); err != nil {
			return fmt.Errorf("customers: register account %d of %s: %w", i, rec.ID, err)
		}
	}
	return nil
}
