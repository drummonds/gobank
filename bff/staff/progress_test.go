package staff

import (
	"strings"
	"testing"
	"time"
)

func TestRuntimeShowsDayInProgress(t *testing.T) {
	html := progressRow(DayProgress{
		Active: true, Day: time.Date(2020, 2, 29, 0, 0, 0, 0, time.UTC), Phase: "projecting positions",
		Done: 404763, Total: 955000, Elapsed: 5 * time.Minute, Rate: 404763 / (5 * 60.0),
	})
	for _, want := range []string{"Day in progress", "29 Feb 2020", "projecting positions", "404,763 / 955,000", "42%", "1,349/s", "5m0s"} {
		if !strings.Contains(html, want) {
			t.Errorf("runtime page missing %q", want)
		}
	}
}

func TestRuntimeShowsLastDayWhenIdle(t *testing.T) {
	html := progressRow(DayProgress{
		LastDay: time.Date(2020, 2, 28, 0, 0, 0, 0, time.UTC), LastDuration: 341 * time.Second, LastAccounts: 477634,
	})
	if strings.Contains(html, "Day in progress") {
		t.Error("idle simulation should not claim a day in progress")
	}
	for _, want := range []string{"Last day", "28 Feb 2020", "477,634 accounts", "5m41s"} {
		if !strings.Contains(html, want) {
			t.Errorf("runtime page missing %q", want)
		}
	}
}
