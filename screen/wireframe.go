package screen

import (
	"fmt"
	"sort"
	"strings"
)

// The wireframe is the app drawn from its screen trees, as the journeys
// are (journeys.go): one phone per screen, with its header, its body
// components and its nav, and every action wired from the component that
// carries it to the screen or endpoint it opens. It is derived purely from
// the trees, so it never drifts from what the BFF serves; a screen not yet
// served is drawn too, dashed, when the Journeys mark it Proposed, so the
// wireframe is also where a new screen is designed first.

// maxLikeRows is how many consecutive components of one type a phone shows
// before folding the rest into a count.
const maxLikeRows = 3

// maxTxRows is how many transaction rows a phone shows before folding the
// rest of the statement into a count.
const maxTxRows = 4

// phoneWidth is a phone's width in the diagram.
const phoneWidth = 340

// Wireframe renders the screens as a d2 wire diagram. Groups puts the
// phones of one state (logged out, logged in, onboarding) in a container;
// Proposed draws a screen dashed.
func (j Journeys) Wireframe() string {
	var b strings.Builder
	b.WriteString("vars: { d2-config: { layout-engine: elk } }\ndirection: down\n\n")

	// Phones, grouped.
	type phone struct {
		node   string // NodeID
		id     string // d2 id, qualified by its group
		screen Screen
	}
	phones := map[string]*phone{}
	var order []string
	groups := map[string][]*phone{}
	var groupOrder []string
	for _, s := range j.Screens {
		n := NodeID(s.Path)
		if _, ok := phones[n]; ok {
			continue
		}
		g := j.Groups[n]
		p := &phone{node: n, screen: s}
		if g != "" {
			p.id = groupID(g) + "." + d2ID(n)
		} else {
			p.id = d2ID(n)
		}
		phones[n] = p
		order = append(order, n)
		if _, seen := groups[g]; !seen {
			groupOrder = append(groupOrder, g)
		}
		groups[g] = append(groups[g], p)
	}
	for _, g := range groupOrder {
		indent := ""
		if g != "" {
			fmt.Fprintf(&b, "%s: %s {\n  grid-columns: %d\n  grid-gap: 40\n  style.fill: \"#fafafa\"\n  style.stroke: \"#bdbdbd\"\n  style.font-size: 20\n", groupID(g), d2Text(g), min(len(groups[g]), 3))
			indent = "  "
		}
		for _, p := range groups[g] {
			j.writePhone(&b, indent, d2ID(p.node), p.screen, j.Proposed[p.node])
		}
		if g != "" {
			b.WriteString("}\n\n")
		}
	}

	// Edges from the component that carries the action, one per target
	// from each screen. Endpoints are drawn as hexagons.
	type wire struct{ from, to, label string }
	var wires []wire
	seen := map[string]bool{}
	endpoints := map[string]bool{}
	navDrawn := map[string]bool{}
	add := func(from, to, label string) {
		if to == "" {
			return
		}
		toNode := NodeID(to)
		toID := d2ID(toNode)
		if p, ok := phones[toNode]; ok {
			toID = p.id
		} else {
			endpoints[toNode] = true
		}
		key := from + "->" + toID
		if seen[key] || strings.HasPrefix(from, toID+".") {
			return
		}
		seen[key] = true
		wires = append(wires, wire{from, toID, label})
	}
	for _, n := range order {
		p := phones[n]
		s := p.screen
		if s.Back != nil {
			add(p.id+".header", s.Back.Screen, "back")
		}
		for i, c := range s.Body {
			if c.Action == nil {
				continue
			}
			label := c.Title
			if label == "" {
				label = c.Type
			}
			from := fmt.Sprintf("%s.body.c%d", p.id, i)
			switch {
			case c.Action.Screen != "":
				add(from, c.Action.Screen, label)
			case c.Action.Submit != "":
				add(from, c.Action.Submit, label)
				if next, ok := j.Endpoints[c.Action.Submit]; ok {
					add(d2ID(NodeID(c.Action.Submit)), next, "ok")
				}
			case c.Action.Logout:
				add(from, "/v1/logout", label)
				if next, ok := j.Endpoints["/v1/logout"]; ok {
					add(d2ID("/v1/logout"), next, "")
				}
			}
		}
		// The tab bar is the same on every screen of a state, so its wires
		// are drawn once, from the first screen that carries it.
		if s.Nav != nil && !navDrawn[j.Groups[n]] {
			navDrawn[j.Groups[n]] = true
			for _, t := range s.Nav.Tabs {
				add(fmt.Sprintf("%s.nav.%s", p.id, d2ID(t.Key)), t.Screen, "tab: "+t.Label)
			}
		}
	}
	var eps []string
	for n := range endpoints {
		eps = append(eps, n)
	}
	sort.Strings(eps)
	for _, n := range eps {
		fmt.Fprintf(&b, "%s: %s {\n  shape: hexagon\n  style.fill: \"#fff3e0\"\n  style.stroke: \"#ff9800\"\n}\n\n", d2ID(n), d2Text(n))
	}
	for _, w := range wires {
		if w.label != "" {
			fmt.Fprintf(&b, "%s -> %s: %s\n", w.from, w.to, d2Text(w.label))
		} else {
			fmt.Fprintf(&b, "%s -> %s\n", w.from, w.to)
		}
	}
	return b.String()
}

