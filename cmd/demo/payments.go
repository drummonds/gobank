package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
)

func poundsToPence(pounds float64) luca.Amount {
	return luca.Amount(math.Round(pounds * 100))
}

// PaymentType distinguishes how money enters/exits the system.
type PaymentType int

const (
	PayTransfer         PaymentType = iota // customer-to-customer
	PayDeposit                             // external money in (to savings)
	PayLoanDisbursement                    // bank lends to customer (creates loan)
)

func (t PaymentType) String() string {
	switch t {
	case PayTransfer:
		return "Transfer"
	case PayDeposit:
		return "Deposit"
	case PayLoanDisbursement:
		return "Loan"
	default:
		return "Unknown"
	}
}

func (t PaymentType) BulmaTag() string {
	switch t {
	case PayTransfer:
		return "is-link is-light"
	case PayDeposit:
		return "is-success is-light"
	case PayLoanDisbursement:
		return "is-info is-light"
	default:
		return "is-light"
	}
}

// PaymentStatus represents the lifecycle of a payment.
type PaymentStatus int

const (
	PaymentPending PaymentStatus = iota
	PaymentProcessing
	PaymentCompleted
)

func (s PaymentStatus) String() string {
	switch s {
	case PaymentPending:
		return "Pending"
	case PaymentProcessing:
		return "Processing"
	case PaymentCompleted:
		return "Completed"
	default:
		return "Unknown"
	}
}

func (s PaymentStatus) BulmaTag() string {
	switch s {
	case PaymentPending:
		return "is-warning"
	case PaymentProcessing:
		return "is-info"
	case PaymentCompleted:
		return "is-success"
	default:
		return "is-light"
	}
}

// Payment represents a single payment transaction.
type Payment struct {
	ID        int
	Type      PaymentType
	FromID    string
	ToID      string
	Amount    luca.Amount
	Status    PaymentStatus
	Reference string
	CreatedAt time.Time
	SettledAt time.Time
}

// The payments component owns the payments table. Other code reads
// payments through the contract_payments view or the API below (ADR-0001).

func (ds *DemoState) createPaymentsTable() {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS payments (
			id INTEGER PRIMARY KEY,
			type SMALLINT NOT NULL,
			from_id VARCHAR(20) NOT NULL,
			to_id VARCHAR(20) NOT NULL,
			amount BIGINT NOT NULL,
			status SMALLINT NOT NULL,
			reference VARCHAR(20) NOT NULL UNIQUE,
			created_at TIMESTAMP NOT NULL,
			settled_at TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS payments_from_id ON payments (from_id)`,
		`CREATE INDEX IF NOT EXISTS payments_to_id ON payments (to_id)`,
		// The contract view: what other components may read. Recreated so
		// a durable database picks up a changed definition.
		`DROP VIEW IF EXISTS contract_payments`,
		`CREATE VIEW contract_payments AS
			SELECT id, reference, type, from_id, to_id, amount, status, created_at, settled_at FROM payments`,
	}
	for _, stmt := range stmts {
		if _, err := ds.db.Exec(stmt); err != nil {
			log.Printf("initDB: payments: %v", err)
		}
	}
}

// insertPayment writes a payment row. q is the database or the transaction
// the payment shares with the customer it funds; nil (no database) drops
// the payment.
func insertPayment(q execer, p Payment) error {
	if q == nil {
		return nil
	}
	_, err := q.Exec(`INSERT INTO payments (id, type, from_id, to_id, amount, status, reference, created_at, settled_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		p.ID, int(p.Type), p.FromID, p.ToID, p.Amount, int(p.Status), p.Reference, p.CreatedAt.UTC(), nullTime(p.SettledAt))
	if err != nil {
		return fmt.Errorf("insert payment %s: %w", p.Reference, err)
	}
	return nil
}

func nullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t.UTC(), Valid: !t.IsZero()}
}

// setPaymentStatus records a lifecycle transition on the payment's row;
// settledAt is written when non-zero.
func (ds *DemoState) setPaymentStatus(id int, status PaymentStatus, settledAt time.Time) {
	if ds.db == nil {
		return
	}
	_, err := ds.db.Exec(`UPDATE payments SET status = $1, settled_at = COALESCE($2, settled_at) WHERE id = $3`,
		int(status), nullTime(settledAt), id)
	if err != nil {
		log.Printf("setPaymentStatus %d: %v", id, err)
	}
}

const paymentColumns = `id, type, from_id, to_id, amount, status, reference, created_at, settled_at`

