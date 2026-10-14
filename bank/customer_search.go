package bank

import (
	"context"

	gbp "git.bytestone.uk/hum3/gobank-products"

	"git.bytestone.uk/hum3/gobank/bank/customers"
	"git.bytestone.uk/hum3/gobank/core"
)

// CustomersByName implements core.CustomerRegister: the customers whose
// name contains name, oldest first, at most core.CustomerSearchLimit.
func (b *Bank) CustomersByName(ctx context.Context, name string) ([]core.CustomerSummary, error) {
	b.open.RLock()
	defer b.open.RUnlock()
	recs, err := b.customers.ByName(ctx, name, core.CustomerSearchLimit)
	if err != nil {
		return nil, err
	}
	out := make([]core.CustomerSummary, 0, len(recs))
	for _, c := range recs {
		out = append(out, summaryOf(c))
	}
	return out, nil
}

// summaryOf is a customer's row of the register: their accounts counted
// and their balances summed by family.
func summaryOf(c customers.Record) core.CustomerSummary {
	row := core.CustomerSummary{ID: c.ID, Accounts: len(c.Accounts)}
	for _, acc := range c.Accounts {
		if acc.Family == gbp.FamilySavings {
			row.Savings += acc.Balance
		} else {
			row.Lending += acc.Balance
		}
	}
	return row
}
