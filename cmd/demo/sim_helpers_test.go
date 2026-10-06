package main

import "context"

// Shorthands for driving the simulation in tests.

func (ds *DemoState) createCustomer()                { ds.sim.OpenCustomer(context.Background()) }
func (ds *DemoState) advanceDay()                    { ds.sim.Step(context.Background()) }
func (ds *DemoState) nextDayCtx(ctx context.Context) { ds.sim.Step(ctx) }
