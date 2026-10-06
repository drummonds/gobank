package payments

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "git.bytestone.uk/hum3/go-postgres"
	"git.bytestone.uk/hum3/gobank/core"
)

var now = time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC)

func open(t *testing.T) (*Payments, *sql.DB) {
	t.Helper()
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, m := range Schema.Migrations {
		for _, stmt := range m.Statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("schema v%d: %v", m.Version, err)
			}
		}
	}
	p, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	return p, db
}

// A new payment takes the next number and its reference; inserted, it is
// on record by ID, on its customers' lists and on the newest-first page,
// and its lifecycle is recorded as it moves.
func TestPaymentsAreNumberedAndRecorded(t *testing.T) {
	p, _ := open(t)
	ctx := context.Background()
	dep := p.New(Deposit, "EXTERNAL", "cust-001", 1000_00, Completed, now)
	if dep.ID != 1 || dep.Reference != "PAY-000001" || !dep.SettledAt.Equal(now) {
		t.Errorf("first payment %+v; want number 1, PAY-000001, settled at creation", dep)
	}
	tr := p.New(Transfer, "cust-001", "cust-002", 50_00, Pending, now.Add(time.Minute))
	if tr.ID != 2 || !tr.SettledAt.IsZero() {
		t.Errorf("second payment %+v; want number 2, unsettled", tr)
	}
	for _, pay := range []Payment{dep, tr} {
		if err := p.Insert(p.db, pay); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := p.ByID(ctx, 2); err != nil || got.Reference != "PAY-000002" || got.Status != Pending {
		t.Errorf("ByID(2) = %+v, %v", got, err)
	}
	if _, err := p.ByID(ctx, 9); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("unknown payment: %v, want ErrNotFound", err)
	}
	if err := p.SetStatus(ctx, 2, Completed, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got, _ := p.ByID(ctx, 2); got.Status != Completed || !got.SettledAt.Equal(now.Add(2*time.Minute)) {
		t.Errorf("after settling: %+v", got)
	}
	page, total, err := p.Page(ctx, 1, 20)
	if err != nil || total != 2 || len(page) != 2 || page[0].ID != 2 {
		t.Errorf("Page = %d of %d, %v; want both, newest first", len(page), total, err)
	}
	if of, _ := p.Of(ctx, "cust-002"); len(of) != 1 || of[0].ID != 2 {
		t.Errorf("Of(cust-002) = %+v", of)
	}
	if of, _ := p.Of(ctx, "cust-001"); len(of) != 2 || of[0].ID != 1 {
		t.Errorf("Of(cust-001) = %+v; want both, oldest first", of)
	}
	c := tr.Core()
	if c.Type != core.PaymentTransfer || c.Status != core.PaymentPending || c.From != "cust-001" || c.Reference != "PAY-000002" {
		t.Errorf("core payment %+v", c)
	}
}

// Reopened over the same database the numbering continues; cleared, it
// starts again from one.
func TestNumberingResumesAndRestarts(t *testing.T) {
	p, db := open(t)
	ctx := context.Background()
	for range 3 {
		if err := p.Insert(p.db, p.New(Deposit, "EXTERNAL", "cust-001", 1, Completed, now)); err != nil {
			t.Fatal(err)
		}
	}
	q, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if next := q.New(Deposit, "EXTERNAL", "cust-001", 1, Completed, now); next.ID != 4 {
		t.Errorf("after reopening the next payment is %d, want 4", next.ID)
	}
	if err := q.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := q.Count(ctx); n != 0 {
		t.Errorf("%d payments after clearing", n)
	}
	if first := q.New(Deposit, "EXTERNAL", "cust-001", 1, Completed, now); first.ID != 1 {
		t.Errorf("after clearing the next payment is %d, want 1", first.ID)
	}
}

// The schema is the component's own: versions in order from 1.
func TestSchemaVersionsRunFromOne(t *testing.T) {
	if Schema.Name != "payments" {
		t.Errorf("Schema.Name = %q", Schema.Name)
	}
	for i, m := range Schema.Migrations {
		if m.Version != i+1 {
			t.Errorf("migration %d listed where version %d is due", m.Version, i+1)
		}
	}
}