func scanPayments(rows *sql.Rows) []Payment {
	defer rows.Close()
	var out []Payment
	for rows.Next() {
		var p Payment
		var settled sql.NullTime
		if err := rows.Scan(&p.ID, &p.Type, &p.FromID, &p.ToID, &p.Amount, &p.Status, &p.Reference, &p.CreatedAt, &settled); err != nil {
			log.Printf("scan payment: %v", err)
			return out
		}
		if settled.Valid {
			p.SettledAt = settled.Time
		}
		out = append(out, p)
	}
	return out
}

// paymentByID reads one payment.
func (ds *DemoState) paymentByID(id int) (Payment, bool) {
	if ds.db == nil {
		return Payment{}, false
	}
	rows, err := ds.db.Query(`SELECT `+paymentColumns+` FROM payments WHERE id = $1`, id)
	if err != nil {
		log.Printf("paymentByID %d: %v", id, err)
		return Payment{}, false
	}
	ps := scanPayments(rows)
	if len(ps) == 0 {
		return Payment{}, false
	}
	return ps[0], true
}

// paymentPage returns one page of payments, newest first, and the total.
func (ds *DemoState) paymentPage(page int) ([]Payment, int) {
	if ds.db == nil {
		return nil, 0
	}
	if page < 1 {
		page = 1
	}
	rows, err := ds.db.Query(`SELECT `+paymentColumns+` FROM payments ORDER BY id DESC LIMIT $1 OFFSET $2`,
		paymentsPerPage, (page-1)*paymentsPerPage)
	if err != nil {
		log.Printf("paymentPage %d: %v", page, err)
		return nil, 0
	}
	return scanPayments(rows), ds.paymentCount()
}

// paymentsOf lists every payment a customer sent or received, oldest first.
func (ds *DemoState) paymentsOf(customerID string) []Payment {
	if ds.db == nil {
		return nil
	}
	rows, err := ds.db.Query(`SELECT `+paymentColumns+` FROM payments WHERE from_id = $1 OR to_id = $1 ORDER BY id`, customerID)
	if err != nil {
		log.Printf("paymentsOf %s: %v", customerID, err)
		return nil
	}
	return scanPayments(rows)
}

// paymentCount is the number of payments on record.
func (ds *DemoState) paymentCount() int {
	if ds.db == nil {
		return 0
	}
	var n int
	if err := ds.db.QueryRow(`SELECT COUNT(*) FROM payments`).Scan(&n); err != nil {
		log.Printf("paymentCount: %v", err)
	}
	return n
}

// clearPaymentsLocked removes every payment and restarts the numbering.
// Must be called with ds.mu held.
func (ds *DemoState) clearPaymentsLocked() {
	if ds.db != nil {
		if _, err := ds.db.Exec(`DELETE FROM payments`); err != nil {
			log.Printf("clearPayments: %v", err)
		}
	}
	ds.nextPaymentID = 1
}

// SendPayment creates a random payment between customers, debiting sender's
// savings account and crediting recipient's savings account.
func (ds *DemoState) SendPayment() {
	ds.mu.Lock()

	if ds.nCustomers < 2 {
		ds.mu.Unlock()
		return
	}

	fromID := ds.randomCustomerID()
	toID := ds.randomCustomerID()
	for toID == fromID {
		toID = ds.randomCustomerID()
	}
	from, ok := ds.customerByID(fromID)
	to, ok2 := ds.customerByID(toID)
	if !ok || !ok2 {
		ds.mu.Unlock()
		return
	}

	// Pay from and into each customer's first savings account.
	fromAcc, toAcc := firstSavingsAccount(from.Accounts), firstSavingsAccount(to.Accounts)
	if fromAcc == nil || toAcc == nil {
		ds.mu.Unlock()
		return
	}

	senderBal := fromAcc.Balance
	amount := min(
		// 100..100000 pence
		luca.Amount(ds.rng.Intn(99901)+100), senderBal)
	if amount < 100 {
		ds.mu.Unlock()
		return
	}

	ref := fmt.Sprintf("PAY-%06d", ds.nextPaymentID)
	ds.recordSimMovement(fromAcc.LedgerAccountID, toAcc.LedgerAccountID, amount, luca.CodeBookTransfer, ref)

	ds.emitTx(ds.currentDay, fromID, fromAcc.LedgerAccountID, fromAcc.ProductName,
		TxTransferOut, amount, fromAcc.Balance-amount, ref)
	ds.emitTx(ds.currentDay, toID, toAcc.LedgerAccountID, toAcc.ProductName,
		TxTransferIn, amount, toAcc.Balance+amount, ref)

	p := Payment{
		ID:        ds.nextPaymentID,
		Type:      PayTransfer,
		FromID:    fromID,
		ToID:      toID,
		Amount:    amount,
		Status:    PaymentPending,
		Reference: ref,
		CreatedAt: time.Now(),
	}
	ds.nextPaymentID++
	var q execer
	if ds.db != nil {
		q = ds.db
	}
	if err := insertPayment(q, p); err != nil {
		log.Printf("SendPayment: %v", err)
	}
	ds.mu.Unlock()

	// Async status transitions
	go func() {
		time.Sleep(500 * time.Millisecond)
		ds.setPaymentStatus(p.ID, PaymentProcessing, time.Time{})
		time.Sleep(1 * time.Second)
		ds.setPaymentStatus(p.ID, PaymentCompleted, time.Now())
	}()
}

