// Package products is the bank's product catalogue and the runner of the
// product contract (ADR-0006): the versions this build carries, their
// adoption on record, and the dispatch of each event to the version an
// account runs on. A rule reads facts through the interface the runner
// implements over the ledger and the bank's settings, and returns
// intents; the runner carries them out under the account's lock, in the
// caller's transaction, and records on every posting the version, the
// event, the parameters the rule read and the position it applied them
// to, so the posting stands without the version's code. The component
// keeps no state of its own beyond those records: an account's balance
// and its accrued-but-unapplied interest are its ledger position.
package products

import (
	"context"
	"database/sql"
	"fmt"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/parameters"
	"git.bytestone.uk/hum3/gobank/bank/schema"
)

// Listing is one product version as the bank's definition lists it: the
// version, the bank's blurb for it, and whether the bank has withdrawn it
// from sale (its accounts run on; no new account opens on it).
type Listing struct {
	Version     gbp.Version
	Description string
	Withdrawn   bool
}

// Currency is every product's money today.
const Currency = "GBP"

// Product is an adopted version as the bank sells it.
type Product struct {
	gbp.Version
	ID            string // Version.Product(): the key the register and the GL use
	Currency      string
	Description   string
	Adopted       time.Time // the business day it went on sale
	Withdrawn     time.Time // the business day it came off sale; zero while on sale
	ModuleVersion string    // the gobank-products release that carried it
}

// OnSale reports whether new accounts open on the version.
func (p Product) OnSale() bool { return p.Withdrawn.IsZero() }

// RateBps is the version's published rate, the parameter named rate_bps.
// A version that derives its rate publishes none and answers zero here;
// the rate an account earns on a day is resolved by the runner.
func (p Product) RateBps() int64 {
	if d, ok := gbp.Declared(p.Version, "rate_bps"); ok && d.Published != "" {
		param, err := d.Parse(d.Published)
		if err == nil {
			return param.Bps()
		}
	}
	return 0
}

// Schema: the adoption record and the rule postings. accrual_state (v1,
// dropped in v2) held accrued interest before positions did.
var Schema = schema.Component{Name: "products", Migrations: []schema.Migration{
	{Version: 1, Statements: []string{`CREATE TABLE IF NOT EXISTS accrual_state (
		account_id VARCHAR(64) PRIMARY KEY,
		numerator BIGINT NOT NULL,
		accrued_pounds_e7 BIGINT NOT NULL DEFAULT 0,
		as_of TIMESTAMP NOT NULL
	)`}},
	{Version: 2, Statements: []string{`DROP TABLE IF EXISTS accrual_state`}},
	{Version: 3, Statements: []string{
		`CREATE TABLE IF NOT EXISTS products (
			product_id VARCHAR(50) NOT NULL,
			version INTEGER NOT NULL,
			adopted TIMESTAMP NOT NULL,
			withdrawn TIMESTAMP,
			module_version VARCHAR(50) NOT NULL,
			parameters VARCHAR(400) NOT NULL,
			PRIMARY KEY (product_id, version)
		)`,
		`CREATE TABLE IF NOT EXISTS product_postings (
			movement_id VARCHAR(64) PRIMARY KEY,
			account_id VARCHAR(64) NOT NULL,
			product_id VARCHAR(50) NOT NULL,
			version INTEGER NOT NULL,
			event VARCHAR(20) NOT NULL,
			parameters VARCHAR(400) NOT NULL,
			input_balance BIGINT NOT NULL,
			input_accrued_num BIGINT NOT NULL,
			posted_at TIMESTAMP NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS product_postings_account ON product_postings (account_id)`,
	}},
}}

// Products is the catalogue over the bank's database.
type Products struct {
	db       *sql.DB
	params   *parameters.Parameters
	listings []Listing // what the bank's definition carries, catalogue order
	adopted  []Product // the same, on record
	now      func() time.Time
}

// New is the catalogue on db for the versions the bank's definition
// lists, with nothing on record until Adopt runs.
func New(db *sql.DB, params *parameters.Parameters, listings []Listing) *Products {
	return &Products{db: db, params: params, listings: listings, now: time.Now}
}

