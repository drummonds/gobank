package main

import (
	"testing"
	"time"
)

// The run row is the simulation's place in time: the instant the clock
// stands at, how many days the bank has run and whether the run loop was
// going. One row, overwritten at each event.
func TestRunRowRoundTrips(t *testing.T) {
	db := openTestDB(t)
	if err := migrate(db, demoSchemas()); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadRun(db); ok {
		t.Fatal("a fresh database has no run to resume")
	}
	day := time.Date(2021, 3, 4, 0, 0, 0, 0, time.UTC)
	at := day.Add(10*time.Hour + 32*time.Minute)
	saveRun(db, runState{At: at, DayCount: 428, Running: true})
	got, ok := loadRun(db)
	if !ok || !got.At.Equal(at) || !got.Day.Equal(day) || got.DayCount != 428 || !got.Running {
		t.Fatalf("loadRun = %+v, %v; want %v on %s, 428 days, running", got, ok, at, day.Format("2006-01-02"))
	}
	saveRun(db, runState{At: day.AddDate(0, 0, 1), DayCount: 429, Running: false})
	got, _ = loadRun(db)
	if got.DayCount != 429 || got.Running {
		t.Errorf("the row is overwritten, not appended: %+v", got)
	}
}
