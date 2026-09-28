package main

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
)

// fakeRecorder records every movement it is handed and the peak number of
// concurrent RecordLinkedMovements calls.
type fakeRecorder struct {
	mu       sync.Mutex
	got      []luca.MovementInput
	times    []time.Time
	inFlight atomic.Int32
	peak     atomic.Int32
	failAt   int // fail the call that would take the total past this; 0 = never
}

func (f *fakeRecorder) RecordLinkedMovements(ms []luca.MovementInput, vt time.Time) (string, error) {
	n := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		p := f.peak.Load()
		if n <= p || f.peak.CompareAndSwap(p, n) {
			break
		}
	}
	time.Sleep(200 * time.Microsecond) // long enough for workers to overlap
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAt > 0 && len(f.got)+len(ms) > f.failAt {
		return "", fmt.Errorf("injected failure")
	}
	f.got = append(f.got, ms...)
	for range ms {
		f.times = append(f.times, vt)
	}
	return "batch", nil
}

func testBatch(n int) accrualBatch {
	vt := time.Date(2020, 3, 17, 23, 59, 58, 0, time.UTC)
	b := accrualBatch{valueTime: vt}
	for i := range n {
		b.inputs = append(b.inputs, luca.MovementInput{
			FromAccountID: "pnl", ToAccountID: fmt.Sprintf("acct-%d", i),
			Amount: luca.Amount(i + 1), Code: codeDailyAccrual,
		})
	}
	return b
}

func TestPostMovementsEachExactlyOnce(t *testing.T) {
	for _, workers := range []int{1, 4} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			b := testBatch(5000)
			rec := &fakeRecorder{}
			var reported atomic.Int64
			if err := postMovements(rec, b, workers, func(n int) { reported.Add(int64(n)) }); err != nil {
				t.Fatal(err)
			}
			if len(rec.got) != len(b.inputs) {
				t.Fatalf("posted %d movements, want %d", len(rec.got), len(b.inputs))
			}
			seen := make(map[string]int)
			for _, m := range rec.got {
				seen[m.ToAccountID]++
			}
			for _, m := range b.inputs {
				if seen[m.ToAccountID] != 1 {
					t.Fatalf("%s posted %d times", m.ToAccountID, seen[m.ToAccountID])
				}
			}
			for _, vt := range rec.times {
				if !vt.Equal(b.valueTime) {
					t.Fatalf("value time %v, want %v", vt, b.valueTime)
				}
			}
			if int(reported.Load()) != len(b.inputs) {
				t.Errorf("progress reported %d, want %d", reported.Load(), len(b.inputs))
			}
		})
	}
}

func TestPostMovementsRunsWorkersConcurrently(t *testing.T) {
	rec := &fakeRecorder{}
	if err := postMovements(rec, testBatch(5000), 4, func(int) {}); err != nil {
		t.Fatal(err)
	}
	if p := rec.peak.Load(); p < 2 {
		t.Errorf("peak concurrent writes %d, want >1 with 4 workers", p)
	}
	if p := rec.peak.Load(); p > 4 {
		t.Errorf("peak concurrent writes %d, want <=4", p)
	}
}

func TestPostMovementsStopsOnError(t *testing.T) {
	rec := &fakeRecorder{failAt: 1000}
	err := postMovements(rec, testBatch(5000), 4, func(int) {})
	if err == nil {
		t.Fatal("want error from failing ledger")
	}
	if len(rec.got) >= 5000 {
		t.Errorf("posted all %d movements despite failure", len(rec.got))
	}
}

// TestPostMovementsPostgres posts through a real ledger on PostgreSQL with
// parallel workers and checks the stored rows match the batch exactly.
// Set GOBANK_PG_DSN to run.
func TestPostMovementsPostgres(t *testing.T) {
	dsn := os.Getenv("GOBANK_PG_DSN")
	if dsn == "" {
		t.Skip("GOBANK_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dropAllPublicTables(db)
	ledger, err := luca.NewSQLLedger(db)
	if err != nil {
		t.Fatal(err)
	}
	pnl, err := ledger.CreateAccount("Expense:Interest", "GBP", -2, 0)
	if err != nil {
		t.Fatal(err)
	}
	const nAccts = 200
	var accts []string
	for i := range nAccts {
		a, err := ledger.CreateAccount(fmt.Sprintf("Liability:Savings:c%d", i), "GBP", -2, 0)
		if err != nil {
			t.Fatal(err)
		}
		accts = append(accts, a.ID)
	}
	b := accrualBatch{valueTime: time.Date(2020, 3, 17, 23, 59, 58, 0, time.UTC)}
	want := make(map[string]int64)
	for i := range 4000 {
		to := accts[i%nAccts]
		b.inputs = append(b.inputs, luca.MovementInput{
			FromAccountID: pnl.ID, ToAccountID: to, Amount: luca.Amount(i%97 + 1), Code: codeDailyAccrual,
		})
		want[to] += int64(i%97 + 1)
	}

	if err := postMovements(ledger, b, 8, func(int) {}); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(`SELECT to_account_id, COUNT(*), SUM(amount) FROM movements GROUP BY to_account_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var total int
	for rows.Next() {
		var id string
		var n int
		var sum int64
		if err := rows.Scan(&id, &n, &sum); err != nil {
			t.Fatal(err)
		}
		total += n
		if sum != want[id] {
			t.Errorf("account %s sum %d, want %d", id, sum, want[id])
		}
	}
	if total != len(b.inputs) {
		t.Errorf("stored %d movements, want %d", total, len(b.inputs))
	}
}

// BenchmarkPostMovementsPostgres measures accrual posting throughput by
// worker count. Set GOBANK_PG_DSN to run.
func BenchmarkPostMovementsPostgres(b *testing.B) {
	dsn := os.Getenv("GOBANK_PG_DSN")
	if dsn == "" {
		b.Skip("GOBANK_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	dropAllPublicTables(db)
	ledger, err := luca.NewSQLLedger(db)
	if err != nil {
		b.Fatal(err)
	}
	pnl, _ := ledger.CreateAccount("Expense:Interest", "GBP", -2, 0)
	hold, _ := ledger.CreateAccount("Liability:AccruedInterest", "GBP", -2, 0)
	batch := accrualBatch{valueTime: time.Date(2020, 3, 17, 23, 59, 58, 0, time.UTC)}
	for range 20_000 {
		batch.inputs = append(batch.inputs, luca.MovementInput{
			FromAccountID: pnl.ID, ToAccountID: hold.ID, Amount: 1, Code: codeDailyAccrual,
		})
	}
	for _, w := range []int{1, 2, 4, 8} {
		b.Run(fmt.Sprintf("workers=%d", w), func(b *testing.B) {
			for range b.N {
				if err := postMovements(ledger, batch, w, func(int) {}); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(len(batch.inputs)*b.N)/b.Elapsed().Seconds(), "movements/s")
		})
	}
}
