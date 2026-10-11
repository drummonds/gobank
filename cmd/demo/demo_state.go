package main

import (
	"context"
	"database/sql"
	"log"
	"runtime"
	"sync"
	"time"

	"git.bytestone.uk/hum3/gobank/bank"
	"git.bytestone.uk/hum3/gobank/cmd/demo/sim"
	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/gobank/modelbank"
)

// passWindow is the batch window the pass's throughput is quoted against:
// a bank's overnight run. "Account days per 12h" says whether the pass, at
// its measured rate, could project every account's day overnight.
const passWindow = 12 * time.Hour

// DemoState is the demo application: the bank (package bank, ADR-0002
// stage 5) wired to the simulation that drives it (package sim, stage 4),
// the database it runs over, the restart record and the console. The
// simulation reaches the bank only through the core; the console's
// methods on DemoState forward to it (console.go). The bank is embedded:
// the pages and the simulation read and write it through the core's
// interfaces, which DemoState satisfies through it.
type DemoState struct {
	*bank.Bank

	mu             sync.Mutex
	restartID      int64  // this process's row in the restart record (mu)
	dsn            string // the database this state was opened on, "" for in-memory
	db             *sql.DB
	dbBackend      string          // human-readable data store description, set by attachDB
	dbIsPostgres   bool            // real PostgreSQL (pgx) rather than in-memory pglike
	sim            *sim.Simulation // the simulation driving this bank through the core (wiring; console.go forwards to it)
	clock          core.Clock      // where the bank reads the time (ADR-0002 stage 4); tests replace it
	memoryLimit    uint64          // auto-stop threshold, see SetMemoryLimit (mu)
	memoryExceeded bool            // true when heap > memoryLimit; simulation pauses (mu)
}

func NewDemoState() *DemoState {
	return NewDemoStateWithDSN("")
}

// NewDemoStateWithDSN creates a DemoState backed by the given database.
// Empty dsn uses in-memory pglike; a postgres:// DSN uses real PostgreSQL,
// and the run recorded in it, if any, resumes.
func NewDemoStateWithDSN(dsn string) *DemoState {
	return newDemoState().openOn(openDB(dsn), dsn)
}

// newDemoStateOn is a DemoState over an already open database: the next
// process over the same database, in tests.
func newDemoStateOn(db *sql.DB, dsn string) *DemoState {
	return newDemoState().openOn(db, dsn)
}

func newDemoState() *DemoState {
	return &DemoState{memoryLimit: defaultMemoryLimit}
}

// openOn takes db as the state's database, wires the simulation, opens
// the bank over the database — resuming the run recorded in it, if any —
// and records the start.
func (ds *DemoState) openOn(db *sql.DB, dsn string) *DemoState {
	ds.attachDB(db, dsn)
	ds.wireSimulation()
	run, resumed := loadRun(ds.db)
	if resumed {
		// The simulation resumes its clock's day; the bank's business day
		// is the latest on its own record, the daily snapshots.
		ds.sim.Resume(sim.Run{At: run.At, DayCount: run.DayCount, Running: run.Running}, run.DayLength, run.DayLengthSet)
	}
	ds.openBank()
	ds.sim.CatchUp() // the feed: the market's publications up to the clock (ADR-0009)
	if !resumed {
		saveRun(ds.db, runState{At: ds.clock.Now()})
	}
	pos := ds.position()
	ds.mu.Lock()
	ds.restartID = recordStart(ds.db, version, pos.DayCount, pos.Customers, run.SavedAt)
	ds.mu.Unlock()
	return ds
}

// openBank opens the bank over the demo's database, on the simulation's
// clock; the risk-free rate reaches it as the simulation publishes the
// market's series (ADR-0009). A fresh bank over a wiped database (Reset)
// reopens the same handle, so the simulation and the pages keep it.
func (ds *DemoState) openBank() {
	opts := bank.Options{
		Definition:    modelbank.Definition(),
		Clock:         core.ClockFunc(func() time.Time { return ds.clock.Now() }),
		Seed:          42,
		AdminPassword: adminPassword(),
	}
	if ds.Bank != nil {
		if err := ds.Bank.Reopen(ds.db); err != nil {
			log.Fatalf("bank: %v", err)
		}
		return
	}
	b, err := bank.Open(ds.db, opts)
	if err != nil {
		log.Fatalf("bank: %v", err)
	}
	ds.Bank = b
}

// wireSimulation puts the simulation on the bank (stage 4): the bank reads
// the time from the simulation's clock, the simulation publishes the
// market's series as its feed and drives the bank through the core, with
// the run recorded in the simulation component's table. A real deployment
// would wire the wall clock and a poller on the market's feed here, and no
// simulation.
func (ds *DemoState) wireSimulation() {
	ds.sim = sim.New(ds, runStore{ds}, sim.Options{
		Workers: ds.dbWriters(),
		Pause:   ds.memoryPause,
		Seed:    42,
	})
	ds.clock = ds.sim.Clock()
}

// position is the bank's position, for the wiring.
func (ds *DemoState) position() core.Position {
	pos, err := ds.Position(context.Background())
	if err != nil {
		log.Printf("position: %v", err)
	}
	return pos
}

// dbWriters is how many connections write to the database at once when
// adding customers. On PostgreSQL the writes spread across cores; the
// in-memory pglike store (and WASM) takes one writer at a time.
func (ds *DemoState) dbWriters() int {
	if !ds.dbIsPostgres {
		return 1
	}
	return runtime.NumCPU()
}

// Reset is a fresh run on the same database: the simulation stops and its
// clock goes back to the opening day, the bank's tables go, and the bank
// is reopened as one on its first day.
func (ds *DemoState) Reset() {
	ds.sim.Reset(sim.OpeningDay)
	ds.mu.Lock()
	ds.memoryExceeded = false
	ds.mu.Unlock()
	if ds.dbIsPostgres {
		// A fresh run on the same database: every table goes and the
		// schema is migrated again from nothing.
		dropAllPublicTables(ds.db)
		ds.attachDB(ds.db, ds.dsn)
	} else {
		ds.initDB()
	}
	ds.openBank()
	ds.sim.CatchUp() // the feed: the series up to the opening day
	saveRun(ds.db, runState{At: ds.clock.Now()})
	if ds.sim.DayLengthRecorded() { // the console's setting outlives the run it was made in
		saveDayLength(ds.db, ds.sim.Settings().DayLength)
	}
}
