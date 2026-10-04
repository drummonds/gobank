package main

// The component registry: which function of the bank owns which source
// files and tables, and what it publishes. It is the single source for the
// documentation page (docs.go), the contract-view rule (ADR-0001, enforced
// by TestContractViewRule) and the explorer's component catalog.

import "slices"

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
		Name:    "schema",
		Purpose: "Which version each component's tables are at; migrations bring a database up to date at start.",
		Files:   []string{"schema.go"},
		Tables:  []string{"schema_versions"},
	},
	{
		Name:    "simulation",
		Purpose: "The run's place in time (the day the bank is on, whether the run loop is going, so a restart resumes) and the restart record: each process start against the stop before it, for an upgrade's downtime.",
		Files:   []string{"run.go", "restarts.go"},
		Tables:  []string{"sim_run", "restarts"},
	},
	{
		Name:    "ledger",
		Purpose: "Double-entry books of account: every balance is the sum of its movements.",
		Library: "go-luca",
		Files:   []string{"ledger.go"},
		Views:   []string{"contract_ledger_movements"},
		Tables: []string{"accounts", "movements", "balances_live", "aliases", "data_points",
			"movement_metadata", "commodities", "commodity_metadata", "customers",
			"customer_metadata", "options"},
	},
	{
		Name:    "customers",
		Purpose: "Who the bank's customers are, their KYC status, and the accounts each holds.",
		Library: "gobanks-customers",
		Files:   []string{"customers.go", "customers_http.go", "customer_register.go", "customer_gen.go", "pii_store.go", "transactions.go"},
		Tables:  []string{"cust_customers", "cust_pii", "customer_accounts"},
		Views:   []string{"contract_customer_accounts"},
	},
	{
		Name:    "products",
		Purpose: "The product catalogue and the interest engine: exact daily accrual, monthly application.",
		Library: "gobank-products",
		Files:   []string{"products.go", "accrual.go"},
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
		Name:    "history",
		Purpose: "The bank's daily series for the dashboard charts: one snapshot a day of the book, the customers, the NIM and the base rate.",
		Files:   []string{"history.go"},
		Tables:  []string{"daily_snapshots"},
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

// contractDebt is the pre-rule baseline: cross-reads that predated
// ADR-0001. It was burnt down to nothing and stays empty; a new cross-read
// gets a contract view or an API, not an entry here.
var contractDebt = []crossRead{}

// componentOf returns the component owning a table, or "" if none does.
func componentOf(table string) string {
	for _, c := range components {
		if slices.Contains(c.Tables, table) {
			return c.Name
		}
	}
	return ""
}
