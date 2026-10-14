package staff

import (
	"context"
	"fmt"
	"html"
	"strings"

	"git.bytestone.uk/hum3/gobank/core"
)

// The customer search: a name posted from the Customers page, the
// matches rendered as the register's table. The name is personal data,
// so it travels in the body of a POST and never in a URL, and the search
// is offered and answered only once personal data is authorised.

// searchForm is the search box on the Customers page, with the slot the
// matches swap into.
const searchForm = `<form class="field has-addons mb-4" hx-post="customers/search" hx-target="#customer-search-results" hx-swap="outerHTML" action="customers/search" method="post">
  <div class="control"><input class="input" type="search" name="name" placeholder="Find by name" autocomplete="off"></div>
  <div class="control"><button class="button is-link" type="submit">Search</button></div>
</form>
<div id="customer-search-results"></div>`

// BuildCustomerSearchHTML renders the customers whose name contains
// name: the slot the Customers page's form swaps the matches into.
func BuildCustomerSearchHTML(q core.CustomerRegister, name string) string {
	ctx := context.Background()
	name = strings.TrimSpace(name)
	var s strings.Builder
	s.WriteString(`<div id="customer-search-results">`)
	if name != "" {
		found, err := q.CustomersByName(ctx, name)
		switch {
		case err != nil:
			s.WriteString(`<p class="has-text-danger">The search failed: ` + html.EscapeString(err.Error()) + `</p>`)
		case len(found) == 0:
			s.WriteString(`<p class="has-text-grey">No customer's name contains “` + html.EscapeString(name) + `”.</p>`)
		default:
			more := ""
			if len(found) >= core.CustomerSearchLimit {
				more = fmt.Sprintf(" — the first %d; give more of the name to narrow it", core.CustomerSearchLimit)
			}
			s.WriteString(fmt.Sprintf(`<p class="is-size-7 mb-2">%d whose name contains “%s”%s</p>`, len(found), html.EscapeString(name), more))
			s.WriteString(customersTable(ctx, q, found, true))
		}
	}
	s.WriteString(`</div>`)
	return s.String()
}

// customersTable is the register's table of the customers given, with
// names when personal data is authorised.
func customersTable(ctx context.Context, q core.CustomerRegister, customers []core.CustomerSummary, piiAuth bool) string {
	var s strings.Builder
	s.WriteString(`<div class="table-container"><table class="table is-fullwidth is-striped is-hoverable">`)
	if piiAuth {
		s.WriteString(`<thead><tr><th>ID</th><th>Name</th><th>Accounts</th><th>Total Savings</th><th>Total Lending</th><th></th></tr></thead><tbody>`)
	} else {
		s.WriteString(`<thead><tr><th>ID</th><th>Accounts</th><th>Total Savings</th><th>Total Lending</th><th></th></tr></thead><tbody>`)
	}
	for _, c := range customers {
		if piiAuth {
			name, _ := q.CustomerName(ctx, c.ID)
			s.WriteString(fmt.Sprintf(`<tr>
  <td><code>%s</code></td><td>%s</td><td>%d</td><td>%s</td><td>%s</td>
  <td><a href="customers/%s" class="button is-small is-link is-light">View</a></td>
</tr>`, c.ID, html.EscapeString(name), c.Accounts, FormatMoney(c.Savings), FormatMoney(c.Lending), c.ID))
		} else {
			s.WriteString(fmt.Sprintf(`<tr>
  <td><code>%s</code></td><td>%d</td><td>%s</td><td>%s</td>
  <td><a href="customers/%s" class="button is-small is-link is-light">View</a></td>
</tr>`, c.ID, c.Accounts, FormatMoney(c.Savings), FormatMoney(c.Lending), c.ID))
		}
	}
	s.WriteString(`</tbody></table></div>`)
	return s.String()
}
