package gobank

// SavingsAccountBehavior implements AccountBehavior for savings accounts.
// EndOfDay delegates to go-luca's CalculateDailyInterest.
type SavingsAccountBehavior struct{}

func (SavingsAccountBehavior) Name() string { return "savings" }

func (SavingsAccountBehavior) OnActivate(ctx EventContext) error {
	ctx.Account.Status = StatusActive
	return nil
}

func (SavingsAccountBehavior) OnOpen(ctx EventContext) error {
	ctx.Account.Status = StatusActive
	return nil
}

func (SavingsAccountBehavior) OnClose(ctx EventContext) error {
	ctx.Account.Status = StatusClosed
	ctx.Account.ClosedAt = ctx.AsOfDate
	return nil
}

func (SavingsAccountBehavior) EndOfDay(ctx EventContext) error {
	_, err := ctx.Sim.Ledger.CalculateDailyInterest(ctx.Account.Account.ID, ctx.AsOfDate)
	return err
}
