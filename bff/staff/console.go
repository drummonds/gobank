package staff

import (
	"context"
	"database/sql"
	"io"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
)

// Console is the simulation console the staff web drives (ADR-0002 stage
// 6, story 1.6.4): what runs the bank in the demo. The demo implements it;
// a real bank has none and serves the staff web without these pages.
type Console interface {
	// SimStatus is the run as it is now.
	SimStatus() SimStatus
	Start()
	Stop()
	AdvanceDay()
	Reset()
	AddCustomersBatch(n int)

	// The payments generator.
	SendPayment()
	StartPayments()
	StopPayments()

	Settings() Settings
	UpdateSettings(maxCustomers int) // out-of-range values are refused
	SetDayLength(d time.Duration)

	// Restarts is the restart record, newest first.
	Restarts(n int) []Restart
	// Runtime is the process and its data store, for the runtime page.
	Runtime() Runtime

	Export(w io.Writer) error
	Import(r io.Reader) error

	// DB is the database as it stands, for the explorer.
	DB() *sql.DB
}

// SimStatus is the simulation console's own state: what is running and
// how fast. It is not part of the bank (ADR-0002: the console drives the
// generators) and so not a core query; the console fills it in.
type SimStatus struct {
	Running             bool
	PaymentsRunning     bool
	AddingCust          bool
	AddingProgress      int
	AddingTarget        int
	CustomersPerSec     float64 // live rate of the running batch add
	LastCustomersPerSec float64 // rate of the last finished batch add
	AccountDaysPer12h   int64   // accounts the pass would project in 12h at the measured whole-day rate
	MemoryExceeded      bool
	DayLength           time.Duration // wall-clock length of a simulated day; zero is flat out
	DayEndsIn           time.Duration // what is left of the day in progress; zero when flat out or stopped
	Wall                time.Time     // the wall clock now
	Clock               time.Time     // the bank's clock now: simulated time
	Warp                float64       // simulated seconds per wall second; zero when flat out
}

// Busy says whether anything is going: the run loop, the payments
// generator or a batch add. The layout shows it and polls while it is.
func (s SimStatus) Busy() bool { return s.Running || s.PaymentsRunning || s.AddingCust }

// Settings are the console's settings.
type Settings struct {
	MaxCustomers int
	DayLength    time.Duration // wall-clock length of a simulated day; zero is flat out
}

// Restart is one process's time over the database: the restart record
// the settings page and /about.json show.
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

// SchemaVersion is the version a component's tables are at.
type SchemaVersion struct {
	Component string `json:"component"`
	Version   int    `json:"version"`
}

// DayProgress is the day in progress, or the last day done, as the bank
// reports it.
type DayProgress struct {
	Active      bool
	Day         time.Time
	Phase       string
	Done, Total int
	Elapsed     time.Duration // since the day began
	Rate        float64       // accounts/s within the current phase

	LastDay      time.Time
	LastDuration time.Duration
	LastAccounts int
}

// Runtime is the process and its data store as the runtime page shows
// them.
type Runtime struct {
	Env            string // "HTTP Server" or "WebAssembly (WASM)"
	DBBackend      string // human-readable data store description
	DBStats        sql.DBStats
	DBConfig       [][2]string // label/value rows describing the live database, PostgreSQL only
	Schema         []SchemaVersion
	MemoryLimit    uint64 // the auto-stop threshold
	MemoryExceeded bool
	Progress       DayProgress
}

// DashData is what the dashboard shows: the bank's position and history
// through the core, and the console's own state.
type DashData struct {
	Sim     SimStatus
	Bank    core.Position
	History core.History
}

// DashboardData gathers the dashboard's data: the bank through the core
// queries, the console from the simulation.
func DashboardData(q core.BookQueries, c Console) DashData {
	pos, _ := q.Position(context.Background())
	hist, _ := q.History(context.Background())
	return DashData{Sim: c.SimStatus(), Bank: pos, History: hist}
}
