package treasury

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "git.bytestone.uk/hum3/go-postgres"
	"git.bytestone.uk/hum3/gobank/core"
)

func open(t *testing.T) *Treasury {
	t.Helper()
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, m := range Schema.Migrations {
		for _, stmt := range m.Statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("schema v%d: %v", m.Version, err)
			}
		}
	}
	day := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	return New(db, func() time.Time { return day })
}

// The opening yield curve is quoted as soon as the schema exists.
func TestYieldsAreQuotedFromTheStart(t *testing.T) {
	tr := open(t)
	yields, err := tr.Yields(context.Background())
	if err != nil || len(yields) == 0 {
		t.Fatalf("Yields = %v, %v", yields, err)
	}
	for _, y := range yields {
		if y.Tenor == "" || y.Rate <= 0 {
			t.Errorf("yield %+v needs a tenor and a positive rate", y)
		}
	}
}

// A purchase is recorded at the quoted yield on the bank's business day;
// a refused purchase records nothing.
func TestBuyRecordsAHoldingOnTheBusinessDay(t *testing.T) {
	tr := open(t)
	ctx := context.Background()
	yields, _ := tr.Yields(ctx)
	tenor, rate := yields[0].Tenor, yields[0].Rate
	if err := tr.Buy(ctx, tenor, core.MinGiltPurchase); err != nil {
		t.Fatal(err)
	}
	holdings, err := tr.Holdings(ctx)
	if err != nil || len(holdings) != 1 {
		t.Fatalf("Holdings = %v, %v; want one", holdings, err)
	}
	h := holdings[0]
	want := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if h.Tenor != tenor || h.FaceValue != core.MinGiltPurchase || h.Yield != rate || !h.PurchaseDate.Equal(want) {
		t.Errorf("holding %+v, want %s for %d at %v on %s", h, tenor, core.MinGiltPurchase, rate, want.Format(time.DateOnly))
	}
	if err := tr.Buy(ctx, "no-such-tenor", core.MinGiltPurchase); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("unknown tenor: err = %v, want ErrNotFound", err)
	}
	if err := tr.Buy(ctx, tenor, core.MinGiltPurchase-1); !errors.Is(err, core.ErrInvalidAmount) {
		t.Errorf("below the minimum: err = %v, want ErrInvalidAmount", err)
	}
	if again, _ := tr.Holdings(ctx); len(again) != 1 {
		t.Errorf("a refused purchase added a holding: %d", len(again))
	}
}

// The schema is the component's own: versions in order from 1.
func TestSchemaVersionsRunFromOne(t *testing.T) {
	if Schema.Name != "treasury" {
		t.Errorf("Schema.Name = %q", Schema.Name)
	}
	for i, m := range Schema.Migrations {
		if m.Version != i+1 {
			t.Errorf("migration %d listed where version %d is due", m.Version, i+1)
		}
	}
}