// Adopt is each listed version's up, and its down: a version not yet on
// record is recorded as adopted on day, with the module version that
// carried it and its parameters as published; a version the definition
// has withdrawn is recorded as withdrawn on day, once. A record already
// there stands, so the days are those of the first run that knew. The
// catalogue is then every listed version on record.
func (p *Products) Adopt(ctx context.Context, day time.Time) error {
	module := moduleVersion()
	p.adopted = p.adopted[:0]
	for _, l := range p.listings {
		v := l.Version
		prod := Product{Version: v, ID: v.Product(), Currency: Currency, Description: l.Description}
		var withdrawn sql.NullTime
		err := p.db.QueryRowContext(ctx, `SELECT adopted, withdrawn, module_version FROM products WHERE product_id = $1 AND version = $2`, v.Product(), v.Version()).
			Scan(&prod.Adopted, &withdrawn, &prod.ModuleVersion)
		switch {
		case err == sql.ErrNoRows:
			prod.Adopted, prod.ModuleVersion = day.UTC(), module
			if _, err := p.db.ExecContext(ctx, `INSERT INTO products (product_id, version, adopted, module_version, parameters) VALUES ($1, $2, $3, $4, $5)`,
				v.Product(), v.Version(), prod.Adopted, module, published(v)); err != nil {
				return fmt.Errorf("products: adopt %s v%d: %w", v.Product(), v.Version(), err)
			}
		case err != nil:
			return fmt.Errorf("products: adoption of %s v%d: %w", v.Product(), v.Version(), err)
		case withdrawn.Valid:
			prod.Withdrawn = withdrawn.Time
		}
		if l.Withdrawn && prod.Withdrawn.IsZero() {
			prod.Withdrawn = day.UTC()
			if _, err := p.db.ExecContext(ctx, `UPDATE products SET withdrawn = $1 WHERE product_id = $2 AND version = $3`, prod.Withdrawn, v.Product(), v.Version()); err != nil {
				return fmt.Errorf("products: withdraw %s v%d: %w", v.Product(), v.Version(), err)
			}
		}
		p.adopted = append(p.adopted, prod)
	}
	return nil
}

// published is a version's version-scoped parameters as text: "rate_bps=150 cycle=monthly".
func published(v gbp.Version) string {
	var parts []string
	for _, d := range v.Parameters() {
		if d.Scope == gbp.ScopeVersion && d.Published != "" {
			parts = append(parts, d.Key+"="+d.Published)
		}
	}
	return strings.Join(parts, " ")
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

// All is the catalogue: every carried version on record, in catalogue
// order.
func (p *Products) All() []Product { return p.adopted }

// ByID is the version of a product on sale: the highest listed version
// not withdrawn. A product every version of which is withdrawn is found
// by its highest version, so its accounts still read as the product.
func (p *Products) ByID(product string) (Product, bool) {
	var best Product
	found := false
	better := func(c Product) bool {
		if !found {
			return true
		}
		if c.OnSale() != best.OnSale() {
			return c.OnSale()
		}
		return c.Version.Version() > best.Version.Version()
	}
	for _, prod := range p.adopted {
		if prod.ID == product && better(prod) {
			best, found = prod, true
		}
	}
	return best, found
}

// At is one version of a product, if the build carries it.
func (p *Products) At(product string, version int) (Product, bool) {
	for _, prod := range p.adopted {
		if prod.ID == product && prod.Version.Version() == version {
			return prod, true
		}
	}
	return Product{}, false
}

// Family is a product's family, by any version of it.
func (p *Products) Family(product string) (gbp.ProductFamily, bool) {
	prod, ok := p.ByID(product)
	if !ok {
		return "", false
	}
	return prod.Family(), true
}

// AccountsByVersion counts the registered accounts on each version.
func (p *Products) AccountsByVersion(ctx context.Context) (map[VersionKey]int, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT product_id, product_version, COUNT(*) FROM contract_customer_accounts GROUP BY product_id, product_version`)
	if err != nil {
		return nil, fmt.Errorf("products: accounts by version: %w", err)
	}
	defer rows.Close()
	out := map[VersionKey]int{}
	for rows.Next() {
		var k VersionKey
		var n int
		if err := rows.Scan(&k.Product, &k.Version, &n); err != nil {
			return nil, fmt.Errorf("products: accounts by version: %w", err)
		}
		out[k] = n
	}
	return out, rows.Err()
}

// VersionKey names a version: product and number.
type VersionKey struct {
	Product string
	Version int
}

// Posting is a rule posting as recorded: why a movement was made.
type Posting struct {
	MovementID      string
	AccountID       string
	Product         string
	Version         int
	Event           gbp.Event
	Parameters      string // what the rule read, "rate_bps=150 cycle=monthly"
	InputBalance    int64  // the position the rule applied them to
	InputAccruedNum int64
	PostedAt        time.Time
}

// Postings is an account's rule postings, oldest first.
func (p *Products) Postings(ctx context.Context, accountID string) ([]Posting, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT movement_id, account_id, product_id, version, event, parameters, input_balance, input_accrued_num, posted_at
		FROM product_postings WHERE account_id = $1 ORDER BY posted_at`, accountID)
	if err != nil {
		return nil, fmt.Errorf("products: postings of %s: %w", accountID, err)
	}
	defer rows.Close()
	var out []Posting
	for rows.Next() {
		var r Posting
		if err := rows.Scan(&r.MovementID, &r.AccountID, &r.Product, &r.Version, &r.Event, &r.Parameters, &r.InputBalance, &r.InputAccruedNum, &r.PostedAt); err != nil {
			return nil, fmt.Errorf("products: postings of %s: %w", accountID, err)
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PostedAt.Before(out[j].PostedAt) })
	return out, rows.Err()
}
