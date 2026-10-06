package main

import (
	"strings"
	"testing"
	"time"
)

func TestDayLengthSetting(t *testing.T) {
	ds := NewDemoState()
	if got := ds.SimStatus().DayLength; got != 0 {
		t.Fatalf("default day length = %v; want 0 (flat out)", got)
	}
	ds.SetDayLength(2 * time.Hour)
	if got := ds.SimStatus().DayLength; got != 2*time.Hour {
		t.Errorf("after SetDayLength(2h): %v", got)
	}
	ds.SetDayLength(-time.Hour) // refused, not applied
	if got := ds.SimStatus().DayLength; got != 2*time.Hour {
		t.Errorf("negative day length should be ignored, got %v", got)
	}

	page := buildSettingsHTML(newCoreAdapter(ds, ""), ds.Settings(), false, nil)
	if !strings.Contains(page, `name="day_length"`) || !strings.Contains(page, `value="2h0m0s"`) {
		t.Errorf("settings page should show the day length field with the current value:\n%s", page)
	}
}

func TestDashboardShowsDayLength(t *testing.T) {
	ds := NewDemoState()
	bank := newCoreAdapter(ds, "")
	if html := buildDashboardHTML(bank, ds); strings.Contains(html, "Day length") {
		t.Errorf("flat-out simulation should not show a day length")
	}
	ds.SetDayLength(2 * time.Hour)
	if html := buildDashboardHTML(bank, ds); !strings.Contains(html, "Day length") || !strings.Contains(html, "2h0m0s") {
		t.Errorf("dashboard should show the day length when set")
	}
}
