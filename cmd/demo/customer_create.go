package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	customers "git.bytestone.uk/hum3/gobanks-customers"
)

// Creating a customer is two steps so ds.mu never spans a database round
// trip: plan (under ds.mu, in memory only) decides everything that depends
// on shared state — the generated customer, its funding, the book, payment
// IDs — then persist writes it in one transaction with no ds.mu held, taking
// the lock again only briefly to finish up.

// customerPlan is one new customer decided under ds.mu, with the handles to
// persist it.
type customerPlan struct {
	epoch    int // ds.epoch when planned; a Reset since then voids the plan
	cust     CustomerRecord
	pii      PIIInput
	day      time.Time
	funding  []customerFunding
	db       *sql.DB
	ledger   *luca.SQLLedger
	store    *customers.SQLCustomerStore
	sim      *gbp.Simulation
	equityID string
}

// customerFunding is the opening deposit or loan disbursement for one of the
// customer's accounts.
type customerFunding struct {
	idx     int // index into cust.Accounts
	payment Payment
}

// createCustomer generates, persists, registers and funds one customer.
// Must not be called with ds.mu held.
func (ds *DemoState) createCustomer() {
	ds.mu.Lock()
	p := ds.planCustomerLocked()
	ds.mu.Unlock()
	ds.persistCustomerPlan(p)
}

// planCustomerLocked generates the next customer and decides its funding,
// putting it on the books. Must be called with ds.mu held.
func (ds *DemoState) planCustomerLocked() customerPlan {
	cust, pii := generateCustomer(ds.rng, ds.nextCustSeq, ds.products, ds.currentDay)
	ds.nextCustSeq++
	p := customerPlan{
		epoch: ds.epoch, cust: cust, pii: pii, day: ds.currentDay,
		db: ds.db, ledger: ds.ledger, store: ds.custStore, sim: ds.sim, equityID: ds.equityAccountID,
	}
	for i := range p.cust.Accounts {
		a := &p.cust.Accounts[i]
		var amount luca.Amount
		ptype, from := PayDeposit, "EXTERNAL"
		if a.Family == gbp.FamilySavings {
			amount = luca.Amount(500+ds.rng.Intn(9500)) * 100
		} else {
			headroom := ds.lendingHeadroom()
			if headroom <= 0 {
				continue
			}
			amount = min(luca.Amount(1000+ds.rng.Intn(49000))*100, headroom)
			ptype, from = PayLoanDisbursement, "BANK"
		}
		a.Balance = amount
		ds.addToBook(a.Family, amount)
		p.funding = append(p.funding, customerFunding{idx: i, payment: ds.newPaymentLocked(ptype, from, cust.ID, amount)})
	}
	ds.nCustomers++
	return p
}

// newPaymentLocked allocates a payment that settles immediately. Must be
// called with ds.mu held.
func (ds *DemoState) newPaymentLocked(ptype PaymentType, fromID, toID string, amount luca.Amount) Payment {
	now := time.Now()
	p := Payment{
		ID: ds.nextPaymentID, Type: ptype, FromID: fromID, ToID: toID, Amount: amount,
		Status: PaymentCompleted, Reference: fmt.Sprintf("PAY-%06d", ds.nextPaymentID),
		CreatedAt: now, SettledAt: now,
	}
	ds.nextPaymentID++
	return p
}

// persistCustomerPlan writes a planned customer — record, ledger accounts,
// register, funding payments and movements — in one transaction, so each
// customer is one commit. A customer the database refuses is taken back off
// the books; later failures are logged and the customer kept, as before the
// split. Must not be called with ds.mu held.
func (ds *DemoState) persistCustomerPlan(p customerPlan) {
	store, sim := p.store, p.sim
	var tx *sql.Tx
	if p.db != nil && p.ledger != nil {
		var err error
		if tx, err = p.db.Begin(); err != nil {
			log.Printf("createCustomer: begin: %v", err)
			tx = nil // fall back to autocommit writes
		}
	}
	if tx != nil {
		if store != nil {
			store = store.WithTx(tx)
		}
		if sim != nil {
			// Shallow copy sharing the account/product maps, swapping only the
			// ledger; under simMu because the engine sweep writes its fields.
			ds.simMu.Lock()
			txSim := *sim
			ds.simMu.Unlock()
			txSim.Ledger = p.ledger.WithTx(tx)
			sim = &txSim
		}
	}

	if err := persistCustomer(store, &p.cust, p.pii); err != nil {
		// Drop the customer entirely rather than keeping an in-memory ghost
		// the database refused.
		log.Printf("createCustomer: persist %s: %v", p.cust.ID, err)
		if tx != nil {
			_ = tx.Rollback()
		}
		ds.mu.Lock()
		ds.abandonPlanLocked(p)
		ds.mu.Unlock()
		return
	}
	ds.addCustomerToLedger(sim, &p.cust)
	// Funding rewrites the equity account's position and the new accounts'
	// inside this transaction, so their locks are held until it commits:
	// creators take turns on the equity account, and the daily pass waits
	// for a new account's funding before projecting it.
	locked := []string{p.equityID}
	for _, a := range p.cust.Accounts {
		locked = append(locked, a.LedgerAccountID)
	}
	unlock := ds.accountLocks.lock(locked...)
	defer unlock()
	var q execer = p.db
	if tx != nil {
		q = tx
	}
	if q != nil {
		if err := registerAccounts(q, &p.cust); err != nil {
			log.Printf("createCustomer: %v", err)
		}
	}
	for _, f := range p.funding {
		if q != nil {
			if err := insertPayment(q, f.payment); err != nil {
				log.Printf("createCustomer: %v", err)
			}
		}
		// The movement carries the payment reference: it is the statement
		// line the customer sees (transactions.go).
		a := p.cust.Accounts[f.idx]
		if sim != nil && a.LedgerAccountID != "" {
			ds.recordSimMovementOn(sim, p.day, p.equityID, a.LedgerAccountID, f.payment.Amount, luca.CodeBookTransfer, f.payment.Reference)
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			log.Printf("createCustomer: commit: %v", err)
			_ = tx.Rollback()
		}
	}

}

// abandonPlanLocked takes a planned customer the database refused back off
// the books. Its sequence number and payment IDs stay used. Must be called
// with ds.mu held.
func (ds *DemoState) abandonPlanLocked(p customerPlan) {
	if p.epoch != ds.epoch {
		return
	}
	ds.nCustomers--
	for _, f := range p.funding {
		ds.addToBook(p.cust.Accounts[f.idx].Family, -f.payment.Amount)
	}
}

// persistCustomer writes a customer record and PII to the SQL customer store.
func persistCustomer(store *customers.SQLCustomerStore, cust *CustomerRecord, pii PIIInput) error {
	if store == nil {
		return nil
	}
	rec := customers.CustomerRecord{
		ID:            cust.ID,
		Ref:           cust.ID, // ref == id in demo (e.g. "cust-001")
		JoinDate:      cust.JoinDate,
		KYCVerified:   cust.KYCStatus.Verified,
		KYCLastCheck:  cust.KYCStatus.LastCheckDate,
		KYCRiskRating: cust.KYCStatus.RiskRating,
	}
	cpii := customers.PIIInput{
		Name:    pii.Name,
		NI:      pii.NI,
		DOB:     pii.DOB,
		Address: pii.Address,
		Email:   pii.Email,
		Phone:   pii.Phone,
	}
	return store.Create(context.Background(), rec, cpii)
}
