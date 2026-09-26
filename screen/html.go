package screen

import (
	"fmt"
	"html"
	"strings"
)

// HTML renders a screen as the inner content of a phone frame: header, body
// and bottom nav. It escapes all text. Document wraps it in a full page.
func HTML(s Screen) string {
	var b strings.Builder
	b.WriteString(`<div class="phone-header">`)
	if s.Back != nil && s.Back.Screen != "" {
		fmt.Fprintf(&b, `<a class="back" href="%s">&#8249; Back</a>`, esc(s.Back.Screen))
	}
	fmt.Fprintf(&b, `<p class="title">%s</p>`, esc(s.Title))
	if s.Subtitle != "" {
		fmt.Fprintf(&b, `<p class="subtitle">%s</p>`, esc(s.Subtitle))
	}
	b.WriteString(`</div><div class="phone-body">`)
	for _, c := range s.Body {
		b.WriteString(component(c))
	}
	b.WriteString(`</div>`)
	if s.Nav != nil {
		b.WriteString(`<div class="phone-nav">`)
		for _, t := range s.Nav.Tabs {
			cls := ""
			if t.Key == s.Nav.Active {
				cls = ` class="is-active"`
			}
			fmt.Fprintf(&b, `<a href="%s"%s><span>%s</span>%s</a>`, esc(t.Screen), cls, glyph(t.Icon), esc(t.Label))
		}
		b.WriteString(`</div>`)
	}
	return b.String()
}

// Document renders a screen as a complete HTML page inside a phone frame,
// with no external scripts or stylesheets.
func Document(s Screen) string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<title>` + esc(s.Title) + `</title><style>` + css + `</style></head><body>` +
		`<div class="phone-frame"><div class="phone-notch"></div>` + HTML(s) + `</div></body></html>`
}

func component(c Component) string {
	var b strings.Builder
	switch c.Type {
	case TypeHero:
		g1, g2 := gradient(c.Tone)
		fmt.Fprintf(&b, `<div class="hero" style="background:linear-gradient(135deg,%s,%s)"><p class="label">%s</p><p class="value">%s</p>`, g1, g2, esc(c.Title), esc(c.Value))
		if len(c.Pairs) > 0 {
			b.WriteString(`<div class="pairs">`)
			for _, p := range c.Pairs {
				fmt.Fprintf(&b, `<span>%s: %s</span>`, esc(p.Label), esc(p.Value))
			}
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	case TypeRow:
		href := ""
		if c.Action != nil && c.Action.Screen != "" {
			href = c.Action.Screen
		}
		if href != "" {
			fmt.Fprintf(&b, `<a class="rowlink" href="%s">`, esc(href))
		}
		fmt.Fprintf(&b, `<div class="row" style="border-left-color:%s"><div><span>%s</span> <strong>%s</strong><br><span class="muted">%s</span></div>`, accent(c.Tone), glyph(c.Icon), esc(c.Title), esc(c.Subtitle))
		fmt.Fprintf(&b, `<div class="right"><div><strong>%s</strong><br><span class="muted">%s</span></div>`, esc(c.Value), esc(c.Note))
		if href != "" {
			b.WriteString(`<span class="chev">&#8250;</span>`)
		}
		b.WriteString(`</div></div>`)
		if href != "" {
			b.WriteString(`</a>`)
		}
	case TypeTx:
		fmt.Fprintf(&b, `<div class="tx"><div class="txicon">%s</div><div class="txbody"><div class="line"><strong>%s</strong><span style="color:%s;font-weight:bold">%s</span></div><div class="line muted"><span>%s</span><span>%s</span></div></div></div>`,
			glyph(c.Icon), esc(c.Title), accent(c.Tone), esc(c.Value), esc(c.Subtitle), esc(c.Note))
	case TypeHeading:
		fmt.Fprintf(&b, `<p class="heading">%s</p>`, esc(c.Text))
	case TypeText:
		switch c.Tone {
		case ToneDanger:
			fmt.Fprintf(&b, `<div class="notice">%s</div>`, esc(c.Text))
		case ToneMuted:
			fmt.Fprintf(&b, `<p class="muted center">%s</p>`, esc(c.Text))
		default:
			fmt.Fprintf(&b, `<p>%s</p>`, esc(c.Text))
		}
	case TypeDetails:
		fmt.Fprintf(&b, `<div class="details"><p class="heading">%s</p>`, esc(c.Title))
		for _, p := range c.Pairs {
			fmt.Fprintf(&b, `<div class="line"><span class="muted">%s</span><span>%s</span></div>`, esc(p.Label), esc(p.Value))
		}
		b.WriteString(`</div>`)
	case TypeForm:
		submit := ""
		if c.Action != nil {
			submit = c.Action.Submit
		}
		fmt.Fprintf(&b, `<form method="POST" action="%s">`, esc(submit))
		for _, f := range c.Fields {
			kind := f.Kind
			if kind == "" {
				kind = "text"
			}
			req := ""
			if f.Required {
				req = " required"
			}
			fmt.Fprintf(&b, `<label>%s</label><input type="%s" name="%s" placeholder="%s" autocomplete="off"%s>`, esc(f.Label), esc(kind), esc(f.Name), esc(f.Placeholder), req)
		}
		fmt.Fprintf(&b, `<button type="submit">%s</button></form>`, esc(c.Title))
	case TypeButton:
		if c.Action != nil && c.Action.Screen != "" {
			fmt.Fprintf(&b, `<p class="center"><a class="button" href="%s">%s</a></p>`, esc(c.Action.Screen), esc(c.Title))
		} else if c.Action != nil && c.Action.Submit != "" {
			fmt.Fprintf(&b, `<form method="POST" action="%s" class="center"><button type="submit">%s</button></form>`, esc(c.Action.Submit), esc(c.Title))
		} else if c.Action != nil && c.Action.Logout {
			fmt.Fprintf(&b, `<form method="POST" action="/v1/logout" class="center"><button type="submit" class="secondary">%s</button></form>`, esc(c.Title))
		}
	default:
		// Unknown component: render a visible fallback rather than nothing,
		// mirroring what clients must do.
		fmt.Fprintf(&b, `<p class="muted center">[%s]</p>`, esc(c.Type))
	}
	return b.String()
}

