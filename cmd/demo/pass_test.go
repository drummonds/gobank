package main

import (
	"context"
	"git.bytestone.uk/hum3/gobank/bank/customers"
	"testing"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	gbp "git.bytestone.uk/hum3/gobank-products"
	"git.bytestone.uk/hum3/gobank/core"
)

// The start-of-day workflow (ADR-0002 stage 3, story e): the date advances
// at the start of the slot and the pass projects every account's position
// for the day. Interest accrues on the closing balance, so the projection
// the pass writes is provisional: an event that moves the balance later in
// the day rewrites it, with the accrual recomputed on the new balance.
func TestEventRewritesTheProvisionalAccrual(t *testing.T) {
	ds := NewDemoState()
	twoFundedCustomers(ds)
	ds.AdvanceDay() // the pass has projected every account for today
	day := ds.position().Day

	from, _ := ds.customerByID("cust-001")
	to, _ := ds.customerByID("cust-002")
	fromAcc, toAcc := customers.FirstSavings(from.Accounts), customers.FirstSavings(to.Accounts)
	if fromAcc == nil || toAcc == nil || fromAcc.Balance < 1000 {
		t.Fatalf("need two funded savings accounts, got %+v and %+v", fromAcc, toAcc)
	}
	if _, err := ds.transfer(core.Transfer{From: "cust-001", To: "cust-002", Amount: 1000}); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		acc   *customers.Account
		delta luca.Amount
	}{{fromAcc, -1000}, {toAcc, 1000}} {
		prev, err := ds.Ledger().PositionAt(c.acc.LedgerAccountID, day.AddDate(0, 0, -1))
		if err != nil || prev == nil {
			t.Fatalf("%s: yesterday's position: %v %v", c.acc.ProductName, prev, err)
		}
		now, err := ds.Ledger().PositionAt(c.acc.LedgerAccountID, day)
		if err != nil || now == nil {
			t.Fatalf("%s: today's position: %v %v", c.acc.ProductName, now, err)
		}
		wantBalance := c.acc.Balance + c.delta
		wantAccrued := prev.Accrued.Num + int64(wantBalance)*gbp.RateBps(c.acc.Rate)
		if !now.Day.Equal(day) || now.Balance != wantBalance || now.Accrued.Num != wantAccrued || now.Accrued.Den != gbp.AccrualDenominator {
			t.Errorf("%s: today's position after the transfer = %s %d accrued %d/%d; want %s %d accrued %d/%d (yesterday's %d plus the new balance's day)",
				c.acc.ProductName, now.Day.Format("2006-01-02"), now.Balance, now.Accrued.Num, now.Accrued.Den,
				day.Format("2006-01-02"), wantBalance, wantAccrued, gbp.AccrualDenominator, prev.Accrued.Num)
		}
	}
}

