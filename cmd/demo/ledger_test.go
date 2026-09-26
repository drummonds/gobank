package main

import "testing"

// contract_ledger_movements is how other components read the ledger: every
// movement with its accounts, amount, code, value date and description.
func TestContractLedgerMovementsView(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	ds.AdvanceDay()
	var movements, viewRows int
	if err := ds.db.QueryRow(`SELECT COUNT(*) FROM movements`).Scan(&movements); err != nil {
		t.Fatal(err)
	}
	if err := ds.db.QueryRow(`SELECT COUNT(*) FROM contract_ledger_movements
		WHERE id <> '' AND from_account_id <> '' AND to_account_id <> '' AND code <> '' AND value_time IS NOT NULL AND description IS NOT NULL`).Scan(&viewRows); err != nil {
		t.Fatal(err)
	}
	if movements == 0 || viewRows != movements {
		t.Errorf("view has %d rows, ledger has %d movements", viewRows, movements)
	}
}
