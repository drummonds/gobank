package main

// The component registry: which function of the bank owns which package
// (or source files, for the demo's own components) and tables, and what
// it publishes. It is the single source for the
// documentation page (docs.go), the contract-view rule (ADR-0001, enforced
// by TestContractViewRule) and the explorer's component catalog.

import (
	"slices"

	"git.bytestone.uk/hum3/gobank/bff/staff"
)

// Component is one function of the bank: a package under bank/ once it has
// moved there (ADR-0002 stage 5), a set of files in this package until then.
// The staff web's documentation page renders the registry (staff.Component).
type Component = staff.Component

var components = []Component{
	{
		Name:    "schema",
		Purpose: "Which version each component's tables are at; migrations bring a database up to date at start.",
		Files:   []string{"schema.go"},
		Tables:  []string{"schema_versions"},
	},
	{
		Name:    "simulation",
		Purpose: "The run's place in time (the day the bank is on, whether the run loop is going, so a restart resumes), the restart record (each process start against the stop before it, for an upgrade's downtime) and the simulation (package sim): the warped clock the bank reads, the replayed base rate, the generators for the population and its payments, and the run loop, all reaching the bank through the core alone.",
		Files:   []string{"run.go", "restarts.go", "console.go", "sim/sim.go", "sim/clock.go", "sim/customers.go", "sim/payments.go", "sim/rates.go", "sim/daylength.go"},
		Tables:  []string{"sim_run", "restarts"},
	},
	{
		Name:    "ledger",
		Purpose: "Double-entry books of account: every balance is the sum of its movements.",
		Library: "go-luca",
		Package: "bank/ledger",
		Views:   []string{"contract_ledger_movements", "contract_ledger_eod_positions", "contract_ledger_live_positions", "contract_ledger_latest_positions"},
		Tables: []string{"accounts", "movements", "balances_live", "ledger_day", "aliases", "data_points",
			"movement_metadata", "commodities", "commodity_metadata", "customers",
			"customer_metadata", "options"},
	},
	{
		Name:    "customers",
		Purpose: "Who the bank's customers are, their KYC status, and the accounts each holds.",
		Library: "gobanks-customers",
		Package: "bank/customers",
		Tables:  []string{"cust_customers", "cust_pii", "customer_accounts"},
		Views:   []string{"contract_customer_accounts"},
	},
	{
		Name:    "products",
		Purpose: "The product catalogue and the interest rules, run for one account at a time by the start-of-day pass and by every event that moves a balance; positions live in the ledger.",
		Library: "gobank-products",
		Package: "bank/products",
	},
	{
		Name:    "payments",
		Purpose: "Moving money between customer accounts and to or from the outside world.",
		Package: "bank/payments",
		Tables:  []string{"payments"},
		Views:   []string{"contract_payments"},
	},
	{
		Name:    "history",
		Purpose: "The bank's daily series for the dashboard charts: one snapshot a day of the book, the customers, the NIM and the base rate.",
		Package: "bank/history",
		Tables:  []string{"daily_snapshots"},
	},
	{
		Name:    "sessions",
		Purpose: "The customer app's live sessions, kept by the BFF so a restart keeps customers logged in.",
		Library: "bff",
		Files:   []string{"sessions.go", "app_bff.go"},
		Tables:  []string{"sessions"},
	},
	{
		Name:    "treasury",
		Purpose: "Gilt yields and holdings: where the bank places its liquidity.",
		Package: "bank/treasury",
		Tables:  []string{"gilt_yields", "gilt_holdings"},
	},
}

// contractDebt is the pre-rule baseline: cross-reads that predated
// ADR-0001. It was burnt down to nothing and stays empty; a new cross-read
// gets a contract view or an API, not an entry here.
var contractDebt = []staff.CrossRead{}

// componentOf returns the component owning a table, or "" if none does.
func componentOf(table string) string {
	for _, c := range components {
		if slices.Contains(c.Tables, table) {
			return c.Name
		}
	}
	return ""
}
