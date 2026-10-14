package customers

import (
	"context"
	"fmt"
	"strings"
)

// ByName is the customers whose name contains name, whatever the case,
// oldest first, at most limit of them, each with their accounts; an empty
// name finds nobody. Names are encrypted at rest, so there is no column
// to search: the register is read oldest first and every customer's name
// decrypted and matched in turn until limit are found or the register
// runs out — a cost proportional to the book, which the capacity
// milestone measures.
func (c *Customers) ByName(ctx context.Context, name string, limit int) ([]Record, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || limit <= 0 {
		return nil, nil
	}
	const page = 500
	var out []Record
	for offset := 0; ; offset += page {
		recs, _, err := c.store.List(ctx, offset, page)
		if err != nil {
			return nil, fmt.Errorf("customers: by name: %w", err)
		}
		for _, rec := range recs {
			n, err := c.store.GetNameByID(ctx, rec.ID)
			if err != nil {
				return nil, fmt.Errorf("customers: %s: name: %w", rec.ID, err)
			}
			if !strings.Contains(strings.ToLower(n), name) {
				continue
			}
			r := recordOf(&rec)
			if r.Accounts, err = c.accountsOf(ctx, rec.ID); err != nil {
				return nil, err
			}
			out = append(out, r)
			if len(out) == limit {
				return out, nil
			}
		}
		if len(recs) < page {
			return out, nil
		}
	}
}
