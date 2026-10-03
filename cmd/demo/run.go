package main

import (
	"database/sql"
	"errors"
	"log"
	"time"
)

// The simulation component's own table: the run's place in time. One row,
// overwritten at the end of every day and when the loop starts or stops,
// so a restart picks the run up where it was (ADR-0003: the run resumes).

var simulationSchema = componentSchema{component: "simulation", migrations: []migration{
	{1, []string{`CREATE TABLE IF NOT EXISTS sim_run (
		id INTEGER PRIMARY KEY,
		current_day TIMESTAMP NOT NULL,
		day_count INTEGER NOT NULL,
		running BOOLEAN NOT NULL,
		updated_at TIMESTAMP NOT NULL
	)`}},
}}

// runState is where the run is: the simulated day the bank is on, how
// many days it has run, and whether the run loop was going.
type runState struct {
	Day      time.Time
	DayCount int
	Running  bool
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

// loadRun reads the run row; ok is false when there is no run to resume.
func loadRun(db *sql.DB) (r runState, ok bool) {
	if db == nil {
		return runState{}, false
	}
	err := db.QueryRow(`SELECT current_day, day_count, running FROM sim_run WHERE id = 1`).Scan(&r.Day, &r.DayCount, &r.Running)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("loadRun: %v", err)
		}
		return runState{}, false
	}
	// A simulated day is a UTC date, whatever the driver's location.
	r.Day = time.Date(r.Day.Year(), r.Day.Month(), r.Day.Day(), 0, 0, 0, 0, time.UTC)
	return r, true
}
