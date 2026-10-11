package products

import (
	"context"
	"fmt"
	"log"
	"time"

	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/bank/ledger"
	"git.bytestone.uk/hum3/gobank/internal/yield"
)

// The parameter-change cascade (ADR-0010): every account's parameters
// count as applied up to an instant, recorded at open (Open) and
// advanced here. When a setting at a scope the account's rules read
// becomes effective after that instant, the account is owed a
// ParameterChange: the runner resolves each declared parameter at the
// applied-to instant and at now, fires the rule for each that differs,
// with the value then and now, and advances the instant. A derived
// parameter changes when its source does, so a market publication
// reaches an account through its derivation with no list of dependants
// kept anywhere. The firing is a function of stored facts, so it
// survives a restart, and a setting ahead of the clock is found when the
// clock reaches it, with nothing scheduled.

// owedChanges is the accounts owed a ParameterChange at now: those with
// no record, and those a setting reaching them became effective for
// after their applied-to instant and by now. A market setting reaches
// every account of its currency, a bank setting every account, a
// product's its accounts, an account's itself. The settings are read
// through the parameters component's contract view (ADR-0001).
const owedChanges = `FROM contract_customer_accounts ca
	LEFT JOIN product_account_parameters ap ON ap.account_id = ca.ledger_account_id
	WHERE ca.ledger_account_id > $3 AND (ap.account_id IS NULL OR EXISTS (
		SELECT 1 FROM contract_parameter_settings s
		WHERE s.effective_from > ap.applied_to AND s.effective_from <= $1
		  AND ((s.scope = 'market' AND s.owner = $2) OR s.scope = 'bank'
		    OR (s.scope = 'version' AND s.owner = ca.product_id)
		    OR (s.scope = 'account' AND s.owner = ca.ledger_account_id))))`

// CountParameterChangesOwed is how many accounts are owed a
// ParameterChange at now.
func (p *Products) CountParameterChangesOwed(ctx context.Context, now time.Time) (int, error) {
	var n int
	err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) `+owedChanges, now.UTC(), Currency, "").Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count parameter changes owed: %w", err)
	}
	return n, nil
}

// owedAccount is one account owed a change, with the instant its
// parameters count as applied up to (zero for an account with no record).
type owedAccount struct {
	Account
	appliedTo time.Time
}

// owed lists up to limit accounts owed a change at now, after cursor in
// ledger-account-ID order.
func (p *Products) owed(ctx context.Context, now time.Time, cursor string, limit int) ([]owedAccount, error) {
	rows, err := p.db.QueryContext(ctx, fmt.Sprintf(`SELECT ca.ledger_account_id, ca.product_id, ap.applied_to `+owedChanges+`
		ORDER BY ca.ledger_account_id LIMIT %d`, limit), now.UTC(), Currency, cursor)
	if err != nil {
		return nil, fmt.Errorf("parameter changes owed: %w", err)
	}
	defer rows.Close()
	var out []owedAccount
	for rows.Next() {
		var a owedAccount
		var applied *time.Time
		if err := rows.Scan(&a.ID, &a.Product, &applied); err != nil {
			return nil, fmt.Errorf("parameter changes owed: scan: %w", err)
		}
		if applied != nil {
			a.appliedTo = applied.UTC()
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ApplyParameterChanges is the cascade at now: every account owed a
// change is visited, under its lock and in a transaction of its own, its
// changes since its applied-to instant fired and the instant advanced to
// now; an account with no record is adopted silently. Returns early when
// ctx ends, leaving the rest owed. An account whose rules this build
// lacks is skipped and counted, as the pass does.
func (p *Products) ApplyParameterChanges(ctx context.Context, books *ledger.Ledger, now time.Time, progress Progress) PassResult {
	var res PassResult
	total, err := p.CountParameterChangesOwed(ctx, now)
	if err != nil {
		log.Printf("parameter changes: %v", err)
		return res
	}
	if total == 0 {
		return res
	}
	progress.Phase("applying parameter changes", total)
	cursor := ""
	unknown := map[string]bool{}
	for ctx.Err() == nil {
		accounts, err := p.owed(ctx, now, cursor, passChunk)
		if err != nil {
			log.Printf("parameter changes: %v", err)
			return res
		}
		if len(accounts) == 0 {
			return res
		}
		done := 0
		for _, a := range accounts {
			if ctx.Err() != nil {
				break
			}
			cursor = a.ID
			if prod, ok := p.ByID(a.Product); !ok || !prod.Carried() {
				if !unknown[a.Product] {
					unknown[a.Product] = true
					log.Printf("parameter changes: accounts on %s are left unapplied: this build does not carry its rules (%s v%d)", a.Product, prod.RulesID, prod.RulesVersion)
				}
				res.Skipped++
				continue
			}
			unlock := books.Lock(a.ID)
			fired, r, err := p.applyChanges(ctx, books, a, now)
			unlock()
			if err != nil {
				log.Printf("parameter changes: %s: %v", a.ID, err)
				continue
			}
			for _, d := range r {
				res.add(d)
			}
			res.Visited++
			if fired {
				res.Fired++
			}
			done++
		}
		progress.Add(done)
		yield.ToBrowser()
	}
	return res
}

// applyChanges is one account's cascade, in one transaction: the
// parameters that differ between the applied-to instant and now are
// fired, and the instant advanced. An account with no record is adopted:
// stamped, nothing fired.
func (p *Products) applyChanges(ctx context.Context, books *ledger.Ledger, a owedAccount, now time.Time) (fired bool, results []DayResult, err error) {
	prod, _ := p.ByID(a.Product)
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return false, nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	books = books.WithTx(tx)
	if !a.appliedTo.IsZero() {
		// What the bank knows at the clock's instant: every setting
		// decided by then. The changes are found before any rule fires,
		// so what a rule writes in answer is not a change of its own.
		knownAt := now
		var changes []gbp.Change
		for _, d := range prod.Rules.Parameters() {
			from, _ := p.resolveAt(ctx, tx, prod, a.ID, d.Key, a.appliedTo, knownAt) // unresolved then is a Param of nothing
			to, _ := p.resolveAt(ctx, tx, prod, a.ID, d.Key, now, knownAt)
			if from != to {
				changes = append(changes, gbp.Change{Key: d.Key, From: from, To: to, Effective: now})
			}
		}
		for _, change := range changes {
			r, err := p.run(gbp.EventParameterChange, books, a.Account, now, func(v gbp.Version, f gbp.Facts) (gbp.Intents, error) { return v.ParameterChange(f, change) })
			if err != nil {
				return false, nil, err
			}
			r.Accrued = 0 // an event's accrual is the pass's to count
			results = append(results, r)
			fired = true
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO product_account_parameters (account_id, applied_to) VALUES ($1, $2)
		ON CONFLICT (account_id) DO UPDATE SET applied_to = EXCLUDED.applied_to`, a.ID, now.UTC()); err != nil {
		return false, nil, fmt.Errorf("record parameters applied: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return false, nil, err
	}
	return fired, results, nil
}
