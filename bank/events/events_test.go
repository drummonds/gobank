package events

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "git.bytestone.uk/hum3/go-postgres"
)

var noon = time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)

func open(t *testing.T) *Events {
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
	e := Open(db)
	tick := 0
	e.now = func() time.Time { tick++; return noon.Add(time.Duration(tick) * time.Minute) }
	return e
}

// A root is raised pending at its instant; it is due once the clock
// reaches the instant and until it is done.
func TestARootIsDueFromItsInstantUntilDone(t *testing.T) {
	e := open(t)
	ctx := context.Background()
	root, err := e.Raise(ctx, nil, Publication, noon, "Bank of England")
	if err != nil {
		t.Fatal(err)
	}
	if root.ID == "" || root.Kind != Publication || !root.At.Equal(noon) || root.Status != Pending || root.RaisedBy != "Bank of England" || root.ParentID != "" || root.RootID != root.ID || !root.DoneAt.IsZero() {
		t.Errorf("raised %+v; want a pending root of its own at noon", root)
	}
	if due, _ := e.Due(ctx, nil, noon.Add(-time.Second)); len(due) != 0 {
		t.Errorf("due before its instant: %+v", due)
	}
	if n, _ := e.CountDue(ctx, noon.Add(-time.Second)); n != 0 {
		t.Errorf("counted due before its instant: %d", n)
	}
	due, err := e.Due(ctx, nil, noon)
	if err != nil || len(due) != 1 || due[0].ID != root.ID {
		t.Fatalf("due at its instant = %+v, %v; want the root", due, err)
	}
	if n, _ := e.CountDue(ctx, noon.Add(time.Hour)); n != 1 {
		t.Errorf("counted due after its instant: %d, want 1", n)
	}
	if err := e.Done(ctx, nil, root.ID); err != nil {
		t.Fatal(err)
	}
	if due, _ := e.Due(ctx, nil, noon.Add(time.Hour)); len(due) != 0 {
		t.Errorf("still due once done: %+v", due)
	}
	got, ok, err := e.ByID(ctx, root.ID)
	if err != nil || !ok || got.Status != Done || got.DoneAt.IsZero() {
		t.Errorf("ByID after Done = %+v, %v, %v; want done at a wall instant", got, ok, err)
	}
	if _, ok, _ := e.ByID(ctx, "no-such"); ok {
		t.Error("an unknown ID is found")
	}
}

// Due roots come in instant order; the recent ones newest first.
func TestDueIsInInstantOrderAndRecentNewestFirst(t *testing.T) {
	e := open(t)
	ctx := context.Background()
	later, _ := e.Raise(ctx, nil, Setting, noon.Add(2*time.Hour), "alice")
	first, _ := e.Raise(ctx, nil, StartOfDay, noon.Add(-12*time.Hour), "clock")
	middle, _ := e.Raise(ctx, nil, Payment, noon, "cust-001")
	due, _ := e.Due(ctx, nil, noon.Add(3*time.Hour))
	if len(due) != 3 || due[0].ID != first.ID || due[1].ID != middle.ID || due[2].ID != later.ID {
		t.Errorf("due = %+v; want the day, the payment, the setting", due)
	}
	recent, err := e.Recent(ctx, 2)
	if err != nil || len(recent) != 2 || recent[0].ID != later.ID || recent[1].ID != middle.ID {
		t.Errorf("Recent(2) = %+v, %v; want the setting then the payment", recent, err)
	}
	// Two at one instant: the one raised later first.
	another, _ := e.Raise(ctx, nil, Opening, noon, "cust-002")
	if recent, _ := e.Recent(ctx, 3); recent[1].ID != another.ID || recent[2].ID != middle.ID {
		t.Errorf("Recent(3) = %+v; want the later-raised opening before the payment at the same instant", recent)
	}
}

// A root at a kind and instant is raised once: the start of a day found
// on record is the one record, so a restart between raising it and the
// day's first effect does not raise a second.
func TestRaiseOnceFindsTheRootOnRecord(t *testing.T) {
	e := open(t)
	ctx := context.Background()
	day := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	a, err := e.RaiseOnce(ctx, nil, StartOfDay, day, "clock")
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.RaiseOnce(ctx, nil, StartOfDay, day, "clock")
	if err != nil || b.ID != a.ID {
		t.Errorf("second RaiseOnce = %+v, %v; want the first root %s", b, err, a.ID)
	}
	c, _ := e.RaiseOnce(ctx, nil, StartOfDay, day.AddDate(0, 0, 1), "clock")
	if c.ID == a.ID {
		t.Error("the next day's root is the first's")
	}
	if n, _ := e.CountDue(ctx, day.AddDate(0, 0, 1)); n != 2 {
		t.Errorf("%d roots due, want 2", n)
	}
}

// A root raised in a transaction is on record only when it commits.
func TestARootIsRaisedInTheCallersTransaction(t *testing.T) {
	e := open(t)
	ctx := context.Background()
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	root, err := e.Raise(ctx, tx, Opening, noon, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Done(ctx, tx, root.ID); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
	if _, ok, _ := e.ByID(ctx, root.ID); ok {
		t.Error("a rolled-back root is on record")
	}
}

func TestSchemaVersionsRunFromOne(t *testing.T) {
	for i, m := range Schema.Migrations {
		if m.Version != i+1 {
			t.Errorf("migration %d has version %d", i, m.Version)
		}
	}
}
