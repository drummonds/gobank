package main

import (
	"database/sql"
	"errors"
	"git.bytestone.uk/hum3/gobank/internal/daylength"
	"log"
	"time"
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
}}

// runState is where the run is: the simulated day the bank is on, how
// many days it has run, and whether the run loop was going.
type runState struct {
	Day          time.Time
	DayCount     int
	Running      bool
	SavedAt      time.Time     // when the row was last written; read only, for the restart record
	DayLength    time.Duration // the day length set on the console; read only, see saveDayLength
	DayLengthSet bool          // false when the run never set one
	SlotStart    time.Time     // when the day in progress began, by the wall clock; read only, see saveSlotStart
}

// saveRun overwrites the run row.
func saveRun(db *sql.DB, r runState) {
	if db == nil {
		return
	}
	_, err := db.Exec(`INSERT INTO sim_run (id, current_day, day_count, running, updated_at) VALUES (1, $1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET current_day = EXCLUDED.current_day, day_count = EXCLUDED.day_count,
			running = EXCLUDED.running, updated_at = EXCLUDED.updated_at`,
		r.Day.UTC(), r.DayCount, r.Running, time.Now().UTC())
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

// saveSlotStart records when the simulated day in progress began, which
// saveRun leaves alone.
func saveSlotStart(db *sql.DB, at time.Time) {
	if db == nil {
		return
	}
	if _, err := db.Exec(`UPDATE sim_run SET slot_start = $1 WHERE id = 1`, at.UTC()); err != nil {
		log.Printf("saveSlotStart: %v", err)
	}
}

// loadRun reads the run row; ok is false when there is no run to resume.
func loadRun(db *sql.DB) (r runState, ok bool) {
	if db == nil {
		return runState{}, false
	}
	var dayLength sql.NullString
	var slotStart sql.NullTime
	err := db.QueryRow(`SELECT current_day, day_count, running, updated_at, day_length, slot_start FROM sim_run WHERE id = 1`).Scan(&r.Day, &r.DayCount, &r.Running, &r.SavedAt, &dayLength, &slotStart)
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
	// A simulated day is a UTC date, whatever the driver's location.
	r.Day = time.Date(r.Day.Year(), r.Day.Month(), r.Day.Day(), 0, 0, 0, 0, time.UTC)
	r.SavedAt = r.SavedAt.UTC()
	if slotStart.Valid {
		r.SlotStart = slotStart.Time.UTC()
	}
	return r, true
}
