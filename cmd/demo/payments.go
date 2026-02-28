package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

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
	From      string
	To        string
	Amount    float64
	Status    PaymentStatus
	Reference string
	CreatedAt time.Time
	SettledAt time.Time
}

const maxPayments = 20

// SendPayment creates a random payment using actual customer names.
func (ds *DemoState) SendPayment() {
	ds.mu.Lock()

	names := make([]string, len(ds.customers))
	for i, c := range ds.customers {
		names[i] = c.Name
	}

	from := names[ds.rng.Intn(len(names))]
	to := names[ds.rng.Intn(len(names))]
	for to == from {
		to = names[ds.rng.Intn(len(names))]
	}
	amount := float64(ds.rng.Intn(99901)+100) / 100.0

	ref := fmt.Sprintf("PAY-%06d", ds.nextPaymentID)

	p := Payment{
		ID:        ds.nextPaymentID,
		From:      from,
		To:        to,
		Amount:    amount,
		Status:    PaymentPending,
		Reference: ref,
		CreatedAt: time.Now(),
	}
	ds.nextPaymentID++
	ds.payments = append(ds.payments, p)

	// Trim to max
	if len(ds.payments) > maxPayments {
		ds.payments = ds.payments[len(ds.payments)-maxPayments:]
	}

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

// BuildPaymentsHTML renders the payments list as a Bulma HTML table.
func (ds *DemoState) BuildPaymentsHTML() string {
	ds.mu.Lock()
	payments := make([]Payment, len(ds.payments))
	copy(payments, ds.payments)
	running := ds.payRunning
	ds.mu.Unlock()

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Payments</h2>`)

	statusTag := `<span class="tag is-light">Stopped</span>`
	if running {
		statusTag = `<span class="tag is-warning">Auto-sending</span>`
	}
	s.WriteString(fmt.Sprintf(`<div class="field is-grouped is-grouped-multiline mb-4">
  <div class="control">%s</div>
  <div class="control"><span class="tag is-info is-light">%d payments</span></div>
</div>`, statusTag, len(payments)))

	if len(payments) == 0 {
		s.WriteString(`<p class="has-text-grey">No payments yet. Send one or start auto-generation.</p>`)
	} else {
		s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped is-hoverable">
<thead><tr>
  <th>ID</th><th>From</th><th>To</th><th>Amount</th><th>Reference</th><th>Status</th><th>Time</th><th></th>
</tr></thead><tbody>`)

		for i := len(payments) - 1; i >= 0; i-- {
			p := payments[i]
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%d</td><td>%s</td><td>%s</td><td>£%.2f</td><td><code>%s</code></td>
  <td><span class="tag %s">%s</span></td>
  <td>%s</td>
  <td><a href="/payments/%d" class="button is-small is-link is-light">Detail</a></td>
</tr>`, p.ID, p.From, p.To, p.Amount, p.Reference, p.Status.BulmaTag(), p.Status, p.CreatedAt.Format("15:04:05"), p.ID))
		}

		s.WriteString(`</tbody></table></div>`)
	}

	return s.String()
}

// BuildPaymentDetailHTML renders a single payment detail with settlement timeline.
func (ds *DemoState) BuildPaymentDetailHTML(id int) string {
	ds.mu.Lock()
	var found *Payment
	for i := range ds.payments {
		if ds.payments[i].ID == id {
			p := ds.payments[i]
			found = &p
			break
		}
	}
	ds.mu.Unlock()

	if found == nil {
		return `<div class="notification is-warning">Payment not found.</div>`
	}

	p := found
	var s strings.Builder
	s.WriteString(fmt.Sprintf(`<h2 class="title is-4">Payment %s</h2>`, p.Reference))

	// Details box
	s.WriteString(`<div class="box">`)
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>From:</strong> %s</div>`, p.From))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>To:</strong> %s</div>`, p.To))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Amount:</strong> £%.2f</div>`, p.Amount))
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
		// Active line segment
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
