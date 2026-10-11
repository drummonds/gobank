// Package products is the bank's product catalogue and the runner of the
// product contract (ADR-0006): the products the bank has instantiated,
// each on a rules version the build carries, their adoption on record,
// and the dispatch of each event to the rules an account's product runs
// on. A rule reads facts through the interface the runner implements
// over the ledger and the bank's settings, and returns intents; the
// runner carries them out under the account's lock, in the caller's
// transaction, and records on every posting the product, its rules, the
// event, the parameters the rule read and the position it applied them
// to, so the posting stands without the rules' code. The component keeps
// no state of its own beyond those records: an account's balance and its
// accrued-but-unapplied interest are its ledger position.
package products

import (
	"context"
	"database/sql"
	"fmt"
	"runtime/debug"
	"sort"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/parameters"
	"git.bytestone.uk/hum3/gobank/bank/schema"
)

// Listing is one product as the bank's definition lists it: its own ID
// and name, the rules version it is an instance of, the bank's blurb for
// it, and whether the bank has withdrawn it from sale (its accounts run
// on; no new account opens on it).
type Listing struct {
	ID          string
	Name        string
	Rules       gbp.Version
	Description string
	Withdrawn   bool
}

// Currency is every product's money today.
const Currency = "GBP"

// Product is a product on record as the bank sells it: an instance of a
// rules version, with its own identity.
type Product struct {
	ID            string
	Name          string
	Rules         gbp.Version // the rules it runs on; nil when this build does not carry them
	RulesID       string      // the rules' name and version as recorded: what Rules says when carried
	RulesVersion  int
	Currency      string
	Description   string
	Adopted       time.Time // the business day it went on sale
	Withdrawn     time.Time // the business day it came off sale; zero while on sale
	ModuleVersion string    // the gobank-products release that carried its rules at adoption
}

// OnSale reports whether new accounts open on the product.
func (p Product) OnSale() bool { return p.Withdrawn.IsZero() }

// Carried reports whether this build carries the product's rules: only
// then can its accounts be run.
func (p Product) Carried() bool { return p.Rules != nil }

// Family is the product's family, by its rules; a product not carried
// answers savings, the family of nothing it can do.
func (p Product) Family() gbp.ProductFamily {
	if p.Rules == nil {
		return gbp.FamilySavings
	}
	return p.Rules.Family()
}

// RateBps is the rate the product's rules publish, the parameter named
// rate_bps. A product whose rules derive it, or that is not carried,
// answers zero here; the rate an account earns on a day is resolved by
// the runner.
func (p Product) RateBps() int64 {
	if p.Rules == nil {
		return 0
	}
	if d, ok := gbp.Declared(p.Rules, "rate_bps"); ok && d.Published != "" {
		param, err := d.Parse(d.Published)
		if err == nil {
			return param.Bps()
		}
	}
	return 0
}

// Schema: the products on record and the rule postings. accrual_state
// (v1, dropped in v2) held accrued interest before positions did.
var Schema = schema.Component{Name: "products", Migrations: []schema.Migration{
	{Version: 1, Statements: []string{`CREATE TABLE IF NOT EXISTS accrual_state (
		account_id VARCHAR(64) PRIMARY KEY,
		accrued_num BIGINT NOT NULL,
		accrued_den BIGINT NOT NULL,
		as_of TIMESTAMP NOT NULL
	)`}},
	{Version: 2, Statements: []string{`DROP TABLE IF EXISTS accrual_state`}},
	{Version: 3, Statements: []string{
		`CREATE TABLE IF NOT EXISTS products (
			product_id VARCHAR(50) PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			rules VARCHAR(50) NOT NULL,
			rules_version INTEGER NOT NULL,
			adopted TIMESTAMP NOT NULL,
			withdrawn TIMESTAMP,
			module_version VARCHAR(50) NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS product_postings (
			movement_id VARCHAR(64) PRIMARY KEY,
			account_id VARCHAR(64) NOT NULL,
			product_id VARCHAR(50) NOT NULL,
			rules VARCHAR(50) NOT NULL,
			rules_version INTEGER NOT NULL,
			event VARCHAR(20) NOT NULL,
			parameters VARCHAR(400) NOT NULL,
			input_balance BIGINT NOT NULL,
			input_accrued_num BIGINT NOT NULL,
			posted_at TIMESTAMP NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS product_postings_account ON product_postings (account_id)`,
	}},
	// The instant up to which each account's parameters have been applied
	// (ADR-0010): a setting effective after it is a ParameterChange owed.
	// An account with no row is from before the record and counts as
	// applied up to the first instant the cascade runs.
	{Version: 4, Statements: []string{`CREATE TABLE IF NOT EXISTS product_account_parameters (
		account_id VARCHAR(64) PRIMARY KEY,
		applied_to TIMESTAMP NOT NULL
	)`}},
}}