// A restart in the middle of the pass resumes it from the projections
// already written: the day does not advance again, the accounts not yet
// visited are projected, and the application the pass books for the day
// before is booked once, however many times an account is visited.
func TestPassResumesFromProjectionsAfterRestart(t *testing.T) {
	first := NewDemoState()
	for range 3 {
		first.createCustomer()
	}
	for range 30 { // 2 Jan to 31 Jan
		first.AdvanceDay()
	}
	feb1 := time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC)

	// 1 Feb's pass books January's interest; the process dies two
	// accounts in.
	ctx, cancel := context.WithCancel(context.Background())
	visited := 0
	first.SetPassHook(func() {
		if visited++; visited == 2 {
			cancel()
		}
	})
	first.nextDayCtx(ctx)
	if p := first.position(); !p.Day.Equal(feb1) || p.DayCount != 31 {
		t.Fatalf("after the interrupted day: %s (%d), want 1 Feb (31)", p.Day.Format("2006-01-02"), p.DayCount)
	}
	if pending, _ := anyUnprojected(first, feb1); !pending {
		t.Fatal("the interrupted pass left no account unprojected; the test would be vacuous")
	}

	// The next process, same database.
	second := newDemoStateOn(first.db, "")
	if p := second.position(); !p.Day.Equal(feb1) || p.DayCount != 31 {
		t.Fatalf("resumed position: %s (%d), want 1 Feb (31)", p.Day.Format("2006-01-02"), p.DayCount)
	}
	second.AdvanceDay() // finishes 1 Feb rather than starting 2 Feb
	if p := second.position(); !p.Day.Equal(feb1) || p.DayCount != 31 {
		t.Fatalf("after finishing the day: %s (%d), want still 1 Feb (31)", p.Day.Format("2006-01-02"), p.DayCount)
	}
	if pending, err := anyUnprojected(second, feb1); pending || err != nil {
		t.Fatalf("accounts still unprojected after the resumed pass (err %v)", err)
	}

	jan31 := feb1.AddDate(0, 0, -1)
	appliedAt := time.Date(2020, 1, 31, 23, 59, 59, 0, time.UTC)
	for _, c := range allCustomers(t, second) {
		for _, a := range c.Accounts {
			prev, err := second.Ledger().PositionAt(a.LedgerAccountID, jan31)
			if err != nil || prev == nil || !prev.Day.Equal(jan31) {
				t.Fatalf("%s %s: 31 Jan position: %v %v", c.ID, a.ProductName, prev, err)
			}
			now, err := second.Ledger().PositionAt(a.LedgerAccountID, feb1)
			if err != nil || now == nil || !now.Day.Equal(feb1) {
				t.Fatalf("%s %s: 1 Feb position: %v %v", c.ID, a.ProductName, now, err)
			}
			if prev.Accrued.Num/gbp.AccrualDenominator != 0 {
				t.Errorf("%s %s: 31 Jan still holds %d/%d unapplied after the pass", c.ID, a.ProductName, prev.Accrued.Num, prev.Accrued.Den)
			}
			if want := prev.Accrued.Num + int64(now.Balance)*gbp.RateBps(a.Rate); now.Accrued.Num != want {
				t.Errorf("%s %s: 1 Feb accrued %d, want %d", c.ID, a.ProductName, now.Accrued.Num, want)
			}
			var applications int
			if err := second.db.QueryRow(`SELECT COUNT(*) FROM movements WHERE to_account_id = $1 AND code = $2 AND value_time = $3`,
				a.LedgerAccountID, luca.CodeInterestAccrual, appliedAt).Scan(&applications); err != nil {
				t.Fatal(err)
			}
			want := 0
			if a.Balance > 0 && a.Rate > 0 {
				want = 1
			}
			if applications != want {
				t.Errorf("%s %s: %d January applications booked, want %d", c.ID, a.ProductName, applications, want)
			}
		}
	}

	second.AdvanceDay()
	if p := second.position(); !p.Day.Equal(feb1.AddDate(0, 0, 1)) || p.DayCount != 32 {
		t.Errorf("the day after: %s (%d), want 2 Feb (32)", p.Day.Format("2006-01-02"), p.DayCount)
	}
}

// The pass runs at the start of the day at the system's capacity whatever
// the day length: the day length is headroom, not a pace, and the rest of
// the day is idle. A real bank's overnight run is the same: done as early
// as the machine allows, with the night as the margin.
func TestPassRunsFlatOutWhateverTheDayLength(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	ds.SetDayLength(2 * time.Hour)
	ds.Start()
	defer ds.Stop()

	deadline := time.Now().Add(3 * time.Second)
	for ds.Progress().LastDay.IsZero() {
		if time.Now().After(deadline) {
			t.Fatalf("the first day's pass did not finish within 3s of a 2h day: %+v", ds.Progress())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if left := ds.SimStatus().DayEndsIn; left < 2*time.Hour-time.Minute {
		t.Errorf("the pass finished with %v of the day left; want nearly all of it", left)
	}
	if p := ds.position(); p.DayCount != 1 {
		t.Errorf("day count %d after the first day; want 1: the loop idles for the rest of the day", p.DayCount)
	}
}

// Setting the day length to zero while the loop idles ends the wait: the
// next day begins at once, and the console's new length applies to the
// day in progress rather than the one after.
func TestDayLengthToZeroEndsTheIdleWait(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	ds.SetDayLength(time.Hour)
	ds.Start()
	defer ds.Stop()

	deadline := time.Now().Add(3 * time.Second)
	for ds.position().DayCount < 1 {
		if time.Now().After(deadline) {
			t.Fatal("the first day did not run")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if left := ds.SimStatus().DayEndsIn; left < 59*time.Minute {
		t.Fatalf("an hour-long day should have most of itself left, has %v", left)
	}

	ds.SetDayLength(0)

	deadline = time.Now().Add(3 * time.Second)
	for ds.position().DayCount < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("flat out from an idle day: day count %d after 3s, want days to follow at once", ds.position().DayCount)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if left := ds.SimStatus().DayEndsIn; left != 0 {
		t.Errorf("flat out has no end of day to count down to, got %v", left)
	}
}
