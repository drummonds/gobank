package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// twoFundedCustomers gives SendPayment a sender and a recipient.
func twoFundedCustomers(ds *DemoState) {
	addFundedCustomer(ds)
	addFundedCustomer(ds)
}

// Payments are read back from the database, not from a list held in
// memory: funding payments from customer creation and transfers alike.
func TestPaymentReadableFromDatabase(t *testing.T) {
	ds := NewDemoState()
	twoFundedCustomers(ds)
	funding := ds.paymentCount()
	if funding == 0 {
		t.Fatal("funding two customers produced no payments")
	}
	first, ok := ds.paymentByID(1)
	if !ok || first.Type != PayDeposit || first.Status != PaymentCompleted || first.SettledAt.IsZero() {
		t.Errorf("first funding payment = %+v, want a completed deposit", first)
	}

	ds.SendPayment()
	if n := ds.paymentCount(); n != funding+1 {
		t.Fatalf("paymentCount = %d after a transfer, want %d", n, funding+1)
	}
	page, total := ds.paymentPage(1)
	if total != funding+1 || len(page) != total {
		t.Fatalf("paymentPage(1) = %d of %d, want all %d", len(page), total, funding+1)
	}
	p := page[0] // newest first
	if p.Type != PayTransfer || p.ID != funding+1 {
		t.Fatalf("newest payment = %+v, want the transfer", p)
	}
	got, ok := ds.paymentByID(p.ID)
	if !ok {
		t.Fatalf("payment %d not found", p.ID)
	}
	if got.Reference != fmt.Sprintf("PAY-%06d", p.ID) || got.FromID == got.ToID ||
		!strings.HasPrefix(got.FromID, "cust-") || !strings.HasPrefix(got.ToID, "cust-") ||
		got.Amount < 100 || got.Status != PaymentPending || got.CreatedAt.IsZero() || !got.SettledAt.IsZero() {
		t.Errorf("transfer read back incomplete: %+v", got)
	}
	for _, id := range []string{got.FromID, got.ToID} {
		found := false
		for _, q := range ds.paymentsOf(id) {
			if q.ID == got.ID {
				found = true
			}
		}
		if !found {
			t.Errorf("paymentsOf(%s) omits payment %d", id, got.ID)
		}
	}
	if _, ok := ds.paymentByID(999999); ok {
		t.Error("unknown payment reported as found")
	}

	ds.ResetPayments()
	if n := ds.paymentCount(); n != 0 {
		t.Errorf("paymentCount = %d after reset, want 0", n)
	}
	ds.SendPayment()
	if p, ok := ds.paymentByID(1); !ok || p.Type != PayTransfer {
		t.Errorf("numbering does not restart after reset: %+v", p)
	}
}

// Status transitions are written to the payment's row.
func TestPaymentSettlementRecorded(t *testing.T) {
	ds := NewDemoState()
	twoFundedCustomers(ds)
	ds.SendPayment()
	page, _ := ds.paymentPage(1)
	id := page[0].ID

	ds.setPaymentStatus(id, PaymentProcessing, time.Time{})
	p, _ := ds.paymentByID(id)
	if p.Status != PaymentProcessing || !p.SettledAt.IsZero() {
		t.Errorf("after processing: %+v", p)
	}
	at := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	ds.setPaymentStatus(id, PaymentCompleted, at)
	p, _ = ds.paymentByID(id)
	if p.Status != PaymentCompleted || !p.SettledAt.Equal(at) {
		t.Errorf("after completion: %+v", p)
	}
}

// contract_payments is the view other code reads: it carries what the
// payments API reports, row for row.
func TestContractPaymentsView(t *testing.T) {
	ds := NewDemoState()
	twoFundedCustomers(ds)
	ds.SendPayment()
	rows, err := ds.db.Query(`SELECT id, reference, type, from_id, to_id, amount, status FROM contract_payments ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var got Payment
		if err := rows.Scan(&got.ID, &got.Reference, &got.Type, &got.FromID, &got.ToID, &got.Amount, &got.Status); err != nil {
			t.Fatal(err)
		}
		want, ok := ds.paymentByID(got.ID)
		if !ok {
			t.Fatalf("view row %d has no payment", got.ID)
		}
		want.CreatedAt, want.SettledAt = time.Time{}, time.Time{}
		if got != want {
			t.Errorf("view row %d = %+v, want %+v", got.ID, got, want)
		}
		n++
	}
	if n != ds.paymentCount() {
		t.Errorf("view has %d rows, paymentCount = %d", n, ds.paymentCount())
	}
}

// Payment times are full UTC datetimes, on the detail page and in every
// list: a bare time of day is ambiguous once the demo has run for more
// than a day (#22).
func TestPaymentTimesAreUTCDatetimes(t *testing.T) {
	ds := NewDemoState()
	twoFundedCustomers(ds)
	ds.SendPayment()
	q := newCoreAdapter(ds, "")
	page, err := q.PaymentPage(context.Background(), 1)
	if err != nil || len(page.Payments) == 0 {
		t.Fatalf("PaymentPage: %v, %d entries", err, len(page.Payments))
	}
	p := page.Payments[0]
	want := p.CreatedAt.UTC().Format("2006-01-02 15:04:05Z")
	if got := fmtUTC(p.CreatedAt); got != want {
		t.Errorf("fmtUTC = %q, want %q", got, want)
	}
	for what, html := range map[string]string{
		"detail": buildPaymentDetailHTML(q, p.ID, false),
		"list":   buildPaymentsHTML(q, false, 1, false),
	} {
		if !strings.Contains(html, want) {
			t.Errorf("%s page lacks the UTC datetime %q", what, want)
		}
	}
	// The customer report lists the customer's own payments.
	for _, p := range page.Payments {
		if p.From == "cust-001" || p.To == "cust-001" {
			if html := buildCustomerViewHTML(q, "cust-001", true); !strings.Contains(html, fmtUTC(p.CreatedAt)) {
				t.Errorf("report page lacks the UTC datetime %q of payment %d", fmtUTC(p.CreatedAt), p.ID)
			}
			break
		}
	}
}