// writePhone draws one screen as a phone: header, body, nav.
func (j Journeys) writePhone(b *strings.Builder, indent, id string, s Screen, proposed bool) {
	w := func(format string, args ...any) {
		fmt.Fprintf(b, indent+format+"\n", args...)
	}
	w("%s: {", id)
	w("  label: \"\"")
	w("  grid-columns: 1")
	w("  grid-gap: 0")
	w("  width: %d", phoneWidth)
	w("  style.fill: \"#ffffff\"")
	w("  style.stroke: \"#263238\"")
	w("  style.stroke-width: 3")
	w("  style.border-radius: 24")
	if proposed {
		w("  style.stroke-dash: 5")
	}
	// Header: screen id and path on the first line (what the diagram is
	// about), the title the customer sees on the second.
	title := s.Title
	if s.Subtitle != "" {
		title += " · " + s.Subtitle
	}
	back := ""
	if s.Back != nil {
		back = "‹ "
	}
	note := ""
	if proposed {
		note = " (proposed)"
	}
	w("  header: |md\n    **%s**%s\n\n    %s%s\n\n    %s\n  | {\n    width: %d\n    style.fill: \"#009688\"\n    style.font-color: \"#ffffff\"\n    style.stroke: \"#009688\"\n  }",
		d2Text(s.ID), note, back, d2Text(title), d2Text(NodeID(s.Path)), phoneWidth-8)

	// Body: components in order, long runs of one type folded.
	w("  body: {")
	w("    label: \"\"")
	w("    grid-columns: 1")
	w("    grid-gap: 4")
	w("    width: %d", phoneWidth-8)
	w("    style.fill: \"#f5f5f5\"")
	w("    style.stroke: \"#f5f5f5\"")
	run, runType := 0, ""
	txs := 0 // transaction rows drawn: a statement is folded after a few
	for i, c := range s.Body {
		if c.Type == TypeTx || (c.Type == TypeHeading && txs > 0) {
			if c.Type == TypeTx {
				txs++
			}
			if txs > maxTxRows {
				rest := 0
				for k := i; k < len(s.Body); k++ {
					if s.Body[k].Type == TypeTx {
						rest++
					}
				}
				if c.Type == TypeTx && txs == maxTxRows+1 {
					w("    c%d: \"… +%d more\" {\n      shape: text\n      style.font-color: \"#9e9e9e\"\n    }", i, rest)
				}
				continue
			}
		}
		if c.Type == runType {
			run++
		} else {
			runType, run = c.Type, 1
		}
		if run > maxLikeRows {
			// Count the rest of this run, draw it once, skip to its end.
			rest := 0
			for k := i; k < len(s.Body) && s.Body[k].Type == runType; k++ {
				rest++
			}
			if run == maxLikeRows+1 {
				w("    c%d: \"… +%d more\" {\n      shape: text\n      style.font-color: \"#9e9e9e\"\n    }", i, rest)
			}
			continue
		}
		w("    c%d: %s", i, wireComponent(c, indent+"    "))
		w("    c%d.width: %d", i, phoneWidth-24)
	}
	w("  }")

	// Nav: the tab bar, active tab bold.
	if s.Nav != nil {
		w("  nav: {")
		w("    label: \"\"")
		w("    grid-rows: 1")
		w("    grid-gap: 0")
		w("    width: %d", phoneWidth-8)
		w("    style.fill: \"#eceff1\"")
		w("    style.stroke: \"#eceff1\"")
		for _, t := range s.Nav.Tabs {
			label := d2Text(t.Label)
			if t.Key == s.Nav.Active {
				label = "● " + label
			}
			w("    %s: %s {\n      style.fill: \"#eceff1\"\n      style.stroke: \"#eceff1\"\n    }", d2ID(t.Key), quote(label))
		}
		w("  }")
	}
	w("}")
	b.WriteString("\n")
}

