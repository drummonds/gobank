package main

// The component registry: which function of the bank owns which source
// files and tables, and what it publishes. It is the single source for the
// documentation page (docs.go), the contract-view rule (ADR-0001, enforced
// by TestContractViewRule) and the explorer's ownership badges.

// Component is one function of the bank.
type Component struct {
	Name    string   // domain name, e.g. "payments"
	Purpose string   // one line
	Library string   // external module that owns the schema, if any
	Files   []string // source files in this package the component owns
	Tables  []string // internal tables: only this component's code touches them
	Views   []string // contract views (contract_*) other code reads
}

var components = []Component{
	{
		Name:    "ledger",
		Purpose: "Double-entry books of account: every balance is the sum of its movements.",
		Library: "go-luca",
		Tables: []string{"accounts", "movements", "balances_live", "aliases", "data_points",
			"movement_metadata", "commodities", "commodity_metadata", "customers",
			"customer_metadata", "options"},
	},
	{
		Name:    "customers",
		Purpose: "Who the bank's customers are, their KYC status, and the accounts each holds.",
		Library: "gobanks-customers",
		Files:   []string{"customers.go", "customers_http.go", "customer_register.go", "customer_gen.go", "pii_store.go"},
		Tables:  []string{"cust_customers", "cust_pii", "customer_accounts"},
	},
	{
		Name:    "products",
		Purpose: "The product catalogue and the interest engine: exact daily accrual, monthly application.",
		Library: "gobank-products",
		Files:   []string{"products.go"},
		Tables:  []string{"accrual_state"},
	},
	{
		Name:    "payments",
		Purpose: "Moving money between customer accounts and to or from the outside world.",
		Files:   []string{"payments.go"},
		Tables:  []string{"payments"},
		Views:   []string{"contract_payments"},
	},
	{
		Name:    "treasury",
		Purpose: "Gilt yields and holdings: where the bank places its liquidity.",
		Files:   []string{"treasury.go"},
		Tables:  []string{"gilt_yields", "gilt_holdings"},
	},
}

// crossRead is a file reading a table it does not own.
type crossRead struct {
	File  string
	Table string
}

// contractDebt is the pre-rule baseline: cross-reads that predate ADR-0001.
// Each stays allowed until it is replaced by a contract view or an API
// call, at which point its entry must be removed. The list only shrinks.
var contractDebt = []crossRead{
	{"book.go", "customer_accounts"},
	{"book.go", "movements"},
	{"customer_register.go", "movements"},
	{"db.go", "accrual_state"},
	{"db.go", "gilt_holdings"},
	{"db.go", "gilt_yields"},
	{"demo_state.go", "accrual_state"},
	{"demo_state.go", "customer_accounts"},
}

// componentOf returns the component owning a table, or "" if none does.
func componentOf(table string) string {
	for _, c := range components {
		for _, t := range c.Tables {
			if t == table {
				return c.Name
			}
		}
	}
	return ""
}
