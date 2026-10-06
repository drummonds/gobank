package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
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
	equityID string
}

// customerFunding is the opening deposit or loan disbursement for one of the
// customer's accounts.
type customerFunding struct {
	idx     int // index into cust.Accounts
	payment Payment
}

// openCustomer is the core's OpenCustomer command (ADR-0002 stage 4): it
// checks the request, plans the customer under ds.mu, persists them in one
// transaction, and returns the ID the register gave them. Must not be
// called with ds.mu held.
func (ds *DemoState) openCustomer(c core.NewCustomer) (string, error) {
	if len(c.Accounts) == 0 {
		return "", core.ErrInvalidAmount
	}
	for _, a := range c.Accounts {
		if a.Opening < 0 {
			return "", core.ErrInvalidAmount
		}
		if _, ok := ds.productByID(a.ProductID); !ok {
			return "", core.ErrNotFound
		}
	}
	ds.mu.Lock()
	p := ds.planCustomerLocked(c)
	ds.mu.Unlock()
	if err := ds.persistCustomerPlan(p); err != nil {
		return "", err
	}
	return p.cust.ID, nil
}

// planCustomerLocked gives the customer the register's next ID, their
// accounts their bank details, and decides the funding: a deposit as
// asked, a loan within the bank's lending headroom, down to nothing. The
// customer goes on the books here. Must be called with ds.mu held.
func (ds *DemoState) planCustomerLocked(c core.NewCustomer) customerPlan {
	day := ds.currentDay
	lastCheck := c.KYC.LastCheck
	if lastCheck.IsZero() {
		lastCheck = day
	}
	cust := CustomerRecord{
		ID:        core.CustomerID(ds.nextCustSeq),
		JoinDate:  day,
		KYCStatus: KYCStatus{Verified: c.KYC.Verified, LastCheckDate: lastCheck, RiskRating: c.KYC.RiskRating},
	}
	ds.nextCustSeq++
	for _, a := range c.Accounts {
		product, _ := ds.productByID(a.ProductID) // checked by openCustomer
		cust.Accounts = append(cust.Accounts, CustomerAccount{
			ProductID:   product.ID,
			ProductName: product.Name,
			Family:      product.Family,
			Currency:    product.Currency,
			Rate:        product.Rate,
			OpenDate:    day,
			SortCode:    bankSortCode,
			AccountNum:  fmt.Sprintf("%08d", ds.rng.Intn(100000000)),
		})
	}
	p := customerPlan{
		epoch: ds.epoch, cust: cust, day: day,
		pii: PIIInput{Name: c.PII.Name, NI: c.PII.NI, DOB: c.PII.DOB, Address: c.PII.Address, Email: c.PII.Email, Phone: c.PII.Phone},
		db:  ds.db, ledger: ds.ledger, store: ds.custStore, equityID: ds.equityAccountID,
	}
	for i := range p.cust.Accounts {
		a := &p.cust.Accounts[i]
		amount := c.Accounts[i].Opening
		ptype, from := PayDeposit, "EXTERNAL"
		if a.Family == gbp.FamilyLending {
			amount = min(amount, ds.lendingHeadroom())
			ptype, from = PayLoanDisbursement, "BANK"
		}
		if amount <= 0 {
			continue
		}
		a.Balance = amount
		ds.addToBook(a.Family, amount)
		p.funding = append(p.funding, customerFunding{idx: i, payment: ds.newPaymentLocked(ptype, from, cust.ID, amount)})
	}
	ds.nCustomers++
	return p
}

const bankSortCode = "30-90-01"

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
// the books and the refusal returned; later failures are logged and the
// customer kept, as before the split. Must not be called with ds.mu held.
func (ds *DemoState) persistCustomerPlan(p customerPlan) error {
	store, ledger := p.store, p.ledger
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
		if ledger != nil {
			ledger = ledger.WithTx(tx)
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
		return fmt.Errorf("open customer %s: %w", p.cust.ID, err)
	}
	ds.addCustomerToLedger(ledger, &p.cust)
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
		if ledger != nil && a.LedgerAccountID != "" {
			ds.postEvent(ledger, p.day, p.equityID, a.LedgerAccountID, f.payment.Amount, luca.CodeBookTransfer, f.payment.Reference)
		}
	}
	// An account has a position from the day it opens, funded or not: the
	// day's rules run on it now, so the day's pass has nothing left to do
	// for it and a day is complete when every registered account has its
	// position. A new account has nothing applied, so nothing is booked.
	for _, a := range p.cust.Accounts {
		if ledger == nil || a.LedgerAccountID == "" {
			continue
		}
		product, _ := ds.productByID(a.ProductID)
		if _, err := ds.accountDay(ledger, dayAccount{id: a.LedgerAccountID, product: product}, p.day); err != nil {
			log.Printf("createCustomer: %s: %v", a.LedgerAccountID, err)
		}
	}
	if tx != nil {
		if err := tx.Commit(); err != nil {
			log.Printf("createCustomer: commit: %v", err)
			_ = tx.Rollback()
		}
	}
	return nil
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
