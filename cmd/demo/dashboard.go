package main

import (
	"context"

	"git.bytestone.uk/hum3/gobank/bff/staff"
	"git.bytestone.uk/hum3/gobank/core"
)

// SimStatus returns the console state under one lock: the simulation's
// status with the bank's pass rate and the process's memory state
// alongside, for the dashboard (staff.SimStatus).
func (ds *DemoState) SimStatus() staff.SimStatus {
	st := ds.sim.Status()
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return staff.SimStatus{
		Running: st.Running, AddingCust: st.AddingCust, AddingProgress: st.AddingProgress, AddingTarget: st.AddingTarget,
		CustomersPerSec: st.CustomersPerSec, LastCustomersPerSec: st.LastCustomersPerSec,
		DayLength: st.DayLength, DayEndsIn: st.DayEndsIn, Wall: st.Wall, Clock: ds.clock.Now(), Warp: st.Warp,
		AccountDaysPer12h: ds.AccountDaysPer(passWindow),
		MemoryExceeded:    ds.memoryExceeded,
	}
}

// dashboardData gathers the dashboard's data: the bank through the core
// queries, the console from the simulation.
func dashboardData(q core.BookQueries, ds *DemoState) staff.DashData {
	pos, _ := q.Position(context.Background())
	hist, _ := q.History(context.Background())
	return staff.DashData{Sim: ds.SimStatus(), Bank: pos, History: hist}
}

// buildDashboardHTML renders the dashboard's data sections, without the
// controls.
func buildDashboardHTML(q core.BookQueries, ds *DemoState) string {
	return staff.BuildDashboardHTML(dashboardData(q, ds))
}
