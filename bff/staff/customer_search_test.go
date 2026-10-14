package staff

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.bytestone.uk/hum3/gobank/core"
)

// searchBank is the register as the search reads it: names by id, in
// the order the customers joined.
type searchBank struct {
	core.StaffQueries
	names map[string]string
	order []string
}

func (b *searchBank) CustomersByName(_ context.Context, name string) ([]core.CustomerSummary, error) {
	var out []core.CustomerSummary
	for _, id := range b.order {
		if strings.Contains(strings.ToLower(b.names[id]), strings.ToLower(name)) {
			out = append(out, core.CustomerSummary{ID: id, Accounts: 1, Savings: 100_00})
		}
	}
	return out, nil
}

func (b *searchBank) CustomerName(_ context.Context, id string) (string, error) {
	return b.names[id], nil
}
func (b *searchBank) Position(context.Context) (core.Position, error) { return core.Position{}, nil }
func (b *searchBank) CustomerPage(context.Context, int) (core.CustomerPage, error) {
	return core.CustomerPage{Page: 1, PerPage: 50}, nil
}

// hxPost posts as htmx does, asking for the fragment back.
func hxPost(h http.Handler, path, form string, cookie *http.Cookie) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", path, strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	h.ServeHTTP(rr, req)
	return rr
}

// The Customers page offers a search by name once personal data is
// authorised. The name goes in the body of a POST, never in a URL, and
// the matches come back as the page's table: a fragment for htmx, a
// whole page for a plain browser. A role without view_pii is refused.
func TestCustomerSearchPostsTheNameAndListsTheMatches(t *testing.T) {
	bank := &searchBank{names: map[string]string{"cust-001": "Ada Lovelace", "cust-002": "Alan Turing"}, order: []string{"cust-001", "cust-002"}}
	site := New(Config{Version: "test", Users: testUsers, Bank: bank})
	admin := signIn(t, site, "admin")

	if body := get(site, "/customers", admin).Body.String(); strings.Contains(body, "customers/search") {
		t.Error("the search is offered before personal data is authorised")
	}
	post(site, "/auth/authorize", "redirect=customers", admin)
	body := get(site, "/customers", admin).Body.String()
	for _, want := range []string{`hx-post="customers/search"`, `action="customers/search"`, `method="post"`, `name="name"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the Customers page lacks %s:\n%s", want, body)
		}
	}

	rr := hxPost(site, "/customers/search", "name=lovelace", admin)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Ada Lovelace") || strings.Contains(rr.Body.String(), "Alan Turing") || !strings.Contains(rr.Body.String(), `href="customers/cust-001"`) {
		t.Errorf("htmx search: %d\n%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "<html") {
		t.Error("an htmx search should be the fragment, not a whole page")
	}
	rr = post(site, "/customers/search", "name=turing", admin)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "<html") || !strings.Contains(rr.Body.String(), "Alan Turing") {
		t.Errorf("plain search: %d\n%s", rr.Code, rr.Body.String())
	}
	if rr = hxPost(site, "/customers/search", "name=babbage", admin); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "No customer") {
		t.Errorf("no match: %d\n%s", rr.Code, rr.Body.String())
	}

	if rr = get(site, "/customers/search?name=lovelace", admin); rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET with the name in the URL: %d, want 405", rr.Code)
	}
	ro := signIn(t, site, "ro")
	if rr = hxPost(site, "/customers/search", "name=lovelace", ro); rr.Code != http.StatusForbidden {
		t.Errorf("read-only search: %d, want 403", rr.Code)
	}
}
