package main

import (
	"context"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
)

// AboutStatus is the demo's state as another program reads it, at
// /about.json: the version and schema this process runs, the console
// settings, the run's position and the restart record. gobank-deploy's
// upgrade drill reads it before and after each redeploy to compare the
// position and read the downtime off the newest restart row.
type AboutStatus struct {
	Version  string          `json:"version"`
	Schema   []SchemaVersion `json:"schema"`
	Settings AboutSettings   `json:"settings"`
	Sim      AboutSim        `json:"sim"`
	Position AboutPosition   `json:"position"`
	Restarts []AboutRestart  `json:"restarts"`
}

// SchemaVersion is the version a component's tables are at.
type SchemaVersion struct {
	Component string `json:"component"`
	Version   int    `json:"version"`
}

// AboutSettings are the console settings.
type AboutSettings struct {
	DayLength    string `json:"day_length"` // Go duration; "0s" is flat out
	MaxCustomers int    `json:"max_customers"`
}

// AboutSim is the run as it is now.
type AboutSim struct {
	Running   bool   `json:"running"`
	DayEndsIn string `json:"day_ends_in"` // Go duration; "0s" when flat out or stopped
}

// AboutPosition is the bank's position: what the dashboard leads with.
type AboutPosition struct {
	Day       string `json:"day"` // YYYY-MM-DD
	DayCount  int    `json:"day_count"`
	Customers int    `json:"customers"`
	Savings   string `json:"savings"` // formatted, as the dashboard shows it
	Lending   string `json:"lending"`
}

// AboutRestart is one row of the restart record, newest first, read with
// the row before it (see Restart).
type AboutRestart struct {
	Version           string     `json:"version"`
	StartedAt         time.Time  `json:"started_at"`
	DayCount          int        `json:"day_count"`
	Customers         int        `json:"customers"`
	PreviousVersion   string     `json:"previous_version"` // "" for unrecorded or none
	PreviousStoppedAt *time.Time `json:"previous_stopped_at"`
	Downtime          string     `json:"downtime"` // Go duration; "" when unknown
	Intact            *bool      `json:"intact"`   // day and customers equal across the stop; null when the previous row cannot say
	StoppedAt         *time.Time `json:"stopped_at"`
	StopDayCount      int        `json:"stop_day_count"`
	StopCustomers     int        `json:"stop_customers"`
}

func aboutStatus(q core.BookQueries, ds *DemoState) AboutStatus {
	pos, _ := q.Position(context.Background())
	sim := ds.SimStatus()
	settings := ds.Settings()
	st := AboutStatus{
		Version:  version,
		Schema:   appliedSchemaVersions(ds.db),
		Settings: AboutSettings{DayLength: settings.DayLength.String(), MaxCustomers: settings.MaxCustomers},
		Sim:      AboutSim{Running: sim.Running, DayEndsIn: sim.DayEndsIn.Round(time.Second).String()},
		Position: AboutPosition{Day: pos.Day.Format("2006-01-02"), DayCount: pos.DayCount, Customers: pos.Customers, Savings: fmtMoney(pos.Savings), Lending: fmtMoney(pos.Lending)},
	}
	restarts := ds.Restarts(10)
	for i, r := range restarts {
		var previous *Restart
		if i+1 < len(restarts) {
			previous = &restarts[i+1]
		}
		st.Restarts = append(st.Restarts, aboutRestart(r, previous))
	}
	return st
}

func aboutRestart(r Restart, previous *Restart) AboutRestart {
	a := AboutRestart{
		Version: r.Version, StartedAt: r.StartedAt, DayCount: r.DayCount, Customers: r.Customers,
		PreviousVersion:   r.PreviousVersion,
		PreviousStoppedAt: nullableTime(r.PreviousStoppedAt),
		StoppedAt:         nullableTime(r.StoppedAt),
		StopDayCount:      r.StopDayCount, StopCustomers: r.StopCustomers,
	}
	if d, ok := r.Downtime(); ok {
		a.Downtime = d.Round(time.Millisecond).String()
	}
	// The same rule as the settings page: the handover is judged only
	// against the row this one actually followed, and only if it stopped.
	if previous != nil && !previous.StoppedAt.IsZero() && r.PreviousVersion == previous.Version {
		intact := r.ResumedIntact(*previous)
		a.Intact = &intact
	}
	return a
}

func nullableTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
