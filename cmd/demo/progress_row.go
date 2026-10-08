package main

import (
	"fmt"
	"git.bytestone.uk/hum3/gobank/bff/staff"
	"math"
	"strconv"
	"time"

	"git.bytestone.uk/hum3/gobank/bank"
)

// progressRow renders the day in progress as a row of the runtime page's
// Simulation table, or "" before the first day has run.
func progressRow(s bank.DayProgress) string {
	switch {
	case s.Active:
		work := s.Phase
		if s.Total > 0 {
			work += fmt.Sprintf(": %s / %s (%d%%) at %s/s", groupInt(s.Done), groupInt(s.Total), s.Done*100/s.Total, groupInt(int(math.Round(s.Rate))))
		}
		return fmt.Sprintf(`<tr><th>Day in progress</th><td>%s — %s</td><td class="has-text-grey">%s elapsed</td></tr>`,
			s.Day.Format("2 Jan 2006"), work, s.Elapsed.Round(time.Second))
	case !s.LastDay.IsZero():
		return fmt.Sprintf(`<tr><th>Last day</th><td>%s: %s accounts in %s</td></tr>`,
			s.LastDay.Format("2 Jan 2006"), groupInt(s.LastAccounts), s.LastDuration.Round(time.Second))
	}
	return ""
}

func groupInt(n int) string { return staff.GroupThousands(strconv.Itoa(n)) }