// Products is the catalogue over the bank's database.
type Products struct {
	db        *sql.DB
	params    *parameters.Parameters
	listings  []Listing // what the bank's definition lists, catalogue order
	catalogue []Product // every product on record, the listed ones first
	now       func() time.Time
}

// New is the catalogue on db for the products the bank's definition
// lists, with nothing on record until Adopt runs.
func New(db *sql.DB, params *parameters.Parameters, listings []Listing) *Products {
	return &Products{db: db, params: params, listings: listings, now: time.Now}
}

// rulesKey names a rules version: its name and number.
type rulesKey struct {
	Rules   string
	Version int
}

// carried is the rules versions this build carries: those the
// definition's products run on.
func (p *Products) carried() map[rulesKey]gbp.Version {
	out := map[rulesKey]gbp.Version{}
	for _, l := range p.listings {
		if l.Rules != nil {
			out[rulesKey{l.Rules.Product(), l.Rules.Version()}] = l.Rules
		}
	}
	return out
}

// Adopt is each listed product's up, and its down: a product not yet on
// record is recorded as adopted on day, with its rules and the module
// version that carried them; a product the definition has withdrawn is
// recorded as withdrawn on day, once. A record already there stands, so
// the days are those of the first run that knew. The catalogue is then
// every product on record, listed or not, with the rules resolved from
// what the build carries.
func (p *Products) Adopt(ctx context.Context, day time.Time) error {
	module := moduleVersion()
	for _, l := range p.listings {
		if l.Rules == nil {
			return fmt.Errorf("products: %s lists no rules", l.ID)
		}
		var withdrawn sql.NullTime
		err := p.db.QueryRowContext(ctx, `SELECT withdrawn FROM products WHERE product_id = $1`, l.ID).Scan(&withdrawn)
		switch {
		case err == sql.ErrNoRows:
			if _, err := p.db.ExecContext(ctx, `INSERT INTO products (product_id, name, rules, rules_version, adopted, module_version) VALUES ($1, $2, $3, $4, $5, $6)`,
				l.ID, l.Name, l.Rules.Product(), l.Rules.Version(), day.UTC(), module); err != nil {
				return fmt.Errorf("products: adopt %s: %w", l.ID, err)
			}
		case err != nil:
			return fmt.Errorf("products: adoption of %s: %w", l.ID, err)
		}
		if l.Withdrawn && !withdrawn.Valid {
			if _, err := p.db.ExecContext(ctx, `UPDATE products SET withdrawn = $1 WHERE product_id = $2`, day.UTC(), l.ID); err != nil {
				return fmt.Errorf("products: withdraw %s: %w", l.ID, err)
			}
		}
	}
	return p.load(ctx)
}

