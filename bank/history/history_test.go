package history

import (
	"context"
	"database/sql"
	"testing"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	_ "git.bytestone.uk/hum3/go-postgres"
)

func open(t *testing.T) *History {
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
	return New(db)
}

func amount(n int) luca.Amount { return luca.Amount(n * 100) }

func day(n int) time.Time {
	return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n)
}

// A day's snapshot is written once: a second snapshot for the same day
// (a restart mid-day) leaves the morning's figures as they were.
func TestADayIsSnapshottedOnce(t *testing.T) {
	h := open(t)
	ctx := context.Background()
	first := Snapshot{Day: day(0), Savings: 100_00, Lending: 40_00, Customers: 3, NIMBps: 120.5, BoERate: 0.0525}
	if err := h.Save(ctx, first); err != nil {
		t.Fatal(err)
	}
	again := first
	again.Savings, again.Customers = 999_99, 99
	if err := h.Save(ctx, again); err != nil {
		t.Fatal(err)
	}
	latest, ok, err := h.Latest(ctx)
	if err != nil || !ok {
		t.Fatalf("Latest = %v, %v, %v", latest, ok, err)
	}
	if latest != first {
		t.Errorf("latest snapshot %+v, want the first write %+v", latest, first)
	}
}

// With nothing on record there is no latest day and no span.
func TestNoRecordNoLatest(t *testing.T) {
	h := open(t)
	ctx := context.Background()
	if _, ok, err := h.Latest(ctx); ok || err != nil {
		t.Errorf("Latest on an empty record: ok %v, err %v", ok, err)
	}
	if _, _, ok, err := h.Span(ctx); ok || err != nil {
		t.Errorf("Span on an empty record: ok %v, err %v", ok, err)
	}
	if s, err := h.Series(ctx); err != nil || len(s.Balances) != 0 {
		t.Errorf("Series on an empty record: %+v, %v", s, err)
	}
}

// The span is the opening day and the latest day on record, and the
// series run from the first day to the last in day order whatever order
// the snapshots were written in.
func TestSpanAndSeriesAreInDayOrder(t *testing.T) {
	h := open(t)
	ctx := context.Background()
	for _, n := range []int{2, 0, 1} {
		if err := h.Save(ctx, Snapshot{Day: day(n), Savings: amount(n), Customers: n, NIMBps: float64(n), BoERate: 0.05}); err != nil {
			t.Fatal(err)
		}
	}
	first, latest, ok, err := h.Span(ctx)
	if err != nil || !ok || !first.Equal(day(0)) || !latest.Equal(day(2)) {
		t.Errorf("Span = %s, %s, %v, %v; want %s to %s", first, latest, ok, err, day(0), day(2))
	}
	s, err := h.Series(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Balances) != 3 || len(s.Customers) != 3 || len(s.NIM) != 3 || len(s.BoERate) != 3 {
		t.Fatalf("series lengths %d %d %d %d, want 3 each", len(s.Balances), len(s.Customers), len(s.NIM), len(s.BoERate))
	}
	for i := range 3 {
		if !s.Balances[i].Date.Equal(day(i)) || s.Balances[i].Savings != amount(i) || s.Customers[i].Count != i || s.NIM[i].NIM != float64(i) || s.BoERate[i].Rate != 0.05 {
			t.Errorf("point %d = %+v %+v %+v %+v", i, s.Balances[i], s.Customers[i], s.NIM[i], s.BoERate[i])
		}
	}
}

// The series the charts draw are the newest MaxPoints days.
func TestSeriesKeepTheNewestPoints(t *testing.T) {
	h := open(t)
	ctx := context.Background()
	for n := range MaxPoints + 5 {
		if err := h.Save(ctx, Snapshot{Day: day(n)}); err != nil {
			t.Fatal(err)
		}
	}
	s, err := h.Series(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Balances) != MaxPoints {
		t.Fatalf("%d points, want %d", len(s.Balances), MaxPoints)
	}
	if !s.Balances[0].Date.Equal(day(5)) || !s.Balances[MaxPoints-1].Date.Equal(day(MaxPoints+4)) {
		t.Errorf("series run %s to %s, want the newest %d days", s.Balances[0].Date, s.Balances[MaxPoints-1].Date, MaxPoints)
	}
}

// The schema is the component's own: versions in order from 1.
func TestSchemaVersionsRunFromOne(t *testing.T) {
	if Schema.Name != "history" {
		t.Errorf("Schema.Name = %q", Schema.Name)
	}
	for i, m := range Schema.Migrations {
		if m.Version != i+1 {
			t.Errorf("migration %d listed where version %d is due", m.Version, i+1)
		}
	}
}