// randomCustomerID picks a customer ID uniformly from those generated so
// far. The ID may be missing (its persist failed); callers skip those.
// Must be called with ds.mu held.
func (ds *DemoState) randomCustomerID() string {
	return fmt.Sprintf("cust-%03d", 1+ds.rng.Intn(ds.nextCustSeq-1))
}

// firstSavingsAccount returns the customer's first savings account, or nil.
func firstSavingsAccount(accounts []CustomerAccount) *CustomerAccount {
	for i := range accounts {
		if accounts[i].Family == gbp.FamilySavings {
			return &accounts[i]
		}
	}
	return nil
}

// StartPayments begins auto-generating payments.
func (ds *DemoState) StartPayments() {
	ds.mu.Lock()
	if ds.payRunning {
		ds.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ds.payCancel = cancel
	ds.payRunning = true
	ds.mu.Unlock()

	go func() {
		// Self-pace (wait after completion) rather than a fixed-rate ticker —
		// see Start(): in WASM a ticker that can't keep up starves the JS
		// event loop and freezes the page.
		ds.SendPayment()
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
				ds.SendPayment()
			}
		}
	}()
}

// StopPayments halts auto-generation.
func (ds *DemoState) StopPayments() {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if !ds.payRunning {
		return
	}
	ds.payRunning = false
	if ds.payCancel != nil {
		ds.payCancel()
		ds.payCancel = nil
	}
}

// IsPaymentsRunning returns whether auto-generation is active.
func (ds *DemoState) IsPaymentsRunning() bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.payRunning
}

// ResetPayments clears all payments and stops auto-generation.
func (ds *DemoState) ResetPayments() {
	ds.StopPayments()
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.clearPaymentsLocked()
}

const paymentsPerPage = 20

// BuildPaymentsHTML renders the payments list as a Bulma HTML table.
// Shows customer IDs; names shown only when piiAuth is true.
func (ds *DemoState) BuildPaymentsHTML(piiAuth bool, page int) string {
	running := ds.IsPaymentsRunning()
	pagePayments, total := ds.paymentPage(page)
	if page < 1 {
		page = 1
	}
	totalPages := max((total+paymentsPerPage-1)/paymentsPerPage, 1)
	if page > totalPages {
		page = totalPages
		pagePayments, _ = ds.paymentPage(page)
	}

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Payments</h2>`)

	statusTag := `<span class="tag is-light">Stopped</span>`
	if running {
		statusTag = `<span class="tag is-warning">Auto-sending</span>`
	}
	s.WriteString(fmt.Sprintf(`<div class="field is-grouped is-grouped-multiline mb-4">
  <div class="control">%s</div>
  <div class="control"><span class="tag is-info is-light">%d payments</span></div>
</div>`, statusTag, total))

	if total == 0 {
		s.WriteString(`<p class="has-text-grey">No payments yet. Send one or start auto-generation.</p>`)
	} else {
		s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped is-hoverable">
<thead><tr>
  <th>ID</th><th>Type</th><th>From</th><th>To</th><th>Amount</th><th>Reference</th><th>Status</th><th>Time</th><th></th>
</tr></thead><tbody>`)

		for _, p := range pagePayments {
			from := p.FromID
			to := p.ToID
			if piiAuth {
				if name := ds.lookupName(p.FromID); name != p.FromID {
					from = fmt.Sprintf("%s (%s)", p.FromID, name)
				}
				if name := ds.lookupName(p.ToID); name != p.ToID {
					to = fmt.Sprintf("%s (%s)", p.ToID, name)
				}
			}
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%d</td><td><span class="tag %s">%s</span></td><td>%s</td><td>%s</td><td>%s</td><td><code>%s</code></td>
  <td><span class="tag %s">%s</span></td>
  <td>%s</td>
  <td><a href="/payments/%d" class="button is-small is-link is-light">Detail</a></td>
</tr>`, p.ID, p.Type.BulmaTag(), p.Type, from, to, fmtMoney(p.Amount), p.Reference, p.Status.BulmaTag(), p.Status, p.CreatedAt.Format("15:04:05"), p.ID))
		}

		s.WriteString(`</tbody></table></div>`)

		// Pagination
		if totalPages > 1 {
			s.WriteString(`<nav class="pagination is-small mt-4" role="navigation">`)
			if page > 1 {
				s.WriteString(fmt.Sprintf(`<a class="pagination-previous" href="/payments?page=%d">Previous</a>`, page-1))
			} else {
				s.WriteString(`<a class="pagination-previous" disabled>Previous</a>`)
			}
			if page < totalPages {
				s.WriteString(fmt.Sprintf(`<a class="pagination-next" href="/payments?page=%d">Next</a>`, page+1))
			} else {
				s.WriteString(`<a class="pagination-next" disabled>Next</a>`)
			}
			s.WriteString(fmt.Sprintf(`<span class="pagination-list">Page %d of %d</span>`, page, totalPages))
			s.WriteString(`</nav>`)
		}
	}

	return s.String()
}

// BuildPaymentDetailHTML renders a single payment detail with settlement timeline.
// Shows customer IDs; names shown only when piiAuth is true.
func (ds *DemoState) BuildPaymentDetailHTML(id int, piiAuth bool) string {
	found, ok := ds.paymentByID(id)
	if !ok {
		return `<div class="notification is-warning">Payment not found.</div>`
	}

	p := &found
	from := p.FromID
	to := p.ToID
	if piiAuth {
		if name := ds.lookupName(p.FromID); name != p.FromID {
			from = fmt.Sprintf("%s (%s)", p.FromID, name)
		}
		if name := ds.lookupName(p.ToID); name != p.ToID {
			to = fmt.Sprintf("%s (%s)", p.ToID, name)
		}
	}

	var s strings.Builder
	s.WriteString(fmt.Sprintf(`<h2 class="title is-4">Payment %s</h2>`, p.Reference))

	// Details box
	s.WriteString(`<div class="box">`)
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Type:</strong> <span class="tag %s">%s</span></div>`, p.Type.BulmaTag(), p.Type))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>From:</strong> %s</div>`, from))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>To:</strong> %s</div>`, to))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Amount:</strong> %s</div>`, fmtMoney(p.Amount)))
	s.WriteString(`</div>`)
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Status:</strong> <span class="tag %s">%s</span></div>`, p.Status.BulmaTag(), p.Status))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Created:</strong> %s</div>`, p.CreatedAt.Format("15:04:05")))
	settled := "—"
	if !p.SettledAt.IsZero() {
		settled = p.SettledAt.Format("15:04:05")
	}
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Settled:</strong> %s</div>`, settled))
	s.WriteString(`</div></div>`)

	// Settlement timeline SVG
	s.WriteString(buildTimelineSVG(p))

	return s.String()
}

