package view

import (
	"testing"

	"git.bytestone.uk/hum3/gobank/core"
)

// The core speaks the bank's view (ADR-0008); the customer's view negates
// savings so a holding reads positive, leaves loans as they are, and
// states the customer's net position with the bank in the customer's sign.
func TestViews(t *testing.T) {
	savings := core.Account{Family: "Savings", Balance: -1000_00, Interest: -1_27, AccruedE7: -410959}
	loan := core.Account{Family: "Lending", Balance: 800_00, Interest: 4_69, AccruedE7: 1606849}
	cases := []struct {
		view                           View
		savingsBal, loanBal, net, sInt int64
		sAccrued                       int64
	}{
		{Bank, -1000_00, 800_00, -200_00, -1_27, -410959},
		{Customer, 1000_00, 800_00, 200_00, 1_27, 410959},
	}
	for _, c := range cases {
		if got := c.view.Balance(savings); int64(got) != c.savingsBal {
			t.Errorf("%s savings balance %d, want %d", c.view, got, c.savingsBal)
		}
		if got := c.view.Balance(loan); int64(got) != c.loanBal {
			t.Errorf("%s loan balance %d, want %d", c.view, got, c.loanBal)
		}
		if got := c.view.NetPosition([]core.Account{savings, loan}); int64(got) != c.net {
			t.Errorf("%s net position %d, want %d", c.view, got, c.net)
		}
		if got := c.view.Interest(savings); int64(got) != c.sInt {
			t.Errorf("%s savings interest %d, want %d", c.view, got, c.sInt)
		}
		if got := c.view.AccruedE7(savings); got != c.sAccrued {
			t.Errorf("%s savings accrued %d, want %d", c.view, got, c.sAccrued)
		}
	}
	tx := core.Transaction{Family: "Savings", Balance: -50_00}
	if Customer.Running(tx) != 50_00 || Bank.Running(tx) != -50_00 {
		t.Errorf("running balances %d, %d", Customer.Running(tx), Bank.Running(tx))
	}
	if Parse("customer") != Customer || Parse("bank") != Bank || Parse("") != Bank || Parse("x") != Bank {
		t.Error("Parse")
	}
	if Bank.Other() != Customer || Customer.Other() != Bank || Bank.Label() != "Bank view" {
		t.Error("Other or Label")
	}
}
