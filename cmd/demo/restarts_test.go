package main

import (
	"database/sql"
	"strings"
	"testing"
	"time"
)

// A process start is recorded against the one before it: which version
// handed over, when it stopped, and what the run looked like on both
// sides, so an upgrade's downtime and whether anything was lost are read
// off the restart record (ADR-0003).
func TestRestartRecordsTheHandover(t *testing.T) {
	defer func(v string) { version = v }(version)
	version = "v1"
	first := NewDemoState()
	for range 3 {
		first.createCustomer()
	}
	for range 5 {
		first.advanceDay()
	}
	before := first.position()
	first.RecordStop()

	version = "v2"
	second := newDemoStateOn(first.db, "")
	restarts := second.Restarts(10)
	if len(restarts) != 2 {
		t.Fatalf("restarts = %d rows, want 2 (first start, second start)", len(restarts))
	}
	latest, previous := restarts[0], restarts[1]
	if latest.Version != "v2" || previous.Version != "v1" {
		t.Errorf("versions newest first = %s, %s; want v2, v1", latest.Version, previous.Version)
	}
	if previous.StoppedAt.IsZero() {
		t.Fatal("the first process stopped cleanly; its record should say when")
	}
	if latest.PreviousVersion != "v1" || !latest.PreviousStoppedAt.Equal(previous.StoppedAt) {
		t.Errorf("handover = from %s stopped %v; want from v1 stopped %v", latest.PreviousVersion, latest.PreviousStoppedAt, previous.StoppedAt)
	}
	if d, ok := latest.Downtime(); !ok || d < 0 || d > time.Minute {
		t.Errorf("downtime = %v, %v; want a small known duration", d, ok)
	}
	if previous.StopDayCount != before.DayCount || previous.StopCustomers != before.Customers {
		t.Errorf("at stop: day %d, %d customers; want day %d, %d customers", previous.StopDayCount, previous.StopCustomers, before.DayCount, before.Customers)
	}
	if latest.DayCount != before.DayCount || latest.Customers != before.Customers {
		t.Errorf("at start: day %d, %d customers; want day %d, %d customers", latest.DayCount, latest.Customers, before.DayCount, before.Customers)
	}
	if !latest.ResumedIntact(previous) {
		t.Error("the same day and customers on both sides of the restart is nothing lost")
	}
}

// A process that did not stop cleanly (killed, crashed) leaves no stop
// time, so the next start knows the version it followed but not the
// downtime.
func TestUncleanStopLeavesDowntimeUnknown(t *testing.T) {
	defer func(v string) { version = v }(version)
	version = "v1"
	first := NewDemoState()
	first.advanceDay()

	version = "v2"
	second := newDemoStateOn(first.db, "")
	latest := second.Restarts(1)[0]
	if latest.PreviousVersion != "v1" || !latest.PreviousStoppedAt.IsZero() {
		t.Errorf("handover = from %s stopped %v; want from v1 with no stop time", latest.PreviousVersion, latest.PreviousStoppedAt)
	}
	if _, ok := latest.Downtime(); ok {
		t.Error("downtime after an unclean stop is unknown")
	}
}

// A release from before restart records (or a rollback to one) runs the
// bank without recording itself. The next recorded start notices the run
// row was written after the last recorded stop, so it names no previous
// version and measures its downtime from that last write.
func TestUnrecordedProcessBetweenRestarts(t *testing.T) {
	defer func(v string) { version = v }(version)
	version = "v2"
	first := NewDemoState()
	first.advanceDay()
	first.RecordStop()
	time.Sleep(20 * time.Millisecond)
	// The older release: it moves the run on and saves the run row.
	older := newDemoStateOnWithoutRecord(first.db)
	older.advanceDay()
	run, _ := loadRun(first.db)

	second := newDemoStateOn(first.db, "")
	latest := second.Restarts(1)[0]
	if latest.PreviousVersion != "" {
		t.Errorf("previous version = %q; want none: the process in between left no record", latest.PreviousVersion)
	}
	if !latest.PreviousStoppedAt.Equal(run.SavedAt) {
		t.Errorf("previous stop = %v; want the run row's last write %v", latest.PreviousStoppedAt, run.SavedAt)
	}
	if _, ok := latest.Downtime(); !ok {
		t.Error("downtime is measured from the run row's last write")
	}
}

// newDemoStateOnWithoutRecord is a process from a release before restart
// records: it runs over the database without a row of its own.
func newDemoStateOnWithoutRecord(db *sql.DB) *DemoState {
	ds := newDemoStateOn(db, "")
	if _, err := db.Exec(`DELETE FROM restarts WHERE id = $1`, ds.restartID); err != nil {
		panic(err)
	}
	return ds
}

// The first start over a fresh database follows nothing.
func TestFirstStartHasNoPrevious(t *testing.T) {
	ds := NewDemoState()
	restarts := ds.Restarts(10)
	if len(restarts) != 1 {
		t.Fatalf("restarts = %d rows, want 1", len(restarts))
	}
	r := restarts[0]
	if r.PreviousVersion != "" || !r.PreviousStoppedAt.IsZero() || !r.StoppedAt.IsZero() {
		t.Errorf("first start = %+v; want no previous and no stop", r)
	}
	if _, ok := r.Downtime(); ok {
		t.Error("the first start has no downtime")
	}
}

// The settings page shows the restart record, newest first.
func TestSettingsPageListsRestarts(t *testing.T) {
	defer func(v string) { version = v }(version)
	version = "v1"
	first := NewDemoState()
	first.RecordStop()
	version = "v2"
	second := newDemoStateOn(first.db, "")
	html := buildSettingsHTML(newCoreAdapter(second, ""), second.Settings(), false, second.Restarts(10))
	i2, i1 := strings.Index(html, "v2"), strings.Index(html, "v1")
	if !strings.Contains(html, "Restarts") || i2 < 0 || i1 < 0 || i2 > i1 {
		t.Errorf("settings page should list restarts newest first; got v2 at %d, v1 at %d", i2, i1)
	}
}
