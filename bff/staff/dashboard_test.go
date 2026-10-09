package staff

import (
	"strings"
	"testing"
	"time"

	"git.bytestone.uk/hum3/gobank/core"
)

// The dashboard says how far the general ledger is posted and whether
// its last close reconciled (ADR-0005): the day, "reconciled", or the
// breaks, never hidden.
func TestDashboardReportsTheGeneralLedger(t *testing.T) {
	day := time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		gl   core.GeneralLedger
		want []string
		not  []string
	}{
		{core.GeneralLedger{}, []string{"General ledger", "not yet closed"}, []string{"reconciled", "break"}},
		{core.GeneralLedger{PostedThrough: day}, []string{"General ledger", "3 Jan 2020", "reconciled"}, []string{"break", "is-danger"}},
		{core.GeneralLedger{PostedThrough: day, Breaks: 1}, []string{"3 Jan 2020", "1 break", "is-danger"}, []string{"reconciled", "breaks"}},
		{core.GeneralLedger{PostedThrough: day, Breaks: 2}, []string{"3 Jan 2020", "2 breaks", "is-danger"}, []string{"reconciled"}},
	} {
		html := BuildDashboardHTML(DashData{Bank: core.Position{GL: c.gl}})
		for _, w := range c.want {
			if !strings.Contains(html, w) {
				t.Errorf("GL %+v: dashboard lacks %q", c.gl, w)
			}
		}
		for _, n := range c.not {
			if strings.Contains(html, n) {
				t.Errorf("GL %+v: dashboard shows %q", c.gl, n)
			}
		}
	}
}
