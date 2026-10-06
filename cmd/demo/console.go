package main

import (
	"context"
	"time"

	"git.bytestone.uk/hum3/gobank/cmd/demo/sim"
)

// The simulation console's surface on DemoState: the pages and the WASM
// bindings call these, and they forward to the simulation (package sim),
// which drives the bank through the core.

func (ds *DemoState) Start()                             { ds.sim.Start() }
func (ds *DemoState) Stop()                              { ds.sim.Stop() }
func (ds *DemoState) Shutdown(ctx context.Context) error { return ds.sim.Shutdown(ctx) }
func (ds *DemoState) IsRunning() bool                    { return ds.sim.IsRunning() }
func (ds *DemoState) ResumedRunning() bool               { return ds.sim.ResumedRunning() }
func (ds *DemoState) AdvanceDay()                        { ds.sim.AdvanceDay() }
func (ds *DemoState) SetDayLength(d time.Duration)       { ds.sim.SetDayLength(d) }
func (ds *DemoState) DefaultDayLength(d time.Duration)   { ds.sim.DefaultDayLength(d) }
func (ds *DemoState) Settings() sim.Settings             { return ds.sim.Settings() }
func (ds *DemoState) AddCustomersBatch(n int)            { ds.sim.AddCustomersBatch(n) }
func (ds *DemoState) IsAddingCustomers() bool            { return ds.sim.IsAddingCustomers() }
func (ds *DemoState) SendPayment()                       { ds.sim.SendPayment() }
func (ds *DemoState) StartPayments()                     { ds.sim.StartPayments() }
func (ds *DemoState) StopPayments()                      { ds.sim.StopPayments() }
func (ds *DemoState) IsPaymentsRunning() bool            { return ds.sim.IsPaymentsRunning() }

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
