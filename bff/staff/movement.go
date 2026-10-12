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
// is one side of, as the journal entry it is. The page carries the two
// legs, the amount and times, and the movement drawn by pta2svg from
// goluca text, the ledger's own format: rendered by the BFF, and again in
// the browser by pta2svg's WASM build when the asset is served, the first
// rendering a general client takes on. The same movement is served as
// goluca text and as JSON.

// wasmFiles is pta2svg's browser build, when the build step put it here
// (task wasm:pta2svg): pta2svg.wasm and wasm_exec.js. Absent, the page
// keeps the BFF's rendering.
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
			s.page(w, r, func() string { return BuildMovementHTML(m, s.view(r), s.wasmServed()) })
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
	// Before the movement: the credited account held its balance after
	// plus the amount (the credit took it down), the debited one its
	// balance after less the amount.
	fmt.Fprintf(&b, "%s balance %q %s %s\n", day, m.Credit.Path, golucaAmount(m.Credit.Balance+m.Amount), m.Currency)
	fmt.Fprintf(&b, "%s balance %q %s %s\n\n", day, m.Debit.Path, golucaAmount(m.Debit.Balance-m.Amount), m.Currency)
	fmt.Fprintf(&b, "%s * %s\n", day, strings.ReplaceAll(m.Description, "\n", " "))
	fmt.Fprintf(&b, "  %q -> %q %q %s %s\n", m.Credit.Path, m.Debit.Path, m.Code, golucaAmount(m.Amount), m.Currency)
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

// MovementSVG is the movement drawn by pta2svg, or an empty string and the
// error when it cannot be drawn.
func MovementSVG(m core.Movement) (string, error) {
	var buf bytes.Buffer
	err := pta2svg.Render(&buf, []byte(MovementGoluca(m)), pta2svg.Options{Format: pta2svg.Goluca, Title: "Movement " + m.ID})
	return buf.String(), err
}

// BuildMovementHTML renders the movement page: the journal entry's two
// legs with the balances they were left with, in the bank's view and,
// when the session has chosen it, the customer's beside it; the drawing;
// the goluca text with links to it and to the JSON.
func BuildMovementHTML(m core.Movement, v view.View, wasm bool) string {
	var s strings.Builder
	fmt.Fprintf(&s, `<h2 class="title is-4">Movement</h2><p class="subtitle is-6 has-text-grey"><code>%s</code></p>`, html.EscapeString(m.ID))
	s.WriteString(`<div class="box"><div class="columns">`)
	fmt.Fprintf(&s, `<div class="column"><strong>Amount:</strong> %s</div>`, FormatMoney(m.Amount))
	fmt.Fprintf(&s, `<div class="column"><strong>Code:</strong> <code>%s</code></div>`, html.EscapeString(m.Code))
	fmt.Fprintf(&s, `<div class="column"><strong>Value:</strong> %s</div>`, FormatUTC(m.ValueTime))
	fmt.Fprintf(&s, `<div class="column"><strong>Known:</strong> %s</div>`, FormatUTC(m.KnowledgeTime))
	s.WriteString(`</div>`)
	fmt.Fprintf(&s, `<p><strong>Description:</strong> %s</p>`, html.EscapeString(m.Description))
	s.WriteString(describeCause(m))
	s.WriteString(`<table class="table is-narrow is-fullwidth mt-3"><thead><tr><th></th><th>Account</th><th>Type</th><th>Balance after (bank view)</th></tr></thead><tbody>`)
	for _, leg := range []struct {
		side string
		l    core.Leg
	}{{"Credited", m.Credit}, {"Debited", m.Debit}} {
		fmt.Fprintf(&s, `<tr><td><strong>%s</strong></td><td><code>%s</code></td><td>%s</td><td>%s</td></tr>`,
			leg.side, html.EscapeString(leg.l.Path), accountTypeTag(leg.l.Type), FormatMoney(leg.l.Balance))
	}
	s.WriteString(`</tbody></table>`)
	if v == view.Customer {
		s.WriteString(`<p class="has-text-grey is-size-7">The session is in the customer view; a movement is always shown in the bank's view, the journal entry it is (ADR-0008).</p>`)
	}
	s.WriteString(`</div>`)

	s.WriteString(`<div class="box"><h3 class="title is-5">Drawn</h3>`)
	svg, err := MovementSVG(m)
	if err != nil {
		fmt.Fprintf(&s, `<p class="has-text-danger">pta2svg: %s</p>`, html.EscapeString(err.Error()))
	} else {
		s.WriteString(`<div id="movement-svg">` + svg + `</div>`)
	}
	s.WriteString(`<p class="has-text-grey is-size-7">Drawn by <a href="https://git.bytestone.uk/hum3/pta2svg">pta2svg</a> from the goluca text below`)
	if wasm {
		s.WriteString(`, in this browser <span id="movement-renderer">(loading…)</span>`)
	} else {
		s.WriteString(` by the BFF`)
	}
	s.WriteString(`.</p></div>`)

	s.WriteString(`<div class="box"><h3 class="title is-5">As goluca</h3>`)
	fmt.Fprintf(&s, `<pre id="movement-goluca">%s</pre>`, html.EscapeString(MovementGoluca(m)))
	fmt.Fprintf(&s, `<p><a href="movements/%s.goluca">goluca</a> · <a href="movements/%s.json">JSON</a></p></div>`, html.EscapeString(m.ID), html.EscapeString(m.ID))
	if wasm {
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

// movementWASMScript redraws the movement in the browser with pta2svg's
// WASM build, from the goluca text on the page; failing that, the BFF's
// drawing stands.
const movementWASMScript = `<script src="wasm/wasm_exec.js"></script>
<script>
(async () => {
  const note = document.getElementById('movement-renderer');
  try {
    const go = new Go();
    const result = await WebAssembly.instantiateStreaming(fetch('wasm/pta2svg.wasm'), go.importObject);
    go.run(result.instance);
    const text = document.getElementById('movement-goluca').textContent;
    const svg = pta2svg.render(text, {format: 'goluca', title: 'Movement'});
    document.getElementById('movement-svg').innerHTML = svg;
    if (note) note.textContent = '(rendered here by pta2svg.wasm)';
  } catch (e) {
    if (note) note.textContent = '(browser rendering unavailable: ' + e + '; the BFF’s drawing stands)';
  }
})();
</script>`
