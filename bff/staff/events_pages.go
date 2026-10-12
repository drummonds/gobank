package staff

import (
	"context"
	"fmt"
	"html"
	"strings"

	"git.bytestone.uk/hum3/gobank/core"
)

// The events page (ADR-0011): the latest root events, newest first, each
// with its status and what its cascade has written so far, so staff see
// the bank's work by what caused it.

// BuildEventsHTML renders the events page from the record.
func BuildEventsHTML(q core.EventQueries) string {
	cascades, err := q.Cascades(context.Background())
	var s strings.Builder
	s.WriteString(`<h2 class="title is-4">Events</h2>`)
	s.WriteString(`<p class="subtitle is-6 has-text-grey">What happened and what it caused: a root event is pending while the bank works through its cascade and done when the last effect is on record (ADR-0011).</p>`)
	if err != nil {
		fmt.Fprintf(&s, `<div class="notification is-danger">%s</div>`, html.EscapeString(err.Error()))
		return s.String()
	}
	if len(cascades) == 0 {
		s.WriteString(`<p class="has-text-grey">Nothing has happened yet.</p>`)
		return s.String()
	}
	s.WriteString(`<table class="table is-narrow is-fullwidth is-striped"><thead><tr><th>At (UTC)</th><th>Event</th><th>Raised by</th><th>Status</th><th class="has-text-right">Postings</th><th class="has-text-right">Settings</th><th class="has-text-right">Payments</th></tr></thead><tbody>`)
	for _, c := range cascades {
		fmt.Fprintf(&s, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td class="has-text-right">%d</td><td class="has-text-right">%d</td><td class="has-text-right">%d</td></tr>`,
			FormatUTC(c.Event.At), html.EscapeString(c.Event.Kind), html.EscapeString(c.Event.By), eventStatusTag(c.Event), c.Postings, c.Settings, c.Payments)
	}
	s.WriteString(`</tbody></table>`)
	return s.String()
}

// eventStatusTag is an event's status as a tag: pending in warning, done
// in success with when.
func eventStatusTag(e core.EventRecord) string {
	if e.Status == "done" {
		return fmt.Sprintf(`<span class="tag is-success is-light" title="done %s">done</span>`, FormatUTC(e.DoneAt))
	}
	return fmt.Sprintf(`<span class="tag is-warning">%s</span>`, html.EscapeString(e.Status))
}

// describeCause is a movement's cause in words, for the movement page:
// the event, when, who raised it and its status, with the step.
func describeCause(m core.Movement) string {
	if m.Cause == nil {
		return ""
	}
	return fmt.Sprintf(`<p><strong>Cause:</strong> <a href="events">%s</a> at %s, raised by %s, %s &middot; step <code>%s</code></p>`,
		html.EscapeString(m.Cause.Kind), FormatUTC(m.Cause.At), html.EscapeString(m.Cause.By), eventStatusTag(*m.Cause), html.EscapeString(m.Step))
}
