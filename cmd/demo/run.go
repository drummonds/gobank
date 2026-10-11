package main

import (
	"database/sql"
	"errors"
	"log"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/gobank/internal/daylength"
)

// The simulation component's tables: the run's place in time, and the
// restart record (restarts.go). One row,
// overwritten at the end of every day and when the loop starts or stops,
// so a restart picks the run up where it was (ADR-0003: the run resumes).

var simulationSchema = componentSchema{Name: "simulation", Migrations: []migration{
	{Version: 1, Statements: []string{`CREATE TABLE IF NOT EXISTS sim_run (
		id INTEGER PRIMARY KEY,
		current_day TIMESTAMP NOT NULL,
		day_count INTEGER NOT NULL,
		running BOOLEAN NOT NULL,
		updated_at TIMESTAMP NOT NULL
	)`}},
	restartsMigration,
	// The day length set on the console is the run's: recorded so the next
	// process resumes at it (ADR-0003: an upgrade lands mid-day). NULL is
	// a run that never set one and takes the environment's.
	{Version: 3, Statements: []string{`ALTER TABLE sim_run ADD COLUMN day_length VARCHAR(20) NULL`}},
	// When the simulated day in progress began, by the wall clock, so the
	// next process resumes the clock mid-day (stage 4: the simulation's
	// clock). NULL is a run from before the clock.
	{Version: 4, Statements: []string{`ALTER TABLE sim_run ADD COLUMN slot_start TIMESTAMP NULL`}},
	// The instant the clock stands at (ADR-0010: the simulation moves by
	// events), so the next process resumes at the same instant and the
	// bank finishes the same cascade. current_day and slot_start are
	// still written for one release, for the drill's rollback. NULL is a
	// run from before the event clock, resumed at the start of its day.
	{Version: 5, Statements: []string{`ALTER TABLE sim_run ADD COLUMN current_at TIMESTAMP NULL`}},
}}

// runState is where the run is: the instant the clock stands at (and
// its day), how many days the bank has run, and whether the run loop was
// going.
type runState struct {
	At           time.Time // the instant the clock stands at
	Day          time.Time // its day; read only, derived from At when saved
	DayCount     int
	Running      bool
	SavedAt      time.Time     // when the row was last written; read only, for the restart record
	DayLength    time.Duration // the day length set on the console; read only, see saveDayLength
	DayLengthSet bool          // false when the run never set one
}

// saveRun overwrites the run row. The day and a slot start are written
// beside the instant for one release, for a rollback to the slot clock.
func saveRun(db *sql.DB, r runState) {
	if db == nil {
		return
	}
	now := time.Now().UTC()
	_, err := db.Exec(`INSERT INTO sim_run (id, current_day, current_at, day_count, running, updated_at, slot_start) VALUES (1, $1, $2, $3, $4, $5, $5)
		ON CONFLICT (id) DO UPDATE SET current_day = EXCLUDED.current_day, current_at = EXCLUDED.current_at, day_count = EXCLUDED.day_count,
			running = EXCLUDED.running, updated_at = EXCLUDED.updated_at, slot_start = EXCLUDED.slot_start`,
		core.BusinessDay(r.At), r.At.UTC(), r.DayCount, r.Running, now)
	if err != nil {
		log.Printf("saveRun: %v", err)
	}
}

// saveDayLength records the day length set on the console against the run
// row, which saveRun leaves alone: the next process resumes at it.
func saveDayLength(db *sql.DB, d time.Duration) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`UPDATE sim_run SET day_length = $1 WHERE id = 1`, d.String()); err != nil {
		log.Printf("saveDayLength: %v", err)
	}
}

// loadRun reads the run row; ok is false when there is no run to resume.
func loadRun(db *sql.DB) (r runState, ok bool) {
	if db == nil {
		return runState{}, false
	}
	var dayLength sql.NullString
	var at sql.NullTime
	err := db.QueryRow(`SELECT current_day, current_at, day_count, running, updated_at, day_length FROM sim_run WHERE id = 1`).Scan(&r.Day, &at, &r.DayCount, &r.Running, &r.SavedAt, &dayLength)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("loadRun: %v", err)
		}
		return runState{}, false
	}
	if dayLength.Valid {
		if d, err := daylength.Parse(dayLength.String); err == nil {
			r.DayLength, r.DayLengthSet = d, true
		}
	}
	// A simulated day is a UTC date, whatever the driver's location; a
	// run from before the event clock resumes at the start of its day.
	r.Day = time.Date(r.Day.Year(), r.Day.Month(), r.Day.Day(), 0, 0, 0, 0, time.UTC)
	r.At = r.Day
	if at.Valid {
		r.At = at.Time.UTC()
	}
	r.SavedAt = r.SavedAt.UTC()
	return r, true
}
