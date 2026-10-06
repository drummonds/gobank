package screen

import (
	"strings"
	"testing"
)

// The wireframe is drawn from the screen trees, as the journeys are: one
// phone per screen with its header, body and nav, the navigation wired
// from the component that carries it to the screen it opens, groups around
// the phones of one state, and proposed screens dashed.
func TestWireframeDrawsScreensAndWiresTheirActions(t *testing.T) {
	login := New("login", "/v1/screen/login", "Model Bank")
	login.Add(Form("Log in", "/v1/login", Field{Name: "customer_id", Label: "Customer ID"}, Field{Name: "password", Label: "Password", Kind: "password"}))
	home := sample() // accounts: hero, a row to product 0, a tab to activity
	home.Add(Button("Log out", ToneMuted, Logout()))
	product := New("product", "/v1/screen/product/0", "Easy Saver")
	product.Back = Go("/v1/screen/accounts")
	for range 25 {
		product.Add(Tx(IconIn, "Deposit", "1 Jan", "£10.00", "", TonePositive))
	}
	pay := New("pay", "/v1/screen/pay", "Pay")
	pay.Add(Form("Pay", "/v1/pay", Field{Name: "to", Label: "To"}))

	j := Journeys{
		Screens:   []Screen{login, home, product, pay},
		Endpoints: map[string]string{"/v1/login": "/v1/screen/accounts", "/v1/logout": "/v1/screen/login", "/v1/pay": "/v1/screen/accounts"},
		Groups:    map[string]string{"/v1/screen/login": "Logged out", "/v1/screen/accounts": "Logged in", "/v1/screen/product/{n}": "Logged in", "/v1/screen/pay": "Logged in"},
		Proposed:  map[string]bool{"/v1/screen/pay": true},
	}
	d2 := j.Wireframe()
	for _, want := range []string{
		"logged_out: Logged out {",         // a group per state
		"v1_screen_login: {",               // a phone per screen
		"Customer ID",                      // the form's fields are drawn
		"Net Balance",                      // the hero's label
		"-> logged_in.v1_screen_product_n", // the row is wired to the screen it opens
		"tab: Activity",                    // the nav tab is wired
		"logged_in.v1_screen_product_n.header -> logged_in.v1_screen_accounts: back",
		"v1_login: /v1/login", // endpoints are drawn
		"v1_login -> logged_in.v1_screen_accounts: ok",
		"style.stroke-dash", // the proposed screen is dashed
		"+22 more",          // long lists are folded
	} {
		if !strings.Contains(d2, want) {
			t.Errorf("wireframe missing %q:\n%s", want, d2)
		}
	}
	if n := strings.Count(d2, "Deposit"); n > 5 {
		t.Errorf("a list of 25 like rows should be folded, drawn %d times", n)
	}
}
