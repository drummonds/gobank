package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// Restart records. One row per process start over the database: the
// version that started, when, what the run looked like, and when that
// process stopped cleanly. Read together with the row before it, a start
// is an upgrade's record (ADR-0003): the downtime is the gap from the
// previous stop to this start, and the run on both sides of the gap says
// whether anything was lost. The migration only adds the table, so the
// previous release runs on the same database and a rollback is possible.
var restartsMigration = migration{Version: 2, Statements: []string{`CREATE TABLE IF NOT EXISTS restarts (
	id SERIAL PRIMARY KEY,
	version VARCHAR(50) NOT NULL,
	started_at TIMESTAMP NOT NULL,
	day_count INTEGER NOT NULL,
	customers INTEGER NOT NULL,
	previous_version VARCHAR(50) NOT NULL,
	previous_stopped_at TIMESTAMP NULL,
	stopped_at TIMESTAMP NULL,
	stop_day_count INTEGER NULL,
	stop_customers INTEGER NULL
)`}}

// Restart is one process's time over the database.
type Restart struct {
	Version   string
	StartedAt time.Time
	DayCount  int // the run at start
	Customers int

	// The process this one followed. PreviousVersion is empty when that
	// process left no record: a release from before this table, or a
	// rollback to one. PreviousStoppedAt is zero when the stop time is
	// unknown, because the previous process did not stop cleanly.
	PreviousVersion   string
	PreviousStoppedAt time.Time

	// Set when this process stopped cleanly.
	StoppedAt     time.Time
	StopDayCount  int
	StopCustomers int
}

// Downtime is how long the bank was unserved before this start: from the
// previous process's stop (or, for a previous process that kept no
// record, its last write of the run row) to this start. ok is false when
// the previous stop time is unknown.
func (r Restart) Downtime() (d time.Duration, ok bool) {
	if r.PreviousStoppedAt.IsZero() {
		return 0, false
	}
	return r.StartedAt.Sub(r.PreviousStoppedAt), true
}

// ResumedIntact reports that this start found the run where previous
// left it: the same day and the same customers.
func (r Restart) ResumedIntact(previous Restart) bool {
	return !previous.StoppedAt.IsZero() && r.DayCount == previous.StopDayCount && r.Customers == previous.StopCustomers
}

