package main

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
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
	CreatedAt time.Time
}

// PaymentSim manages a list of simulated payments.
type PaymentSim struct {
	mu       sync.Mutex
	payments []Payment
	nextID   int
	running  bool
	cancel   context.CancelFunc
}

func NewPaymentSim() *PaymentSim {
	return &PaymentSim{nextID: 1}
}

var (
	paymentNames = []string{
		"Alice", "Bob", "Charlie", "Diana", "Eve",
		"Frank", "Grace", "Henry", "Iris", "Jack",
	}
)

// SendPayment creates a random payment and starts its lifecycle.
func (ps *PaymentSim) SendPayment() {
	ps.mu.Lock()

	from := paymentNames[rand.Intn(len(paymentNames))]
	to := paymentNames[rand.Intn(len(paymentNames))]
	for to == from {
		to = paymentNames[rand.Intn(len(paymentNames))]
	}
	amount := float64(rand.Intn(99901)+100) / 100.0 // £1.00 to £999.99

	p := Payment{
		ID:        ps.nextID,
		From:      from,
		To:        to,
		Amount:    amount,
		Status:    PaymentPending,
		CreatedAt: time.Now(),
	}
	ps.nextID++
	ps.payments = append(ps.payments, p)
	idx := len(ps.payments) - 1
	ps.mu.Unlock()

	// Async status transitions
	go func() {
		time.Sleep(500 * time.Millisecond)
		ps.mu.Lock()
		if idx < len(ps.payments) {
			ps.payments[idx].Status = PaymentProcessing
		}
		ps.mu.Unlock()

		time.Sleep(1 * time.Second)
		ps.mu.Lock()
		if idx < len(ps.payments) {
			ps.payments[idx].Status = PaymentCompleted
		}
		ps.mu.Unlock()
	}()
}

// Start begins auto-generating payments at intervals.
func (ps *PaymentSim) Start() {
	ps.mu.Lock()
	if ps.running {
		ps.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ps.cancel = cancel
	ps.running = true
	ps.mu.Unlock()

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		// Send one immediately
		ps.SendPayment()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ps.SendPayment()
			}
		}
	}()
}

// Stop halts auto-generation.
func (ps *PaymentSim) Stop() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if !ps.running {
		return
	}
	ps.running = false
	if ps.cancel != nil {
		ps.cancel()
		ps.cancel = nil
	}
}

// IsRunning returns whether auto-generation is active.
func (ps *PaymentSim) IsRunning() bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.running
}

// Reset clears all payments and stops auto-generation.
func (ps *PaymentSim) Reset() {
	ps.Stop()
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.payments = nil
	ps.nextID = 1
}

// BuildHTML renders the payments list as a Bulma HTML table.
func (ps *PaymentSim) BuildHTML() string {
	ps.mu.Lock()
	payments := make([]Payment, len(ps.payments))
	copy(payments, ps.payments)
	running := ps.running
	ps.mu.Unlock()

	var s strings.Builder

	s.WriteString(`<h2 class="title is-4">Payments</h2>`)

	// Status
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
  <th>ID</th><th>From</th><th>To</th><th>Amount</th><th>Status</th><th>Time</th>
</tr></thead><tbody>`)

		// Show most recent first, limit to 20
		start := 0
		if len(payments) > 20 {
			start = len(payments) - 20
		}
		for i := len(payments) - 1; i >= start; i-- {
			p := payments[i]
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%d</td><td>%s</td><td>%s</td><td>£%.2f</td>
  <td><span class="tag %s">%s</span></td>
  <td>%s</td>
</tr>`, p.ID, p.From, p.To, p.Amount, p.Status.BulmaTag(), p.Status, p.CreatedAt.Format("15:04:05")))
		}

		s.WriteString(`</tbody></table></div>`)
	}

	return s.String()
}