// wireComponent draws one body component as a d2 node body (after "id: ").
func wireComponent(c Component, indent string) string {
	box := func(fill, stroke, md string) string {
		return fmt.Sprintf("|md\n%s\n%s| {\n%s  style.fill: \"%s\"\n%s  style.stroke: \"%s\"\n%s}", indentLines(md, indent+"  "), indent, indent, fill, indent, stroke, indent)
	}
	switch c.Type {
	case TypeHero:
		var md strings.Builder
		md.WriteString(fmt.Sprintf("%s\n\n# %s", d2Text(c.Title), d2Text(c.Value)))
		for _, p := range c.Pairs {
			md.WriteString(fmt.Sprintf("\n\n%s **%s**", d2Text(p.Label), d2Text(p.Value)))
		}
		return box(toneFill(c.Tone), toneStroke(c.Tone), md.String())
	case TypeRow:
		md := fmt.Sprintf("%s **%s** %s › **%s**", glyphText(c.Icon), d2Text(c.Title), d2Text(c.Value), d2Text(c.Subtitle))
		if c.Note != "" {
			md += "\n\n" + d2Text(c.Note)
		}
		return box("#ffffff", toneStroke(c.Tone), md)
	case TypeTx:
		return box("#ffffff", "#e0e0e0", fmt.Sprintf("%s %s · %s **%s**", glyphText(c.Icon), d2Text(c.Title), d2Text(c.Subtitle), d2Text(c.Value)))
	case TypeHeading:
		return fmt.Sprintf("\"%s\" {\n%s  shape: text\n%s  style.bold: true\n%s}", d2Text(c.Text), indent, indent, indent)
	case TypeText:
		return fmt.Sprintf("\"%s\" {\n%s  shape: text\n%s  style.italic: true\n%s  style.font-color: \"%s\"\n%s}", d2Text(c.Text), indent, indent, indent, toneStroke(c.Tone), indent)
	case TypeDetails:
		var md strings.Builder
		md.WriteString("**" + d2Text(c.Title) + "**")
		for _, p := range c.Pairs {
			md.WriteString(fmt.Sprintf("\n\n%s  %s", d2Text(p.Label), d2Text(p.Value)))
		}
		return box("#ffffff", "#bdbdbd", md.String())
	case TypeForm:
		var md strings.Builder
		for _, f := range c.Fields {
			label := f.Label
			if label == "" {
				label = f.Name
			}
			md.WriteString(fmt.Sprintf("%s\n\n`[ %s ]`\n\n", d2Text(label), d2Text(f.Placeholder)))
		}
		md.WriteString(fmt.Sprintf("**[ %s ]**", d2Text(c.Title)))
		return box("#ffffff", "#009688", md.String())
	case TypeButton:
		return fmt.Sprintf("\"%s\" {\n%s  shape: oval\n%s  style.fill: \"%s\"\n%s  style.stroke: \"%s\"\n%s}", d2Text(c.Title), indent, indent, toneFill(c.Tone), indent, toneStroke(c.Tone), indent)
	}
	return fmt.Sprintf("\"%s\"", d2Text(c.Type))
}

func toneFill(t Tone) string {
	switch t {
	case ToneSavings:
		return "#e0f2f1"
	case ToneLending:
		return "#fff3e0"
	case ToneDanger:
		return "#ffebee"
	case ToneMuted:
		return "#f5f5f5"
	}
	return "#ffffff"
}

func toneStroke(t Tone) string {
	switch t {
	case ToneSavings:
		return "#009688"
	case ToneLending:
		return "#ff9800"
	case ToneDanger:
		return "#c62828"
	case ToneMuted, ToneNeutral:
		return "#9e9e9e"
	case TonePositive:
		return "#2e7d32"
	case ToneNegative:
		return "#c62828"
	}
	return "#9e9e9e"
}

// glyphText is a plain-text stand-in for an icon in a wireframe.
func glyphText(i Icon) string {
	switch i {
	case IconSavings:
		return "£"
	case IconLending:
		return "⌂"
	case IconIn:
		return "↓"
	case IconOut:
		return "↑"
	case IconInterest:
		return "%"
	case IconLoan:
		return "⌂"
	case IconBank:
		return "🏦"
	case IconHome:
		return "⌂"
	case IconAccounts:
		return "▤"
	case IconActivity:
		return "≡"
	}
	return "·"
}

func indentLines(s, indent string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = indent + l
		}
	}
	return strings.Join(lines, "\n")
}

// groupID is a group's d2 id: lower case, so it reads as a container.
func groupID(g string) string { return strings.ToLower(d2ID(g)) }

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `'`) + `"` }
