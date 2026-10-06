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
	"git.bytestone.uk/hum3/gobank/core"
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

var paymentsSchema = componentSchema{Name: "payments", Migrations: []migration{
	{Version: 1, Statements: []string{
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
	}},
}}

// createPaymentsView (re)creates the contract view: what other components
// may read. Recreated at every start so a durable database picks up a
// changed definition.
func (ds *DemoState) createPaymentsView() {
	stmts := []string{
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

// lastPaymentID is the highest payment ID on record, zero when there are
// none: a resumed run numbers its next payment after it.
func lastPaymentID(db *sql.DB) int {
	var id int
	if err := db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM payments`).Scan(&id); err != nil {
		log.Printf("lastPaymentID: %v", err)
	}
	return id
}

// transfer is the core's Transfer command (ADR-0002 stage 1): it moves
// amount between the two customers' first savings accounts, records the
// movement in the ledger and the engine, the customer-facing entries, and
// the payment, whose status then settles asynchronously.
func (ds *DemoState) transfer(t core.Transfer) (Payment, error) {
	if t.Amount <= 0 {
		return Payment{}, core.ErrInvalidAmount
	}
	if t.From == t.To {
		return Payment{}, core.ErrSameCustomer
	}
	ds.mu.Lock()
	defer ds.mu.Unlock()

	from, ok := ds.customerByID(t.From)
	to, ok2 := ds.customerByID(t.To)
	if !ok || !ok2 {
		return Payment{}, core.ErrNotFound
	}
	fromAcc, toAcc := firstSavingsAccount(from.Accounts), firstSavingsAccount(to.Accounts)
	if fromAcc == nil || toAcc == nil {
		return Payment{}, core.ErrNotFound
	}
	if fromAcc.Balance < t.Amount {
		return Payment{}, core.ErrInsufficientFunds
	}

	ref := fmt.Sprintf("PAY-%06d", ds.nextPaymentID)
	if ds.ledger != nil {
		// The two accounts' positions are rewritten: take their locks so
		// the daily pass and this event take turns on each of them.
		unlock := ds.ledger.Lock(fromAcc.LedgerAccountID, toAcc.LedgerAccountID)
		defer unlock()
		fromProduct, _ := ds.productByID(fromAcc.ProductID)
		toProduct, _ := ds.productByID(toAcc.ProductID)
		for _, r := range ds.postEvent(ds.ledger, ds.currentDay, fromAcc.LedgerAccountID, toAcc.LedgerAccountID, t.Amount, luca.CodeBookTransfer, ref,
			dayAccount{id: fromAcc.LedgerAccountID, product: fromProduct}, dayAccount{id: toAcc.LedgerAccountID, product: toProduct}) {
			ds.bookResultLocked(r)
		}
	}

	p := Payment{
		ID:        ds.nextPaymentID,
		Type:      PayTransfer,
		FromID:    t.From,
		ToID:      t.To,
		Amount:    t.Amount,
		Status:    PaymentPending,
		Reference: ref,
		CreatedAt: ds.clock.Now(),
	}
	ds.nextPaymentID++
	var q execer
	if ds.db != nil {
		q = ds.db
	}
	if err := insertPayment(q, p); err != nil {
		log.Printf("transfer: %v", err)
	}

	// Async status transitions
	go func() {
		time.Sleep(500 * time.Millisecond)
		ds.setPaymentStatus(p.ID, PaymentProcessing, time.Time{})
		time.Sleep(1 * time.Second)
		ds.setPaymentStatus(p.ID, PaymentCompleted, ds.clock.Now())
	}()
	return p, nil
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

// ResetPayments clears all payments and stops auto-generation.
func (ds *DemoState) ResetPayments() {
	ds.StopPayments()
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.clearPaymentsLocked()
}

const paymentsPerPage = 20

// paymentTypeTag is the Bulma tag class for a payment type.
func paymentTypeTag(t core.PaymentType) string {
	switch t {
	case core.PaymentTransfer:
		return "is-link is-light"
	case core.PaymentDeposit:
		return "is-success is-light"
	case core.PaymentLoan:
		return "is-info is-light"
	default:
		return "is-light"
	}
}

// paymentStatusTag is the Bulma tag class for a payment status.
func paymentStatusTag(s core.PaymentStatus) string {
	switch s {
	case core.PaymentPending:
		return "is-warning"
	case core.PaymentProcessing:
		return "is-info"
	case core.PaymentCompleted:
		return "is-success"
	default:
		return "is-light"
	}
}

// paymentStatusReached reports whether a payment has reached a status in
// its lifecycle: pending, then processing, then completed.
func paymentStatusReached(have, want core.PaymentStatus) bool {
	rank := map[core.PaymentStatus]int{core.PaymentPending: 0, core.PaymentProcessing: 1, core.PaymentCompleted: 2}
	return rank[have] >= rank[want]
}

// partyLabel shows a customer ID, with the name when PII is authorised.
func partyLabel(q core.CustomerRegister, id string, piiAuth bool) string {
	if piiAuth {
		if name, err := q.CustomerName(context.Background(), id); err == nil && name != id {
			return fmt.Sprintf("%s (%s)", id, name)
		}
	}
	return id
}

// buildPaymentsHTML renders the payments list as a Bulma HTML table.
// Shows customer IDs; names shown only when piiAuth is true. running is
// the generator's state, shown as a tag.
func buildPaymentsHTML(q core.StaffQueries, piiAuth bool, page int, running bool) string {
	ctx := context.Background()
	pp, _ := q.PaymentPage(ctx, page)
	if pp.Page > pp.TotalPages() {
		pp, _ = q.PaymentPage(ctx, pp.TotalPages())
	}
	page, totalPages, total := pp.Page, pp.TotalPages(), pp.Total

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

		for _, p := range pp.Payments {
			from := partyLabel(q, p.From, piiAuth)
			to := partyLabel(q, p.To, piiAuth)
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%d</td><td><span class="tag %s">%s</span></td><td>%s</td><td>%s</td><td>%s</td><td><code>%s</code></td>
  <td><span class="tag %s">%s</span></td>
  <td>%s</td>
  <td><a href="/payments/%d" class="button is-small is-link is-light">Detail</a></td>
</tr>`, p.ID, paymentTypeTag(p.Type), p.Type, from, to, fmtMoney(p.Amount), p.Reference, paymentStatusTag(p.Status), p.Status, fmtUTC(p.CreatedAt), p.ID))
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

// buildPaymentDetailHTML renders a single payment detail with settlement timeline.
// Shows customer IDs; names shown only when piiAuth is true.
func buildPaymentDetailHTML(q core.StaffQueries, id int, piiAuth bool) string {
	p, err := q.Payment(context.Background(), id)
	if err != nil {
		return `<div class="notification is-warning">Payment not found.</div>`
	}
	from := partyLabel(q, p.From, piiAuth)
	to := partyLabel(q, p.To, piiAuth)

	var s strings.Builder
	s.WriteString(fmt.Sprintf(`<h2 class="title is-4">Payment %s</h2>`, p.Reference))

	// Details box
	s.WriteString(`<div class="box">`)
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Type:</strong> <span class="tag %s">%s</span></div>`, paymentTypeTag(p.Type), p.Type))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>From:</strong> %s</div>`, from))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>To:</strong> %s</div>`, to))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Amount:</strong> %s</div>`, fmtMoney(p.Amount)))
	s.WriteString(`</div>`)
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Status:</strong> <span class="tag %s">%s</span></div>`, paymentStatusTag(p.Status), p.Status))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Created:</strong> %s</div>`, fmtUTC(p.CreatedAt)))
	settled := "—"
	if !p.SettledAt.IsZero() {
		settled = fmtUTC(p.SettledAt)
	}
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Settled:</strong> %s</div>`, settled))
	s.WriteString(`</div></div>`)

	// Settlement timeline SVG
	s.WriteString(buildTimelineSVG(p))

	return s.String()
}

func buildTimelineSVG(p core.Payment) string {
	var s strings.Builder

	s.WriteString(`<div class="box mt-4"><h3 class="title is-5">Settlement Timeline</h3>`)
	s.WriteString(`<svg viewBox="0 0 500 92" xmlns="http://www.w3.org/2000/svg" style="max-width:500px;width:100%;height:auto">`)
	s.WriteString(`<style>text{font-family:Arial,Helvetica,sans-serif}</style>`)

	// Line
	s.WriteString(`<line x1="50" y1="30" x2="450" y2="30" stroke="#dbdbdb" stroke-width="3"/>`)

	steps := []struct {
		X     int
		Label string
		Time  string
		Done  bool
	}{
		{50, "Pending", fmtUTC(p.CreatedAt), paymentStatusReached(p.Status, core.PaymentPending)},
		{250, "Processing", "", paymentStatusReached(p.Status, core.PaymentProcessing)},
		{450, "Completed", "", paymentStatusReached(p.Status, core.PaymentCompleted)},
	}

	if !p.SettledAt.IsZero() {
		steps[2].Time = fmtUTC(p.SettledAt)
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
		if st.Time != "" { // the UTC datetime, date over time
			date, clock, _ := strings.Cut(st.Time, " ")
			s.WriteString(fmt.Sprintf(`<text x="%d" y="74" text-anchor="middle" font-size="9" fill="#7a7a7a">%s</text>`, st.X, date))
			s.WriteString(fmt.Sprintf(`<text x="%d" y="86" text-anchor="middle" font-size="9" fill="#7a7a7a">%s</text>`, st.X, clock))
		}
	}

	s.WriteString(`</svg></div>`)
	return s.String()
}
