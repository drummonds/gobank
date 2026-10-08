package staff

import (
	"context"
	"fmt"
	"math"
	"strings"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/core"
)

// The payments pages: the list and a payment's detail, read through the
// core.

func poundsToPence(pounds float64) luca.Amount {
	return luca.Amount(math.Round(pounds * 100))
}

// paymentTypeTag is the Bulma tag class for a payment type.
func paymentTypeTag(t core.PaymentType) string {
	switch t {
	case core.PaymentTransfer:
		return "is-link is-light"
	case core.PaymentDeposit:
		return "is-success is-light"
	case core.PaymentLoan:
		return "is-info is-light"
	default:
		return "is-light"
	}
}

// paymentStatusTag is the Bulma tag class for a payment status.
func paymentStatusTag(s core.PaymentStatus) string {
	switch s {
	case core.PaymentPending:
		return "is-warning"
	case core.PaymentProcessing:
		return "is-info"
	case core.PaymentCompleted:
		return "is-success"
	default:
		return "is-light"
	}
}

// paymentStatusReached reports whether a payment has reached a status in
// its lifecycle: pending, then processing, then completed.
func paymentStatusReached(have, want core.PaymentStatus) bool {
	rank := map[core.PaymentStatus]int{core.PaymentPending: 0, core.PaymentProcessing: 1, core.PaymentCompleted: 2}
	return rank[have] >= rank[want]
}

// partyLabel shows a customer ID, with the name when PII is authorised.
func partyLabel(q core.CustomerRegister, id string, piiAuth bool) string {
	if piiAuth {
		if name, err := q.CustomerName(context.Background(), id); err == nil && name != id {
			return fmt.Sprintf("%s (%s)", id, name)
		}
	}
	return id
}

// BuildPaymentsHTML renders the payments list as a Bulma HTML table.
// Shows customer IDs; names shown only when piiAuth is true. running is
// the generator's state, shown as a tag.
func BuildPaymentsHTML(q core.StaffQueries, piiAuth bool, page int, running bool) string {
	ctx := context.Background()
	pp, _ := q.PaymentPage(ctx, page)
	if pp.Page > pp.TotalPages() {
		pp, _ = q.PaymentPage(ctx, pp.TotalPages())
	}
	page, totalPages, total := pp.Page, pp.TotalPages(), pp.Total

	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Payments</h2>`)

	statusTag := `<span class="tag is-light">Stopped</span>`
	if running {
		statusTag = `<span class="tag is-warning">Auto-sending</span>`
	}
	s.WriteString(fmt.Sprintf(`<div class="field is-grouped is-grouped-multiline mb-4">
  <div class="control">%s</div>
  <div class="control"><span class="tag is-info is-light">%d payments</span></div>
</div>`, statusTag, total))

	if total == 0 {
		s.WriteString(`<p class="has-text-grey">No payments yet. Send one or start auto-generation.</p>`)
	} else {
		s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped is-hoverable">
<thead><tr>
  <th>ID</th><th>Type</th><th>From</th><th>To</th><th>Amount</th><th>Reference</th><th>Status</th><th>Time</th><th></th>
</tr></thead><tbody>`)

		for _, p := range pp.Payments {
			from := partyLabel(q, p.From, piiAuth)
			to := partyLabel(q, p.To, piiAuth)
			s.WriteString(fmt.Sprintf(`<tr>
  <td>%d</td><td><span class="tag %s">%s</span></td><td>%s</td><td>%s</td><td>%s</td><td><code>%s</code></td>
  <td><span class="tag %s">%s</span></td>
  <td>%s</td>
  <td><a href="payments/%d" class="button is-small is-link is-light">Detail</a></td>
</tr>`, p.ID, paymentTypeTag(p.Type), p.Type, from, to, FormatMoney(p.Amount), p.Reference, paymentStatusTag(p.Status), p.Status, FormatUTC(p.CreatedAt), p.ID))
		}

		s.WriteString(`</tbody></table></div>`)

		// Pagination
		if totalPages > 1 {
			s.WriteString(`<nav class="pagination is-small mt-4" role="navigation">`)
			if page > 1 {
				s.WriteString(fmt.Sprintf(`<a class="pagination-previous" href="payments?page=%d">Previous</a>`, page-1))
			} else {
				s.WriteString(`<a class="pagination-previous" disabled>Previous</a>`)
			}
			if page < totalPages {
				s.WriteString(fmt.Sprintf(`<a class="pagination-next" href="payments?page=%d">Next</a>`, page+1))
			} else {
				s.WriteString(`<a class="pagination-next" disabled>Next</a>`)
			}
			s.WriteString(fmt.Sprintf(`<span class="pagination-list">Page %d of %d</span>`, page, totalPages))
			s.WriteString(`</nav>`)
		}
	}

	return s.String()
}

