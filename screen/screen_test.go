package screen

import (
	"encoding/json"
	"strings"
	"testing"
)

func sample() Screen {
	s := New("accounts", "/v1/screen/accounts", "Alice <Test>")
	s.Subtitle = "cust-001"
	s.Add(
		Hero("Net Balance", "£1,234.56", ToneSavings, Pair{"Savings", "£1,234.56"}, Pair{"Lending", "£0.00"}),
		Row(IconSavings, "Easy Saver", "3.50% APR", "£1,234.56", "Interest: £1.20", ToneSavings, Go("/v1/screen/product/0")),
		Text("<script>alert(1)</script>", ToneDanger),
		Component{Type: "hologram", Text: "future"},
	)
	s.Nav = &Nav{Active: "accounts", Tabs: []Tab{
		{Key: "accounts", Label: "Accounts", Icon: IconAccounts, Screen: "/v1/screen/accounts"},
		{Key: "activity", Label: "Activity", Icon: IconActivity, Screen: "/v1/screen/activity"},
	}}
	return s
}

func TestJSONRoundTrip(t *testing.T) {
	in := sample()
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"schema":1`) {
		t.Errorf("schema version missing: %s", data)
	}
	if strings.Contains(string(data), `"fields"`) {
		t.Errorf("unused fields should be omitted: %s", data)
	}
	var out Screen
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.ID != "accounts" || len(out.Body) != 4 || out.Body[1].Action.Screen != "/v1/screen/product/0" {
		t.Errorf("round trip mismatch: %+v", out)
	}
}

func TestHTMLEscapesAndFallsBack(t *testing.T) {
	h := HTML(sample())
	if strings.Contains(h, "<script>") {
		t.Errorf("unescaped script in output")
	}
	for _, want := range []string{"Alice &lt;Test&gt;", "£1,234.56", `href="/v1/screen/product/0"`, "[hologram]", `class="is-active"`} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %q in output", want)
		}
	}
	doc := Document(sample())
	if !strings.HasPrefix(doc, "<!DOCTYPE html>") || strings.Contains(doc, "<script") || strings.Contains(doc, "cdn.") {
		t.Errorf("document should be self-contained with no scripts: %.80s", doc)
	}
}

func TestFormHTML(t *testing.T) {
	s := New("login", "/v1/screen/login", "Model Bank")
	s.Add(Form("Log in", "/v1/login",
		Field{Name: "customer_id", Label: "Customer ID", Kind: "text", Required: true},
		Field{Name: "password", Label: "Password", Kind: "password"}))
	h := HTML(s)
	for _, want := range []string{`action="/v1/login"`, `type="password"`, `name="customer_id"`, ` required`, `<button type="submit">Log in</button>`} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %q in %s", want, h)
		}
	}
}

func TestJourneys(t *testing.T) {
	login := New("login", "/v1/screen/login", "Model Bank")
	login.Add(Form("Log in", "/v1/login", Field{Name: "customer_id"}))
	product := New("product", "/v1/screen/product/0", "Easy Saver")
	product.Back = Go("/v1/screen/accounts")
	product.Add(Button("Load more", ToneMuted, Go("/v1/screen/product/0?page=2")))
	j := Journeys{Screens: []Screen{login, sample(), product}, Endpoints: map[string]string{"/v1/login": "/v1/screen/accounts"}}

	if got := NodeID("/v1/screen/product/3?page=2"); got != "/v1/screen/product/{n}" {
		t.Errorf("NodeID = %q", got)
	}
	edges := j.Edges()
	want := map[Edge]bool{
		{"/v1/screen/login", "/v1/login", "Log in"}:                     true,
		{"/v1/login", "/v1/screen/accounts", "ok"}:                      true,
		{"/v1/screen/accounts", "/v1/screen/product/{n}", "Easy Saver"}: true,
		{"/v1/screen/accounts", "/v1/screen/activity", "tab: Activity"}: true,
		{"/v1/screen/product/{n}", "/v1/screen/accounts", "back"}:       true,
	}
	for _, e := range edges {
		delete(want, e)
		if e.From == e.To {
			t.Errorf("self edge %+v", e)
		}
	}
	for e := range want {
		t.Errorf("missing edge %+v", e)
	}
	d2 := j.D2()
	for _, s := range []string{"v1_screen_login", "v1_login: /v1/login", "shape: hexagon", "-> v1_screen_product_n: Easy Saver"} {
		if !strings.Contains(d2, s) {
			t.Errorf("d2 missing %q:\n%s", s, d2)
		}
	}
}

// The savings glyph follows the account's currency (#26): a GBP account
// shows a pound note, never the dollar sign the renderer used to hard-code.
func TestSavingsGlyphFollowsCurrency(t *testing.T) {
	const dollar = "&#128178;"
	for currency, want := range map[string]string{
		"GBP": "&#128183;", "USD": "&#128181;", "EUR": "&#128182;", "JPY": "&#128180;", "": "&#128176;",
	} {
		row := Row(IconSavings, "Easy Saver", "3.50% APR", "£1.00", "", ToneSavings, nil)
		row.Currency = currency
		tx := Tx(IconSavings, "Deposit", "Opening", "+£1.00", "Bal: £1.00", TonePositive)
		tx.Currency = currency
		s := New("accounts", "/v1/screen/accounts", "Alice")
		s.Add(row, tx)
		h := HTML(s)
		if strings.Count(h, want) != 2 || strings.Contains(h, dollar) {
			t.Errorf("currency %q: want the glyph %s on the row and the tx and no dollar sign; got %s", currency, want, h)
		}
	}
	if g := Glyph(IconLending, "GBP"); g != glyph(IconLending) {
		t.Errorf("Glyph(lending, GBP) = %s, want the lending glyph %s regardless of currency", g, glyph(IconLending))
	}
}
