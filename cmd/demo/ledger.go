package main

import "log"

// The ledger component is go-luca: its tables are the books of account.
// This file publishes the contract views other components read (ADR-0001).

// createLedgerViews (re)creates the ledger's contract views. Called once
// the ledger has created its tables.
func (ds *DemoState) createLedgerViews() {
	if ds.db == nil {
		return
	}
	stmts := []string{
		`DROP VIEW IF EXISTS contract_ledger_movements`,
		`CREATE VIEW contract_ledger_movements AS
			SELECT id, from_account_id, to_account_id, amount, code, value_time, description FROM movements`,
	}
	for _, stmt := range stmts {
		if _, err := ds.db.Exec(stmt); err != nil {
			log.Printf("initLedger: views: %v", err)
		}
	}
}