// BuildPaymentDetailHTML renders a single payment detail with settlement timeline.
// Shows customer IDs; names shown only when piiAuth is true.
func BuildPaymentDetailHTML(q core.StaffQueries, id int, piiAuth bool) string {
	p, err := q.Payment(context.Background(), id)
	if err != nil {
		return `<div class="notification is-warning">Payment not found.</div>`
	}
	from := partyLabel(q, p.From, piiAuth)
	to := partyLabel(q, p.To, piiAuth)

	var s strings.Builder
	s.WriteString(fmt.Sprintf(`<h2 class="title is-4">Payment %s</h2>`, p.Reference))

	// Details box
	s.WriteString(`<div class="box">`)
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Type:</strong> <span class="tag %s">%s</span></div>`, paymentTypeTag(p.Type), p.Type))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>From:</strong> %s</div>`, from))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>To:</strong> %s</div>`, to))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Amount:</strong> %s</div>`, FormatMoney(p.Amount)))
	s.WriteString(`</div>`)
	s.WriteString(`<div class="columns">`)
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Status:</strong> <span class="tag %s">%s</span></div>`, paymentStatusTag(p.Status), p.Status))
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Created:</strong> %s</div>`, FormatUTC(p.CreatedAt)))
	settled := "—"
	if !p.SettledAt.IsZero() {
		settled = FormatUTC(p.SettledAt)
	}
	s.WriteString(fmt.Sprintf(`<div class="column"><strong>Settled:</strong> %s</div>`, settled))
	s.WriteString(`</div></div>`)

	// Settlement timeline SVG
	s.WriteString(buildTimelineSVG(p))

	return s.String()
}

func buildTimelineSVG(p core.Payment) string {
	var s strings.Builder

	s.WriteString(`<div class="box mt-4"><h3 class="title is-5">Settlement Timeline</h3>`)
	s.WriteString(`<svg viewBox="0 0 500 92" xmlns="http://www.w3.org/2000/svg" style="max-width:500px;width:100%;height:auto">`)
	s.WriteString(`<style>text{font-family:Arial,Helvetica,sans-serif}</style>`)

	// Line
	s.WriteString(`<line x1="50" y1="30" x2="450" y2="30" stroke="#dbdbdb" stroke-width="3"/>`)

	steps := []struct {
		X     int
		Label string
		Time  string
		Done  bool
	}{
		{50, "Pending", FormatUTC(p.CreatedAt), paymentStatusReached(p.Status, core.PaymentPending)},
		{250, "Processing", "", paymentStatusReached(p.Status, core.PaymentProcessing)},
		{450, "Completed", "", paymentStatusReached(p.Status, core.PaymentCompleted)},
	}

	if !p.SettledAt.IsZero() {
		steps[2].Time = FormatUTC(p.SettledAt)
	}

	for _, st := range steps {
		color := "#dbdbdb"
		if st.Done {
			color = "#48c78e"
		}
		if st.Done && st.X > 50 {
			prevX := 50
			if st.X == 450 {
				prevX = 250
			}
			s.WriteString(fmt.Sprintf(`<line x1="%d" y1="30" x2="%d" y2="30" stroke="#48c78e" stroke-width="3"/>`, prevX, st.X))
		}
		s.WriteString(fmt.Sprintf(`<circle cx="%d" cy="30" r="10" fill="%s"/>`, st.X, color))
		if st.Done {
			s.WriteString(fmt.Sprintf(`<text x="%d" y="34" text-anchor="middle" font-size="11" fill="#fff" font-weight="bold">&#10003;</text>`, st.X))
		}
		s.WriteString(fmt.Sprintf(`<text x="%d" y="60" text-anchor="middle" font-size="11" fill="#363636">%s</text>`, st.X, st.Label))
		if st.Time != "" { // the UTC datetime, date over time
			date, clock, _ := strings.Cut(st.Time, " ")
			s.WriteString(fmt.Sprintf(`<text x="%d" y="74" text-anchor="middle" font-size="9" fill="#7a7a7a">%s</text>`, st.X, date))
			s.WriteString(fmt.Sprintf(`<text x="%d" y="86" text-anchor="middle" font-size="9" fill="#7a7a7a">%s</text>`, st.X, clock))
		}
	}

	s.WriteString(`</svg></div>`)
	return s.String()
}
