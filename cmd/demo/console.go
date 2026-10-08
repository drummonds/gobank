package main

import (
	"context"
	"io"
	"log"
	"time"

	"git.bytestone.uk/hum3/gobank/bff/staff"
	"git.bytestone.uk/hum3/gobank/cmd/demo/sim"
)

// The simulation console's surface on DemoState (staff.Console): the
// staff web's console pages call these, and they forward to the
// simulation (package sim), which drives the bank through the core.

var _ staff.Console = (*DemoState)(nil)

func (ds *DemoState) Start()                             { ds.sim.Start() }
func (ds *DemoState) Stop()                              { ds.sim.Stop() }
func (ds *DemoState) Shutdown(ctx context.Context) error { return ds.sim.Shutdown(ctx) }
func (ds *DemoState) IsRunning() bool                    { return ds.sim.IsRunning() }
func (ds *DemoState) ResumedRunning() bool               { return ds.sim.ResumedRunning() }
func (ds *DemoState) AdvanceDay()                        { ds.sim.AdvanceDay() }
func (ds *DemoState) SetDayLength(d time.Duration)       { ds.sim.SetDayLength(d) }
func (ds *DemoState) DefaultDayLength(d time.Duration)   { ds.sim.DefaultDayLength(d) }
func (ds *DemoState) Settings() staff.Settings           { return staff.Settings(ds.sim.Settings()) }
func (ds *DemoState) AddCustomersBatch(n int)            { ds.sim.AddCustomersBatch(n) }
func (ds *DemoState) IsAddingCustomers() bool            { return ds.sim.IsAddingCustomers() }
func (ds *DemoState) SendPayment()                       { ds.sim.SendPayment() }
func (ds *DemoState) StartPayments()                     { ds.sim.StartPayments() }
func (ds *DemoState) StopPayments()                      { ds.sim.StopPayments() }
func (ds *DemoState) IsPaymentsRunning() bool            { return ds.sim.IsPaymentsRunning() }

// ResetPayments clears all payments and stops auto-generation.
func (ds *DemoState) ResetPayments() {
	ds.StopPayments()
	if err := ds.ClearPayments(context.Background()); err != nil {
		log.Print(err)
	}
}

// UpdateSettings sets the population cap, within the console's bounds.
func (ds *DemoState) UpdateSettings(maxCust int) {
	if maxCust < 3 || maxCust > 1_000_000 {
		return
	}
	ds.sim.SetMaxCustomers(maxCust)
}

// runStore records the run in the simulation component's table (run.go).
type runStore struct{ ds *DemoState }

func (r runStore) SaveRun(run sim.Run) {
	saveRun(r.ds.db, runState{Day: run.Day, DayCount: run.DayCount, Running: run.Running})
	saveSlotStart(r.ds.db, run.SlotStart)
}

func (r runStore) SaveDayLength(d time.Duration) { saveDayLength(r.ds.db, d) }

// SimStatus returns the console state under one lock: the simulation's
// status with the bank's pass rate and the process's memory state
// alongside.
func (ds *DemoState) SimStatus() staff.SimStatus {
	st := ds.sim.Status()
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return staff.SimStatus{
		Running: st.Running, PaymentsRunning: ds.sim.IsPaymentsRunning(),
		AddingCust: st.AddingCust, AddingProgress: st.AddingProgress, AddingTarget: st.AddingTarget,
		CustomersPerSec: st.CustomersPerSec, LastCustomersPerSec: st.LastCustomersPerSec,
		DayLength: st.DayLength, DayEndsIn: st.DayEndsIn, Wall: st.Wall, Clock: ds.clock.Now(), Warp: st.Warp,
		AccountDaysPer12h: ds.AccountDaysPer(passWindow),
		MemoryExceeded:    ds.memoryExceeded,
	}
}

// Export and Import pass the book through the bank (staff.Console wants
// the io types the bank's own signatures spell out).
func (ds *DemoState) Export(w io.Writer) error { return ds.Bank.Export(w) }
func (ds *DemoState) Import(r io.Reader) error { return ds.Bank.Import(r) }

// Runtime is the process and its data store, for the runtime page.
func (ds *DemoState) Runtime() staff.Runtime {
	ds.mu.Lock()
	db, backend, limit, exceeded := ds.db, ds.dbBackend, ds.memoryLimit, ds.memoryExceeded
	ds.mu.Unlock()
	rt := staff.Runtime{
		Env: runtimeEnv, DBBackend: backend, DBConfig: ds.dbConfigRows(), Schema: appliedSchemaVersions(db),
		MemoryLimit: limit, MemoryExceeded: exceeded, Progress: staff.DayProgress(ds.Progress()),
	}
	if db != nil {
		rt.DBStats = db.Stats()
	}
	return rt
}
