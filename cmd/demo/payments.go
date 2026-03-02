package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

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
	Amount    float64
	Status    PaymentStatus
	Reference string
	CreatedAt time.Time
	SettledAt time.Time
}

// SendPayment creates a random payment between customers, debiting sender's
// savings account and crediting recipient's savings account.
func (ds *DemoState) SendPayment() {
	ds.mu.Lock()

	if len(ds.customers) < 2 {
		ds.mu.Unlock()
		return
	}

	fromIdx := ds.rng.Intn(len(ds.customers))
	toIdx := ds.rng.Intn(len(ds.customers))
	for toIdx == fromIdx {
		toIdx = ds.rng.Intn(len(ds.customers))
	}

	// Find first savings account on each
	fromAccIdx := -1
	for i, a := range ds.customers[fromIdx].Accounts {
		if a.Family == FamilySavings {
			fromAccIdx = i
			break
		}
	}
	toAccIdx := -1
	for i, a := range ds.customers[toIdx].Accounts {
		if a.Family == FamilySavings {
			toAccIdx = i
			break
		}
	}
	if fromAccIdx < 0 || toAccIdx < 0 {
		ds.mu.Unlock()
		return
	}

	senderBal := ds.customers[fromIdx].Accounts[fromAccIdx].Balance
	amount := float64(ds.rng.Intn(99901)+100) / 100.0
	if amount > senderBal {
		amount = senderBal
	}
	if amount < 1.0 {
		ds.mu.Unlock()
		return
	}

	// Debit sender, credit recipient
	ds.customers[fromIdx].Accounts[fromAccIdx].Balance -= amount
	ds.customers[toIdx].Accounts[toAccIdx].Balance += amount

	fromID := ds.customers[fromIdx].ID
	toID := ds.customers[toIdx].ID
	ref := fmt.Sprintf("PAY-%06d", ds.nextPaymentID)

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
	ds.payments = append(ds.payments, p)

	idx := len(ds.payments) - 1
	ds.mu.Unlock()

	// Async status transitions
	go func() {
		time.Sleep(500 * time.Millisecond)
		ds.mu.Lock()
		if idx < len(ds.payments) && ds.payments[idx].ID == p.ID {
			ds.payments[idx].Status = PaymentProcessing
		}
		ds.mu.Unlock()

		time.Sleep(1 * time.Second)
		ds.mu.Lock()
		if idx < len(ds.payments) && ds.payments[idx].ID == p.ID {
			ds.payments[idx].Status = PaymentCompleted
			ds.payments[idx].SettledAt = time.Now()
		}
		ds.mu.Unlock()
	}()
}

// makePayment creates and records a payment, settling it immediately.
// Must be called with ds.mu held. Directly modifies the target account balance.
func (ds *DemoState) makePayment(ptype PaymentType, fromID, toID string, amount float64) {
	ref := fmt.Sprintf("PAY-%06d", ds.nextPaymentID)
	p := Payment{
		ID:        ds.nextPaymentID,
		Type:      ptype,
		FromID:    fromID,
		ToID:      toID,
		Amount:    amount,
		Status:    PaymentCompleted,
		Reference: ref,
		CreatedAt: time.Now(),
		SettledAt: time.Now(),
	}
	ds.nextPaymentID++
	ds.payments = append(ds.payments, p)
}

// fundCustomer creates deposit and loan disbursement payments for a newly
// created customer's accounts. Must be called with ds.mu held.
func (ds *DemoState) fundCustomer(custIdx int) {
	cust := &ds.customers[custIdx]
	for i := range cust.Accounts {
		a := &cust.Accounts[i]
		if a.Family == FamilySavings {
			amount := float64(500 + ds.rng.Intn(9500))
			a.Balance = amount
			ds.makePayment(PayDeposit, "EXTERNAL", cust.ID, amount)
		} else {
			headroom := ds.lendingHeadroom()
			if headroom <= 0 {
				continue
			}
			amount := float64(1000 + ds.rng.Intn(49000))
			if amount > headroom {
				amount = headroom
			}
			a.Balance = amount
			ds.makePayment(PayLoanDisbursement, "BANK", cust.ID, amount)
		}
	}
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
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		ds.SendPayment()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
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
	ds.payments = nil
	ds.nextPaymentID = 1
}

const paymentsPerPage = 20

// BuildPaymentsHTML renders the payments list as a Bulma HTML table.
// Shows customer IDs; names shown only when piiAuth is true.
func (ds *DemoState) BuildPaymentsHTML(piiAuth bool, page int) string {
	ds.mu.Lock()
	payments := make([]Payment, len(ds.payments))
	copy(payments, ds.payments)
	running := ds.payRunning
	piiStore := ds.piiStore
	ds.mu.Unlock()

	total := len(payments)
	if page < 1 {
		page = 1
	}
	totalPages := (total + paymentsPerPage - 1) / paymentsPerPage
	if totalPages < 1 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	// Reverse order (newest first)
	for i, j := 0, len(payments)-1; i < j; i, j = i+1, j-1 {
		payments[i], payments[j] = payments[j], payments[i]
	}

	start := (page - 1) * paymentsPerPage
	end := start + paymentsPerPage
	if end > total {
		end = total
	}
	pagePayments := payments[start:end]

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
				if name := piiStore.RetrieveName(p.FromID); name != "" {
					from = fmt.Sprintf("%s (%s)", p.FromID, name)
				}
				if name := piiStore.RetrieveName(p.ToID); name != "" {
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
	ds.mu.Lock()
	var found *Payment
	for i := range ds.payments {
		if ds.payments[i].ID == id {
			p := ds.payments[i]
			found = &p
			break
		}
	}
	piiStore := ds.piiStore
	ds.mu.Unlock()

	if found == nil {
		return `<div class="notification is-warning">Payment not found.</div>`
	}

	p := found
	from := p.FromID
	to := p.ToID
	if piiAuth {
		if name := piiStore.RetrieveName(p.FromID); name != "" {
			from = fmt.Sprintf("%s (%s)", p.FromID, name)
		}
		if name := piiStore.RetrieveName(p.ToID); name != "" {
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