// recordStart inserts this process's row. runSavedAt is when the run row
// was last written before this process touched it: a write after the last
// recorded stop means an unrecorded process ran in between, and the
// downtime is measured from that write instead.
func recordStart(db *sql.DB, version string, dayCount, customers int, runSavedAt time.Time) (id int64) {
	if db == nil {
		return 0
	}
	prevVersion, prevStop := previousStop(db, runSavedAt)
	err := db.QueryRow(`INSERT INTO restarts (version, started_at, day_count, customers, previous_version, previous_stopped_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		version, time.Now().UTC(), dayCount, customers, prevVersion, nullTime(prevStop)).Scan(&id)
	if err != nil {
		log.Printf("recordStart: %v", err)
	}
	return id
}

// previousStop works out which process this start follows and when it
// stopped, from the newest restart row and the run row's last write.
func previousStop(db *sql.DB, runSavedAt time.Time) (version string, stoppedAt time.Time) {
	var prev string
	var stop sql.NullTime
	err := db.QueryRow(`SELECT version, stopped_at FROM restarts ORDER BY id DESC LIMIT 1`).Scan(&prev, &stop)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Nothing recorded: a database written by a release before this
		// table. Its last write is all there is to measure from.
		return "", runSavedAt
	case err != nil:
		log.Printf("previousStop: %v", err)
		return "", time.Time{}
	case !stop.Valid:
		return prev, time.Time{}
	case runSavedAt.After(stop.Time.UTC()):
		// The run moved on after the last recorded stop: an unrecorded
		// process ran in between.
		return "", runSavedAt
	}
	return prev, stop.Time.UTC()
}

// recordStop marks this process's row as stopped cleanly, with the run as
// it leaves it.
func recordStop(db *sql.DB, id int64, dayCount, customers int) {
	if db == nil || id == 0 {
		return
	}
	if _, err := db.Exec(`UPDATE restarts SET stopped_at = $1, stop_day_count = $2, stop_customers = $3 WHERE id = $4`,
		time.Now().UTC(), dayCount, customers, id); err != nil {
		log.Printf("recordStop: %v", err)
	}
}

// listRestarts reads the newest n restart rows, newest first.
func listRestarts(db *sql.DB, n int) []Restart {
	if db == nil {
		return nil
	}
	rows, err := db.Query(`SELECT version, started_at, day_count, customers, previous_version, previous_stopped_at,
		stopped_at, stop_day_count, stop_customers FROM restarts ORDER BY id DESC LIMIT $1`, n)
	if err != nil {
		log.Printf("listRestarts: %v", err)
		return nil
	}
	defer rows.Close()
	var out []Restart
	for rows.Next() {
		var r Restart
		var prevStop, stop sql.NullTime
		var stopDay, stopCust sql.NullInt64
		if err := rows.Scan(&r.Version, &r.StartedAt, &r.DayCount, &r.Customers, &r.PreviousVersion, &prevStop,
			&stop, &stopDay, &stopCust); err != nil {
			log.Printf("listRestarts: %v", err)
			return out
		}
		r.StartedAt = r.StartedAt.UTC()
		if prevStop.Valid {
			r.PreviousStoppedAt = prevStop.Time.UTC()
		}
		if stop.Valid {
			r.StoppedAt = stop.Time.UTC()
		}
		r.StopDayCount, r.StopCustomers = int(stopDay.Int64), int(stopCust.Int64)
		out = append(out, r)
	}
	return out
}

// Restarts is the restart record, newest first.
func (ds *DemoState) Restarts(n int) []Restart { return listRestarts(ds.db, n) }

// RecordStop is the process's last write: it stopped cleanly, leaving the
// run here. serve calls it after the HTTP server has drained.
func (ds *DemoState) RecordStop() {
	ds.mu.Lock()
	db, id, day, customers := ds.db, ds.restartID, ds.dayCount, ds.nCustomers
	ds.mu.Unlock()
	recordStop(db, id, day, customers)
}

// renderRestarts is the settings page's restart record: each start
// against the stop before it, so an upgrade's downtime and what it
// carried across are read off the console.
func renderRestarts(restarts []Restart) string {
	var s strings.Builder
	s.WriteString(`<h3 class="title is-5 mt-5">Restarts</h3>`)
	s.WriteString(`<p class="help mb-2">Each process start against the stop before it. Downtime is from the previous stop (or its last write of the run, for a release that kept no record) to this start; day and customers are at that stop and at this start.</p>`)
	s.WriteString(`<table class="table is-narrow is-fullwidth"><thead><tr><th>Started</th><th>Version</th><th>Previous</th><th>Downtime</th><th>Day</th><th>Customers</th><th>Stopped</th></tr></thead><tbody>`)
	for i, r := range restarts {
		var previous *Restart
		if i+1 < len(restarts) {
			previous = &restarts[i+1]
		}
		s.WriteString(`<tr>`)
		s.WriteString(fmt.Sprintf(`<td>%s</td><td>%s</td>`, r.StartedAt.Format("2 Jan 15:04:05"), r.Version))
		switch {
		case r.PreviousVersion != "":
			s.WriteString(fmt.Sprintf(`<td>%s</td>`, r.PreviousVersion))
		case !r.PreviousStoppedAt.IsZero():
			s.WriteString(`<td class="has-text-grey">unrecorded</td>`)
		default:
			s.WriteString(`<td class="has-text-grey">—</td>`)
		}
		if d, ok := r.Downtime(); ok {
			s.WriteString(fmt.Sprintf(`<td>%s</td>`, d.Round(time.Second)))
		} else if r.PreviousVersion != "" {
			s.WriteString(`<td class="has-text-danger">unknown (unclean stop)</td>`)
		} else {
			s.WriteString(`<td class="has-text-grey">—</td>`)
		}
		s.WriteString(renderHandover(r, previous))
		if r.StoppedAt.IsZero() {
			s.WriteString(`<td class="has-text-grey">—</td>`)
		} else {
			s.WriteString(fmt.Sprintf(`<td>%s</td>`, r.StoppedAt.Format("2 Jan 15:04:05")))
		}
		s.WriteString(`</tr>`)
	}
	s.WriteString(`</tbody></table>`)
	return s.String()
}

// renderHandover is the day and customers cells: at the previous stop and
// at this start, red when they differ.
func renderHandover(r Restart, previous *Restart) string {
	if previous == nil || previous.StoppedAt.IsZero() || r.PreviousVersion != previous.Version {
		return fmt.Sprintf(`<td>%d</td><td>%s</td>`, r.DayCount, groupThousands(strconv.Itoa(r.Customers)))
	}
	class := ""
	if !r.ResumedIntact(*previous) {
		class = ` class="has-text-danger"`
	}
	return fmt.Sprintf(`<td%s>%d → %d</td><td%s>%s → %s</td>`, class, previous.StopDayCount, r.DayCount,
		class, groupThousands(strconv.Itoa(previous.StopCustomers)), groupThousands(strconv.Itoa(r.Customers)))
}