func buildTimelineSVG(p *Payment) string {
	var s strings.Builder

	s.WriteString(`<div class="box mt-4"><h3 class="title is-5">Settlement Timeline</h3>`)
	s.WriteString(`<svg viewBox="0 0 500 80" xmlns="http://www.w3.org/2000/svg" style="max-width:500px;width:100%;height:auto">`)
	s.WriteString(`<style>text{font-family:Arial,Helvetica,sans-serif}</style>`)

	// Line
	s.WriteString(`<line x1="50" y1="30" x2="450" y2="30" stroke="#dbdbdb" stroke-width="3"/>`)

	steps := []struct {
		X     int
		Label string
		Time  string
		Done  bool
	}{
		{50, "Pending", p.CreatedAt.Format("15:04:05"), p.Status >= PaymentPending},
		{250, "Processing", "", p.Status >= PaymentProcessing},
		{450, "Completed", "", p.Status >= PaymentCompleted},
	}

	if !p.SettledAt.IsZero() {
		steps[2].Time = p.SettledAt.Format("15:04:05")
	}

	for _, st := range steps {
		color := "#dbdbdb"
		if st.Done {
			color = "#48c78e"
		}
		if st.Done && st.X > 50 {
			prevX := 50
			if st.X == 450 {
				prevX = 250
			}
			s.WriteString(fmt.Sprintf(`<line x1="%d" y1="30" x2="%d" y2="30" stroke="#48c78e" stroke-width="3"/>`, prevX, st.X))
		}
		s.WriteString(fmt.Sprintf(`<circle cx="%d" cy="30" r="10" fill="%s"/>`, st.X, color))
		if st.Done {
			s.WriteString(fmt.Sprintf(`<text x="%d" y="34" text-anchor="middle" font-size="11" fill="#fff" font-weight="bold">&#10003;</text>`, st.X))
		}
		s.WriteString(fmt.Sprintf(`<text x="%d" y="60" text-anchor="middle" font-size="11" fill="#363636">%s</text>`, st.X, st.Label))
		if st.Time != "" {
			s.WriteString(fmt.Sprintf(`<text x="%d" y="74" text-anchor="middle" font-size="9" fill="#7a7a7a">%s</text>`, st.X, st.Time))
		}
	}

	s.WriteString(`</svg></div>`)
	return s.String()
}
