package gobank

// LendingAccountBehavior implements AccountBehavior for lending (asset) accounts.
// EndOfDay delegates to go-luca's CalculateDailyInterest.
type LendingAccountBehavior struct{}

func (LendingAccountBehavior) Name() string { return "lending" }

func (LendingAccountBehavior) OnActivate(ctx EventContext) error {
	ctx.Account.Status = StatusActive
	return nil
}

func (LendingAccountBehavior) OnOpen(ctx EventContext) error {
	ctx.Account.Status = StatusActive
	return nil
}

func (LendingAccountBehavior) OnClose(ctx EventContext) error {
	ctx.Account.Status = StatusClosed
	ctx.Account.ClosedAt = ctx.AsOfDate
	return nil
}

func (LendingAccountBehavior) EndOfDay(ctx EventContext) error {
	_, err := ctx.Sim.Ledger.CalculateDailyInterest(ctx.Account.Account.ID, ctx.AsOfDate)
	return err
}
