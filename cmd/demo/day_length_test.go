package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseDayLength(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"0", 0},
		{"2h", 2 * time.Hour},
		{"90m", 90 * time.Minute},
		{"1h30m", 90 * time.Minute},
	}
	for _, c := range cases {
		got, err := parseDayLength(c.in)
		if err != nil || got != c.want {
			t.Errorf("parseDayLength(%q) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"fast", "-1h", "2 hours"} {
		if _, err := parseDayLength(bad); err == nil {
			t.Errorf("parseDayLength(%q) should fail", bad)
		}
	}
}

// nextDayDelay is the wait after a day completes, given the day length and
// how long the day's work took.
func TestNextDayDelay(t *testing.T) {
	cases := []struct {
		name      string
		dayLength time.Duration
		elapsed   time.Duration
		want      time.Duration
	}{
		{"flat out", 0, 50 * time.Millisecond, minDayGap},
		{"two hour day, work took a minute", 2 * time.Hour, time.Minute, 2*time.Hour - time.Minute},
		{"work took longer than the day", time.Second, 2 * time.Second, minDayGap},
		{"work nearly filled the day", time.Second, time.Second - time.Millisecond, minDayGap},
	}
	for _, c := range cases {
		if got := nextDayDelay(c.dayLength, c.elapsed); got != c.want {
			t.Errorf("%s: nextDayDelay(%v, %v) = %v; want %v", c.name, c.dayLength, c.elapsed, got, c.want)
		}
	}
}

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

	page := buildSettingsHTML(newCoreAdapter(ds, ""), ds.Settings(), false)
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
