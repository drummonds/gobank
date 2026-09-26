package screen

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Journeys describes how a set of screens link to one another. It is derived
// purely from the actions in the screen trees, so the diagram can never drift
// from what the BFF actually serves.
type Journeys struct {
	Screens []Screen
	// Endpoints names non-screen POST targets (e.g. "/v1/login") and the
	// screen each leads to on success, so form submits show as journeys too.
	Endpoints map[string]string
}

// Edge is one navigation link between two nodes of the journey graph.
type Edge struct {
	From, To, Label string
}

var numSeg = regexp.MustCompile(`/\d+`)

// NodeID normalises a path to a node identity: the query string is dropped and
// numeric path segments become "{n}", so "/v1/screen/product/3?page=2" and
// "/v1/screen/product/0" are the same node.
func NodeID(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	return numSeg.ReplaceAllString(path, "/{n}")
}

// Edges lists every navigation link in the screens, deduplicated, in a stable
// order. Labels come from the component or tab that carries the action.
func (j Journeys) Edges() []Edge {
	seen := map[Edge]bool{}
	var out []Edge
	add := func(from, to, label string) {
		if to == "" {
			return
		}
		e := Edge{From: NodeID(from), To: NodeID(to), Label: label}
		if e.From == e.To || seen[e] {
			return
		}
		seen[e] = true
		out = append(out, e)
	}
	for _, s := range j.Screens {
		if s.Back != nil {
			add(s.Path, s.Back.Screen, "back")
		}
		for _, c := range s.Body {
			if c.Action == nil {
				continue
			}
			label := c.Title
			if label == "" {
				label = c.Type
			}
			switch {
			case c.Action.Screen != "":
				add(s.Path, c.Action.Screen, label)
			case c.Action.Submit != "":
				add(s.Path, c.Action.Submit, label)
				if next, ok := j.Endpoints[c.Action.Submit]; ok {
					add(c.Action.Submit, next, "ok")
				}
			case c.Action.Logout:
				add(s.Path, "/v1/logout", label)
				if next, ok := j.Endpoints["/v1/logout"]; ok {
					add("/v1/logout", next, "")
				}
			}
		}
		if s.Nav != nil {
			for _, t := range s.Nav.Tabs {
				add(s.Path, t.Screen, "tab: "+t.Label)
			}
		}
	}
	return out
}

// D2 renders the journey graph as a d2 diagram: one node per screen (labelled
// with its title and the component types it shows), one node per endpoint,
// and one edge per navigation link.
func (j Journeys) D2() string {
	var b strings.Builder
	b.WriteString("direction: down\n\n")
	ids := map[string]string{}
	for _, s := range j.Screens {
		n := NodeID(s.Path)
		if _, ok := ids[n]; ok {
			continue
		}
		ids[n] = d2ID(n)
		var kinds []string
		seenKind := map[string]bool{}
		for _, c := range s.Body {
			if !seenKind[c.Type] {
				seenKind[c.Type] = true
				kinds = append(kinds, c.Type)
			}
		}
		// The node shows the screen ID, not the title: titles often carry
		// customer data (a name), which does not belong in a diagram.
		fmt.Fprintf(&b, "%s: |md\n  **%s**\n\n  `%s`\n\n  %s\n| {\n  style.fill: \"#e0f2f1\"\n  style.stroke: \"#009688\"\n}\n\n",
			ids[n], d2Text(s.ID), d2Text(n), d2Text(strings.Join(kinds, " · ")))
	}
	edges := j.Edges()
	var endpoints []string
	for _, e := range edges {
		for _, n := range []string{e.From, e.To} {
			if _, ok := ids[n]; !ok {
				ids[n] = d2ID(n)
				endpoints = append(endpoints, n)
			}
		}
	}
	sort.Strings(endpoints)
	for _, n := range endpoints {
		fmt.Fprintf(&b, "%s: %s {\n  shape: hexagon\n  style.fill: \"#fff3e0\"\n  style.stroke: \"#ff9800\"\n}\n\n", ids[n], d2Text(n))
	}
	for _, e := range edges {
		if e.Label != "" {
			fmt.Fprintf(&b, "%s -> %s: %s\n", ids[e.From], ids[e.To], d2Text(e.Label))
		} else {
			fmt.Fprintf(&b, "%s -> %s\n", ids[e.From], ids[e.To])
		}
	}
	return b.String()
}

var nonWord = regexp.MustCompile(`[^A-Za-z0-9]+`)

func d2ID(n string) string {
	id := strings.Trim(nonWord.ReplaceAllString(n, "_"), "_")
	if id == "" {
		return "root"
	}
	return id
}

func d2Text(s string) string {
	return strings.NewReplacer("\"", "'", "\n", " ", "|", "/").Replace(s)
}
