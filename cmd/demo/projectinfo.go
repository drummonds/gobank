package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"strings"
)

//go:embed project_data.json
var projectDataJSON []byte

type scopeItem struct {
	Title string `json:"title"`
	Desc  string `json:"desc"`
	Done  bool   `json:"done"`
}

type projectData struct {
	Name        string      `json:"name"`
	Tagline     string      `json:"tagline"`
	Repo        string      `json:"repo"`
	GoLucaRepo  string      `json:"goluca_repo"`
	Description string      `json:"description"`
	What        string      `json:"what"`
	CurrentAims []scopeItem `json:"current_aims"`
	FutureScope []scopeItem `json:"future_scope"`
	NonGoals    []scopeItem `json:"non_goals"`
}

var project projectData

func init() {
	if err := json.Unmarshal(projectDataJSON, &project); err != nil {
		log.Fatalf("failed to parse project_data.json: %v", err)
	}
}

// BuildProjectAboutHTML renders the project overview page.
func BuildProjectAboutHTML() string {
	var s strings.Builder

	s.WriteString(`<h2 class="title is-4">`)
	s.WriteString(project.Name)
	s.WriteString(` &mdash; `)
	s.WriteString(project.Tagline)
	s.WriteString(`</h2>`)

	s.WriteString(`<p class="subtitle is-6 has-text-grey">`)
	s.WriteString(project.Description)
	s.WriteString(`</p>`)

	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">What is this?</h3>`)
	s.WriteString(`<p>`)
	s.WriteString(project.What)
	s.WriteString(`</p>`)
	s.WriteString(`</div>`)

	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Current Aims</h3>`)
	renderScopeItems(&s, project.CurrentAims)
	s.WriteString(`</div>`)

	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Future Scope</h3>`)
	renderScopeItems(&s, project.FutureScope)
	s.WriteString(`</div>`)

	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Non-Goals</h3>`)
	renderScopeItems(&s, project.NonGoals)
	s.WriteString(`</div>`)

	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Links</h3>`)
	s.WriteString(`<ul>`)
	s.WriteString(`<li><a href="`)
	s.WriteString(project.Repo)
	s.WriteString(`">`)
	s.WriteString(project.Repo)
	s.WriteString(`</a></li>`)
	s.WriteString(`<li><a href="`)
	s.WriteString(project.GoLucaRepo)
	s.WriteString(`">`)
	s.WriteString(project.GoLucaRepo)
	s.WriteString(`</a> &mdash; accounting library</li>`)
	s.WriteString(`</ul>`)
	s.WriteString(`</div>`)

	return s.String()
}

func renderScopeItems(s *strings.Builder, items []scopeItem) {
	s.WriteString(`<table class="table is-fullwidth">`)
	for _, item := range items {
		s.WriteString(`<tr><th>`)
		s.WriteString(item.Title)
		if item.Done {
			s.WriteString(` <span class="tag is-success is-light">done</span>`)
		}
		s.WriteString(`</th><td>`)
		s.WriteString(item.Desc)
		s.WriteString(`</td></tr>`)
	}
	s.WriteString(`</table>`)
}
