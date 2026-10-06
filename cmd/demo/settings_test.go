package main

import (
	"strings"
	"testing"
	"time"
)

func TestResetKeepsConsoleSettings(t *testing.T) {
	ds := NewDemoState()
	ds.UpdateSettings(50)
	ds.SetDayLength(2 * time.Hour)
	ds.Reset()
	if got := ds.Settings(); got.MaxCustomers != 50 || got.DayLength != 2*time.Hour {
		t.Errorf("console settings after Reset = %+v; want max 50, day 2h", got)
	}
}

// The settings form is never re-rendered by polling, or a value being typed
// is wiped before Save. Only the status line polls, and only when asked.
func TestSettingsFormIsStaticWhileStatusPolls(t *testing.T) {
	ds := NewDemoState()
	bank := newCoreAdapter(ds, "")

	polled := buildSettingsHTML(bank, ds.Settings(), true, nil)
	found := strings.Contains(polled, `id="settings-status"`)
	hx := strings.Index(polled, `hx-get="/settings/status"`)
	form := strings.Index(polled, `<form`)
	if !found || hx < 0 {
		t.Fatalf("polled page should have a status section that polls /settings/status:\n%s", polled)
	}
	if !(hx < form) || strings.Count(polled, "hx-") != 3 { // hx-get, hx-trigger, hx-swap on the status div only
		t.Errorf("only the status section may carry HTMX attributes; form must be static")
	}
	if !strings.Contains(polled, `name="day_length"`) {
		t.Errorf("form missing the day length field")
	}

	still := buildSettingsHTML(bank, ds.Settings(), false, nil)
	if strings.Contains(still, "hx-") {
		t.Errorf("a page that is not polling should carry no HTMX attributes")
	}
	if !strings.Contains(renderSettingsStatus(bank, true), `hx-get="/settings/status"`) {
		t.Errorf("the status fragment served to the poll must keep polling")
	}
}
