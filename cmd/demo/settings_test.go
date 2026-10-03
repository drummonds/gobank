package main

import (
	"strings"
	"testing"
	"time"
)

// The console settings are a value behind Get and Update: a reader gets a
// snapshot it can keep, a writer changes the fields it names, and no caller
// sees the lock.
func TestSimSettingsGetIsASnapshot(t *testing.T) {
	var s simSettings
	s.Update(func(v *Settings) { *v = DefaultSettings() })
	snap := s.Get()
	s.Update(func(v *Settings) { v.MaxCustomers = 7 })
	if snap.MaxCustomers != DefaultSettings().MaxCustomers {
		t.Errorf("snapshot changed under the reader: %+v", snap)
	}
	if got := s.Get(); got.MaxCustomers != 7 || got.DayLength != 0 {
		t.Errorf("Update should change only the field it names, got %+v", got)
	}
}

// Reset clears the bank's run, not what the operator set on the console.
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
