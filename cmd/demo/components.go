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
		Purpose: "The run's place in time (the instant the clock stands at, whether the run loop is going, so a restart resumes), the restart record (each process start against the stop before it, for an upgrade's downtime) and the simulation (package sim, ADR-0010): the stepped clock the bank reads, the timeline of events (the start of each day, the market's decision days and publications, the generator's payments, the settings staff make), the Bank of England's series replayed as the pound's feed, the generators for the population and its payments, and the run loop that moves the clock to the next event only when the bank is at rest, all reaching the bank through the core alone.",
		Files:   []string{"run.go", "restarts.go", "console.go", "sim/sim.go", "sim/clock.go", "sim/events.go", "sim/customers.go", "sim/payments.go", "sim/rates.go", "sim/daylength.go"},
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
		Name:    "gl",
		Purpose: "The general ledger, the bank's own books (ADR-0005): a control account per product and the bank's own accounts, posted per event from the sub-ledger's movements and by a journal at the GL close behind each day's pass, and reconciled to the sub-ledger daily. In shadow until story 1.6a.3 moves the bank-level reads onto it.",
		Library: "go-luca",
		Package: "bank/gl",
		Views:   []string{"contract_gl_ledger_movements", "contract_gl_ledger_eod_positions", "contract_gl_ledger_live_positions", "contract_gl_ledger_latest_positions"},
		Tables: []string{"gl_accounts", "gl_movements", "gl_balances_live", "gl_ledger_day", "gl_aliases", "gl_data_points",
			"gl_movement_metadata", "gl_commodities", "gl_commodity_metadata", "gl_customers",
			"gl_customer_metadata", "gl_options", "gl_opening", "gl_closes", "gl_journal", "gl_reconciliations"},
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
		Purpose: "The product catalogue (the versions the bank's definition carries, their adoption and withdrawal on record) and the runner of the product contract (ADR-0006): each event is dispatched to the version an account runs on, one account at a time, and every rule posting is recorded with the version, the event, the parameters read and the position they applied to; positions live in the ledger. The instant each account's parameters are applied up to drives the ParameterChange cascade (ADR-0010).",
		Library: "gobank-products",
		Package: "bank/products",
		Tables:  []string{"products", "product_postings", "product_account_parameters"},
	},
	{
		Name:    "events",
		Purpose: "The record of what happened to the bank and what it caused (ADR-0011): every command a root event, raised pending at the instant its work is due and done when its cascade has finished; postings, settings and payments carry their root as their cause, and a restart replays the roots due and not done, each effect proven by its own fact.",
		Package: "bank/events",
		Tables:  []string{"events"},
	},
	{
		Name:    "parameters",
		Purpose: "The bank's settings (ADR-0006): the stored values of the parameters product versions, the bank's policy and the markets declare, each with the instant it is effective from and the moment it was decided, append-only, resolved at an instant as known at a moment; a day's value is its end-of-day value (ADR-0009).",
		Package: "bank/parameters",
		Tables:  []string{"parameter_settings"},
		Views:   []string{"contract_parameter_settings"},
	},
	{
		Name:    "market",
		Purpose: "Where a currency's official risk-free rate comes from (ADR-0009): the markets the bank knows (GBP: the Bank of England's Bank Rate, published at noon London time on a decision day, with the MPC's calendar of decision days), each publication a setting at the market scope that only the market's feed writes, through one core command. Owns no table.",
		Package: "bank/market",
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
		Purpose: "The bank's daily series for the dashboard charts: one snapshot a day of the book, the customers, the NIM and the risk-free rate.",
		Package: "bank/history",
		Tables:  []string{"daily_snapshots"},
	},
	{
		Name:    "sessions",
		Purpose: "The customer app's and the staff web's live sessions, kept by the BFF so a restart keeps everyone logged in.",
		Library: "bff",
		Files:   []string{"sessions.go", "app_bff.go"},
		Tables:  []string{"sessions", "staff_sessions"},
	},
	{
		Name:    "users",
		Purpose: "Who may use the bank: a user is a login name, a password hash (argon2id) and the roles it holds; staff and customers are one table told apart by role. The staff web signs in through it.",
		Package: "bank/users",
		Tables:  []string{"users", "user_roles"},
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
