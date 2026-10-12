package staff

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	luca "git.bytestone.uk/hum3/go-luca"
	"git.bytestone.uk/hum3/gobank/core"
	"git.bytestone.uk/hum3/gobank/internal/view"
	"git.bytestone.uk/hum3/pta2svg"
)

// The movement page (ADR-0008): beneath a statement line, the movement it
// is one side of, as written: a signed amount from one account to
// another. The page carries the two legs, the amount and times, and the
// movement drawn by pta2svg from its model, the ledger as data, so no
// text is parsed: rendered by the BFF, and again in the browser by
// pta2svg's lite WASM build from the model embedded in the page when the
// asset is served, the first rendering a general client takes on. The
// same movement is served as goluca text, the ledger's own format, and as
// JSON.

// wasmFiles is pta2svg's lite browser build, when the build step put it
// here (task wasm:pta2svg): pta2svg.wasm, compiled by TinyGo, and its
// wasm_exec.js. Absent, the page keeps the BFF's rendering.
//
//go:embed all:wasm
var wasmFiles embed.FS

// movementRoutes serves the movement page, its goluca text and JSON, and
// the WASM assets.
func (s *Site) movementRoutes() {
	bank := s.cfg.Bank
	s.mux.HandleFunc("/movements/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		format := ""
		for _, suffix := range []string{".goluca", ".json"} {
			if before, ok := strings.CutSuffix(id, suffix); ok {
				id, format = before, suffix
			}
		}
		m, err := bank.Movement(r.Context(), id)
		if errors.Is(err, core.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("movement %s: %v", id, err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		switch format {
		case ".goluca":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = fmt.Fprint(w, MovementGoluca(m))
		case ".json":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(m)
		default:
			// Quiescent once loaded: a movement does not change, so the
			// page never re-polls.
			if r.Method != "GET" {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			content := BuildMovementHTML(m, s.view(r), s.wasmServed())
			if s.fragment(w, r, content) {
				return
			}
			s.staticPage(w, r, content)
		}
	})
	sub, _ := fs.Sub(wasmFiles, "wasm")
	s.mux.Handle("/wasm/", http.StripPrefix("/wasm/", http.FileServer(http.FS(sub))))
}

// wasmServed says whether pta2svg's browser build is embedded.
func (s *Site) wasmServed() bool {
	_, err := fs.Stat(wasmFiles, "wasm/pta2svg.wasm")
	return err == nil
}

// MovementGoluca is a movement as goluca text pta2svg draws: a balance
// directive per account for what it held before, then the movement, so
// the diagram shows the balances the accounts are left with. Paths are
// quoted, since a product ID carries a hyphen.
func MovementGoluca(m core.Movement) string {
	day := m.ValueTime.UTC().Format("2006-01-02")
	var b strings.Builder
	fmt.Fprintf(&b, "# Movement %s: %s\n", m.ID, m.Description)
	fmt.Fprintf(&b, "# code %s, value %s, known %s\n\n", m.Code, m.ValueTime.UTC().Format(time.RFC3339), m.KnowledgeTime.UTC().Format(time.RFC3339))
	// Before the movement: the from account held its balance after plus
	// the amount (the movement took it down by that), the to account its
	// balance after less the amount.
	fmt.Fprintf(&b, "%s balance %q %s %s\n", day, m.From.Path, golucaAmount(m.From.Balance+m.Amount), m.Currency)
	fmt.Fprintf(&b, "%s balance %q %s %s\n\n", day, m.To.Path, golucaAmount(m.To.Balance-m.Amount), m.Currency)
	fmt.Fprintf(&b, "%s * %s\n", day, strings.ReplaceAll(m.Description, "\n", " "))
	fmt.Fprintf(&b, "  %q -> %q %q %s %s\n", m.From.Path, m.To.Path, m.Code, golucaAmount(m.Amount), m.Currency)
	return b.String()
}

// golucaAmount is minor units as goluca writes money: pounds to two places, signed.
func golucaAmount(v luca.Amount) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}

// MovementModel is the movement as pta2svg's model: the two accounts with
// their types, the balances they held before as the opening, and one
// transaction carrying the signed movement with its code as the arrow's
// label. Money is in major units, as the model draws it.
func MovementModel(m core.Movement) pta2svg.Model {
	account := func(l core.Leg) pta2svg.ModelAccount {
		return pta2svg.ModelAccount{Name: l.Path, Short: l.Path[strings.LastIndex(l.Path, ":")+1:], Type: l.Type}
	}
	return pta2svg.Model{
		Accounts: []pta2svg.ModelAccount{account(m.From), account(m.To)},
		Opening: map[string]float64{
			m.From.Path: major(m.From.Balance + m.Amount),
			m.To.Path:   major(m.To.Balance - m.Amount),
		},
		Transactions: []pta2svg.ModelTransaction{{
			Date: m.ValueTime.UTC().Format("2006-01-02"), Flag: "*", Payee: m.Description,
			Movements: []pta2svg.ModelMovement{{From: m.From.Path, To: m.To.Path, Amount: major(m.Amount), Commodity: m.Currency, Description: m.Code}},
		}},
	}
}

// major is minor units as the major unit a drawing shows: pounds, to two
// places. For presentation only; the ledger holds minor units.
func major(v luca.Amount) float64 {
	return float64(v) / 100
}

// movementPalette fills pta2svg's boxes in the bank's colours: the light
// shades of the Bulma tags accountTypeTag uses for the same types.
var movementPalette = map[string]string{
	"Asset":     "#d1f0e2", // success
	"Liability": "#fcd5dd", // danger
	"Equity":    "#fff3c4", // warning
	"Income":    "#d6e6f5", // info
	"Expense":   "#ccfaf3", // primary
}

// movementOptions is how the movement is drawn, by the BFF and in the
// browser alike: the layered layout, so the account the movement is from
// sits left of the account it is to and the arrow reads with the
// movement (the column layout would place them by account type); money
// written as the bank writes it; the boxes in the bank's colours.
func movementOptions(m core.Movement) pta2svg.Options {
	return pta2svg.Options{
		Layout: pta2svg.Sugiyama, Title: "Movement " + m.ID,
		Commodities: map[string]pta2svg.Commodity{"GBP": {Symbol: "£", Decimals: 2, Thousands: ","}},
		Palette:     movementPalette,
	}
}

// MovementSVG is the movement drawn by pta2svg from its model, or an
// empty string and the error when it cannot be drawn.
func MovementSVG(m core.Movement) (string, error) {
	model := MovementModel(m)
	d, err := model.Diagram()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = pta2svg.RenderDiagram(&buf, d, movementOptions(m))
	return buf.String(), err
}

// BuildMovementHTML renders the movement page: the account the movement
// is from and the account it is to, with the balances they were left
// with in the bank's view; the signed amount; the drawing; the goluca
// text with links to it and to the JSON.
func BuildMovementHTML(m core.Movement, v view.View, wasm bool) string {
	var s strings.Builder
	fmt.Fprintf(&s, `<h2 class="title is-4">Movement</h2><p class="subtitle is-6 has-text-grey"><code>%s</code></p>`, html.EscapeString(m.ID))
	s.WriteString(`<div class="box"><div class="columns">`)
	fmt.Fprintf(&s, `<div class="column"><strong>Amount:</strong> %s</div>`, FormatMoney(m.Amount))
	fmt.Fprintf(&s, `<div class="column"><strong>Code:</strong> <code>%s</code> <span class="has-text-grey is-size-7">%s</span></div>`, html.EscapeString(m.Code), html.EscapeString(DescribeCode(m.Code)))
	fmt.Fprintf(&s, `<div class="column"><strong>Value:</strong> %s</div>`, FormatUTC(m.ValueTime))
	fmt.Fprintf(&s, `<div class="column"><strong>Known:</strong> %s</div>`, FormatUTC(m.KnowledgeTime))
	s.WriteString(`</div>`)
	fmt.Fprintf(&s, `<p><strong>Description:</strong> %s</p>`, html.EscapeString(m.Description))
	s.WriteString(describeCause(m))
	s.WriteString(`<table class="table is-narrow is-fullwidth mt-3"><thead><tr><th></th><th>Account</th><th>Type</th><th>Balance after (bank view)</th></tr></thead><tbody>`)
	for _, leg := range []struct {
		side string
		l    core.Leg
	}{{"From", m.From}, {"To", m.To}} {
		fmt.Fprintf(&s, `<tr><td><strong>%s</strong></td><td><code>%s</code></td><td>%s</td><td>%s</td></tr>`,
			leg.side, html.EscapeString(leg.l.Path), accountTypeTag(leg.l.Type), FormatMoney(leg.l.Balance))
	}
	s.WriteString(`</tbody></table>`)
	if v == view.Customer {
		s.WriteString(`<p class="has-text-grey is-size-7">The session is in the customer view; a movement is always shown in the bank's view, as it was written (ADR-0008).</p>`)
	}
	s.WriteString(`</div>`)

	s.WriteString(`<div class="box"><h3 class="title is-5">Drawn</h3>`)
	svg, err := MovementSVG(m)
	if err != nil {
		fmt.Fprintf(&s, `<p class="has-text-danger">pta2svg: %s</p>`, html.EscapeString(err.Error()))
	} else {
		s.WriteString(`<div id="movement-svg">` + svg + `</div>`)
	}
	s.WriteString(`</div>`)

	s.WriteString(`<div class="box"><h3 class="title is-5">As goluca</h3>`)
	fmt.Fprintf(&s, `<pre id="movement-goluca">%s</pre>`, html.EscapeString(MovementGoluca(m)))
	fmt.Fprintf(&s, `<p><a href="movements/%s.goluca">goluca</a> · <a href="movements/%s.json">JSON</a></p></div>`, html.EscapeString(m.ID), html.EscapeString(m.ID))
	if wasm {
		// The model the browser renders and the options it draws it
		// under, as JSON; encoding/json escapes <, > and & so a script
		// element cannot be closed early.
		model, _ := json.Marshal(MovementModel(m))
		options, _ := json.Marshal(movementOptions(m))
		fmt.Fprintf(&s, `<script type="application/json" id="movement-model">%s</script>`, model)
		fmt.Fprintf(&s, `<script type="application/json" id="movement-options">%s</script>`, options)
		s.WriteString(movementWASMScript)
	}
	return s.String()
}

// accountTypeTag colours an account type as pta2svg does.
func accountTypeTag(kind string) string {
	class := "is-light"
	switch kind {
	case "Asset":
		class = "is-success is-light"
	case "Liability":
		class = "is-danger is-light"
	case "Equity":
		class = "is-warning is-light"
	case "Income":
		class = "is-info is-light"
	case "Expense":
		class = "is-primary is-light"
	}
	return fmt.Sprintf(`<span class="tag %s">%s</span>`, class, html.EscapeString(kind))
}

// movementWASMScript redraws the movement once in the browser with
// pta2svg's lite WASM build, from the model and options embedded in the
// page; failing that, the BFF's drawing stands and the console says why.
// The options' Go field names are the JSON keys the WASM entry reads,
// lower-cased.
const movementWASMScript = `<script src="wasm/wasm_exec.js"></script>
<script>
(async () => {
  try {
    const go = new Go();
    const result = await WebAssembly.instantiateStreaming(fetch('wasm/pta2svg.wasm'), go.importObject);
    go.run(result.instance);
    const model = JSON.parse(document.getElementById('movement-model').textContent);
    const o = JSON.parse(document.getElementById('movement-options').textContent);
    const opts = {layout: o.Layout, title: o.Title, palette: o.Palette, commodities: {}};
    for (const [code, c] of Object.entries(o.Commodities || {})) opts.commodities[code] = c;
    document.getElementById('movement-svg').innerHTML = pta2svg.renderModel(model, opts);
  } catch (e) {
    console.warn('pta2svg in the browser: ' + e + '; the BFF’s drawing stands');
  }
})();
</script>`