func esc(s string) string { return html.EscapeString(s) }

func glyph(i Icon) string {
	switch i {
	case IconHome:
		return "&#127968;"
	case IconAccounts, IconLending:
		return "&#128179;"
	case IconActivity:
		return "&#128196;"
	case IconSavings:
		return "&#128178;"
	case IconIn:
		return "&#8595;"
	case IconOut:
		return "&#8593;"
	case IconInterest:
		return "&#9733;"
	case IconLoan, IconBank:
		return "&#127974;"
	}
	return ""
}

func accent(t Tone) string {
	switch t {
	case ToneSavings, TonePositive:
		return "#48c78e"
	case ToneLending:
		return "#3e8ed0"
	case ToneNegative, ToneDanger:
		return "#f14668"
	case ToneMuted:
		return "#7a7a7a"
	}
	return "#363636"
}

func gradient(t Tone) (string, string) {
	if t == ToneLending {
		return "#3e8ed0", "#5ea8e5"
	}
	return "#00947e", "#00b89c"
}

const css = `
body{background:#2c2c2e;margin:0;display:flex;justify-content:center;align-items:center;min-height:100vh;font-family:system-ui,sans-serif;font-size:15px;color:#363636}
.phone-frame{width:375px;height:812px;border-radius:44px;border:6px solid #1c1c1e;background:#fff;box-shadow:0 20px 60px rgba(0,0,0,.4);overflow:hidden;position:relative;display:flex;flex-direction:column}
.phone-notch{width:150px;height:28px;background:#1c1c1e;border-radius:0 0 18px 18px;margin:0 auto;position:relative;z-index:10}
.phone-header{background:#00947e;color:#fff;padding:8px 16px 12px;text-align:center}
.phone-header .title{margin:0;font-size:1.1rem;font-weight:600}.phone-header .subtitle{margin:0;font-size:.8rem;opacity:.8}
.phone-header .back{display:block;text-align:left;color:rgba(255,255,255,.8);text-decoration:none;font-size:.8rem}
.phone-body{flex:1;overflow-y:auto;padding:12px 16px}
.phone-nav{background:#fafafa;border-top:1px solid #eee;display:flex;padding:8px 0 12px}
.phone-nav a{flex:1;text-align:center;color:#7a7a7a;font-size:.7rem;text-decoration:none}.phone-nav a.is-active{color:#00947e;font-weight:bold}.phone-nav a span{display:block;font-size:1.2rem}
.hero{border-radius:14px;padding:20px;color:#fff;margin-bottom:16px}.hero p{margin:0}.hero .label{font-size:.8rem;opacity:.8}.hero .value{font-size:1.8rem;font-weight:bold;margin:4px 0}
.hero .pairs{display:flex;justify-content:space-between;font-size:.75rem;opacity:.85;margin-top:8px}
.rowlink{display:block;text-decoration:none;color:inherit}
.row{background:#fafafa;border-radius:10px;padding:14px 16px;border-left:4px solid #363636;margin-bottom:8px;display:flex;justify-content:space-between;align-items:center}
.row .right{display:flex;align-items:center;text-align:right}.row .chev{color:#b5b5b5;font-size:1.2rem;margin-left:8px}
.muted{font-size:.75rem;color:#7a7a7a}.center{text-align:center}
.tx{display:flex;align-items:center;padding:8px 0;border-bottom:1px solid #f0f0f0}
.txicon{width:32px;height:32px;border-radius:50%;background:#f0f0f0;display:flex;align-items:center;justify-content:center;margin-right:10px;font-size:.9rem}
.txbody{flex:1}.line{display:flex;justify-content:space-between;font-size:.85rem}.line.muted{font-size:.7rem}
.heading{font-size:.8rem;color:#7a7a7a;margin:12px 0 4px;font-weight:bold}
.details{background:#fafafa;border-radius:10px;padding:14px 16px;margin-bottom:16px}.details .line{padding:6px 0;border-bottom:1px solid #eee;font-size:.8rem}.details .heading{margin-top:0;color:#363636}
.notice{background:#feecf0;color:#cc0f35;border-radius:10px;padding:10px 14px;font-size:.8rem;margin-bottom:12px}
form label{display:block;font-size:.75rem;color:#7a7a7a;margin-bottom:4px}
form input{width:100%;box-sizing:border-box;padding:10px 14px;border:1px solid #dbdbdb;border-radius:10px;font-size:.9rem;margin-bottom:12px}
button,.button{display:inline-block;width:100%;box-sizing:border-box;padding:12px;border:none;border-radius:10px;background:#00947e;color:#fff;font-weight:bold;font-size:.95rem;cursor:pointer;text-decoration:none;text-align:center}
button.secondary{background:#eee;color:#363636}
`
