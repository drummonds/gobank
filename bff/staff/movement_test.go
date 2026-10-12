package staff

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/gobank/internal/view"
)

// Savings interest as the contract writes it (ADR-0008): a negative
// amount from the expense to the account.
func interestMovement() core.Movement {
	return core.Movement{
		ID:     "7f3a",
		From:   core.Leg{AccountID: "e1", Path: "Expense:Interest", Type: "Expense", Balance: 1_27},
		To:     core.Leg{AccountID: "a1", Path: "Liability:Savings:cust-001:easy-access", Type: "Liability", Balance: -1001_27},
		Amount: -1_27, Currency: "GBP", Code: "LDAS:FTDP:ACRU",
		ValueTime:     time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC),
		KnowledgeTime: time.Date(2026, 2, 1, 0, 0, 4, 0, time.UTC),
		Description:   "Interest applied for month ending 2026-01-31",
	}
}

// A movement as goluca: the balances before as directives, then the
// signed movement from one account to the other as written, paths quoted
// for their hyphens.
func TestMovementGoluca(t *testing.T) {
	got := MovementGoluca(interestMovement())
	want := `# Movement 7f3a: Interest applied for month ending 2026-01-31
# code LDAS:FTDP:ACRU, value 2026-01-31T23:59:59Z, known 2026-02-01T00:00:04Z

2026-01-31 balance "Expense:Interest" 0.00 GBP
2026-01-31 balance "Liability:Savings:cust-001:easy-access" -1000.00 GBP

2026-01-31 * Interest applied for month ending 2026-01-31
  "Expense:Interest" -> "Liability:Savings:cust-001:easy-access" "LDAS:FTDP:ACRU" -1.27 GBP
`
	if got != want {
		t.Errorf("goluca:\n%s\nwant:\n%s", got, want)
	}
}

// pta2svg draws the movement with both accounts at the balances they are
// left with, the account it is from on the left of the account it is to,
// so the arrow reads with the movement.
func TestMovementSVG(t *testing.T) {
	svg, err := MovementSVG(interestMovement())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<svg", "-1001.27", "-1.27", "easy-access", "Interest"} {
		if !strings.Contains(svg, want) {
			t.Errorf("svg missing %q", want)
		}
	}
	if from, to := strings.Index(svg, ">Interest<"), strings.Index(svg, ">easy-access<"); from < 0 || to < 0 || nodeX(svg, from) >= nodeX(svg, to) {
		t.Errorf("the expense is not drawn left of the savings account: from at %d, to at %d", nodeX(svg, from), nodeX(svg, to))
	}
}

// nodeX is the x translation of the node whose label starts at i.
func nodeX(svg string, i int) int {
	const mark = `class="node" transform="translate(`
	start := strings.LastIndex(svg[:i], mark)
	if start < 0 {
		return -1
	}
	var x, y float64
	if _, err := fmt.Sscanf(svg[start+len(mark):], "%f,%f)", &x, &y); err != nil {
		return -1
	}
	return int(x)
}

// The page carries the movement's two accounts and signed amount, the
// drawing and the goluca text with its links; in the customer view it says the movement stays in the
// bank's; with the WASM asset served it loads the browser renderer.
func TestBuildMovementHTML(t *testing.T) {
	page := BuildMovementHTML(interestMovement(), view.Bank, false)
	for _, want := range []string{"From", "To", "Liability:Savings:cust-001:easy-access", "Expense:Interest", "-£1,001.27", "-£1.27",
		"LDAS:FTDP:ACRU", "<svg", `id="movement-goluca"`, `href="movements/7f3a.goluca"`, `href="movements/7f3a.json"`, "by the BFF"} {
		if !strings.Contains(page, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if strings.Contains(page, "wasm_exec.js") {
		t.Error("page loads the WASM renderer with no asset served")
	}
	customer := BuildMovementHTML(interestMovement(), view.Customer, true)
	for _, want := range []string{"customer view", "wasm/wasm_exec.js", "wasm/pta2svg.wasm", "pta2svg.render("} {
		if !strings.Contains(customer, want) {
			t.Errorf("customer-view page with WASM missing %q", want)
		}
	}
}