// load reads every product on record into the catalogue, the listed ones
// first in catalogue order, the rest by ID.
func (p *Products) load(ctx context.Context) error {
	rows, err := p.db.QueryContext(ctx, `SELECT product_id, name, rules, rules_version, adopted, withdrawn, module_version FROM products ORDER BY product_id`)
	if err != nil {
		return fmt.Errorf("products: catalogue: %w", err)
	}
	defer rows.Close()
	carried := p.carried()
	byID := map[string]Product{}
	var ids []string
	for rows.Next() {
		var prod Product
		var withdrawn sql.NullTime
		if err := rows.Scan(&prod.ID, &prod.Name, &prod.RulesID, &prod.RulesVersion, &prod.Adopted, &withdrawn, &prod.ModuleVersion); err != nil {
			return fmt.Errorf("products: catalogue: %w", err)
		}
		if withdrawn.Valid {
			prod.Withdrawn = withdrawn.Time
		}
		prod.Currency = Currency
		prod.Rules = carried[rulesKey{prod.RulesID, prod.RulesVersion}]
		byID[prod.ID] = prod
		ids = append(ids, prod.ID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("products: catalogue: %w", err)
	}
	catalogue := make([]Product, 0, len(byID))
	listed := map[string]bool{}
	for _, l := range p.listings {
		if prod, ok := byID[l.ID]; ok {
			prod.Description = l.Description
			catalogue = append(catalogue, prod)
			listed[l.ID] = true
		}
	}
	for _, id := range ids {
		if !listed[id] {
			catalogue = append(catalogue, byID[id])
		}
	}
	p.catalogue = catalogue
	return nil
}

// moduleVersion is the gobank-products release built in, "(devel)" when
// the build carries no record of it.
func moduleVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "git.bytestone.uk/hum3/gobank-products" {
				return dep.Version
			}
		}
	}
	return "(devel)"
}

// All is the catalogue: every product on record, the listed ones first.
func (p *Products) All() []Product { return p.catalogue }

// ByID is a product on record.
func (p *Products) ByID(id string) (Product, bool) {
	for _, prod := range p.catalogue {
		if prod.ID == id {
			return prod, true
		}
	}
	return Product{}, false
}

// Family is a product's family.
func (p *Products) Family(id string) (gbp.ProductFamily, bool) {
	prod, ok := p.ByID(id)
	if !ok {
		return "", false
	}
	return prod.Family(), true
}

// AccountsByProduct counts the registered accounts on each product.
func (p *Products) AccountsByProduct(ctx context.Context) (map[string]int, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT product_id, COUNT(*) FROM contract_customer_accounts GROUP BY product_id`)
	if err != nil {
		return nil, fmt.Errorf("products: accounts by product: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("products: accounts by product: %w", err)
		}
		out[id] = n
	}
	return out, rows.Err()
}

// Posting is a rule posting as recorded: why a movement was made.
type Posting struct {
	MovementID      string
	AccountID       string
	Product         string
	Rules           string // the rules the product ran on when it posted
	RulesVersion    int
	Event           gbp.Event
	Parameters      string // what the rule read, "rate_bps=150 cycle=monthly"
	InputBalance    int64  // the position the rule applied them to
	InputAccruedNum int64
	PostedAt        time.Time
}

// Postings is an account's rule postings, oldest first.
func (p *Products) Postings(ctx context.Context, accountID string) ([]Posting, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT movement_id, account_id, product_id, rules, rules_version, event, parameters, input_balance, input_accrued_num, posted_at
		FROM product_postings WHERE account_id = $1 ORDER BY posted_at`, accountID)
	if err != nil {
		return nil, fmt.Errorf("products: postings of %s: %w", accountID, err)
	}
	defer rows.Close()
	var out []Posting
	for rows.Next() {
		var r Posting
		if err := rows.Scan(&r.MovementID, &r.AccountID, &r.Product, &r.Rules, &r.RulesVersion, &r.Event, &r.Parameters, &r.InputBalance, &r.InputAccruedNum, &r.PostedAt); err != nil {
			return nil, fmt.Errorf("products: postings of %s: %w", accountID, err)
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PostedAt.Before(out[j].PostedAt) })
	return out, rows.Err()
}
