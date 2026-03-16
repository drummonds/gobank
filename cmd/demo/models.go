package main

import (
	_ "embed"
	"strings"
)

//go:embed diagrams/system_context.svg
var systemContextSVG string

//go:embed diagrams/container.svg
var containerSVG string

// BuildModelsHTML renders C4 architecture diagrams (pre-rendered from d2).
func BuildModelsHTML() string {
	var s strings.Builder

	s.WriteString(`<h2 class="title is-4">Architecture Models</h2>`)
	s.WriteString(`<p class="subtitle is-6 has-text-grey">C4 architecture diagrams for the Model Bank</p>`)

	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">System Context</h3>`)
	s.WriteString(systemContextSVG)
	s.WriteString(`</div>`)

	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Container Diagram</h3>`)
	s.WriteString(containerSVG)
	s.WriteString(`</div>`)

	return s.String()
}
