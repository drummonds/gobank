package staff

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/gobank/internal/view"
)

type cascades []core.Cascade

func (c cascades) Cascades(context.Context) ([]core.Cascade, error) { return c, nil }

type noRecord struct{}

func (noRecord) Cascades(context.Context) ([]core.Cascade, error) {
	return nil, errors.New("no record")
}

// The events page lists the roots newest first with their status and
// what each cascade wrote; a pending root is marked as such.
func TestBuildEventsHTML(t *testing.T) {
	noon := time.Date(2020, 3, 19, 12, 0, 0, 0, time.UTC)
	page := BuildEventsHTML(cascades{
		{Event: core.EventRecord{ID: "e2", Kind: "publication", At: noon, Status: "pending", By: "Bank of England"}, Settings: 1},
		{Event: core.EventRecord{ID: "e1", Kind: "start of day", At: noon.Add(-12 * time.Hour), Status: "done", By: "clock", RaisedAt: noon.Add(time.Hour), DoneAt: noon.Add(time.Hour + 1500*time.Millisecond)}, Postings: 12},
	})
	// Each root shows its instant in the bank's time and the wall time it
	// was raised at, and a done root how long its cascade took: in a
	// simulation the two clocks run apart, so this is how fast the bank
	// works through its events.
	for _, want := range []string{"publication", "Bank of England", `is-warning">pending`, "start of day", "clock", `>done<`, "<td class=\"has-text-right\">12</td>",
		"<th>Bank time", "<th>Wall time", "2020-03-19 00:00:00Z", "2020-03-19 13:00:00Z", "1.5s"} {
		if !strings.Contains(page, want) {
			t.Errorf("page missing %q:\n%s", want, page)
		}
	}
	if i, j := strings.Index(page, "publication"), strings.Index(page, "start of day"); i > j {
		t.Error("the newer event is not first")
	}
	if empty := BuildEventsHTML(cascades{}); !strings.Contains(empty, "Nothing has happened") {
		t.Errorf("an empty record: %s", empty)
	}
	if failed := BuildEventsHTML(noRecord{}); !strings.Contains(failed, "no record") {
		t.Errorf("a failed read: %s", failed)
	}
}

// A rule's movement names its cause on the movement page; a movement
// that is not a rule's names none.
func TestMovementPageNamesItsCause(t *testing.T) {
	m := interestMovement()
	m.Cause = &core.EventRecord{ID: "e1", Kind: "start of day", At: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), Status: "done", By: "clock", DoneAt: time.Date(2026, 2, 1, 0, 0, 5, 0, time.UTC)}
	m.Step = "apply"
	page := BuildMovementHTML(m, view.Bank, false)
	for _, want := range []string{"Cause:", `href="events"`, "start of day", "clock", "<code>apply</code>"} {
		if !strings.Contains(page, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if plain := BuildMovementHTML(interestMovement(), view.Bank, false); strings.Contains(plain, "Cause:") {
		t.Error("a movement with no cause names one")
	}
}
