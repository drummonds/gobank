// Command gendocs generates docs/index.html from cmd/demo/project_data.json.
package main

import (
	"encoding/json"
	"html/template"
	"log"
	"os"
)

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

const tmpl = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Name}} &mdash; {{.Tagline}}</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bulma@1.0.4/css/bulma.min.css">
</head>
<body>
  <nav class="navbar is-primary">
    <div class="navbar-brand">
      <span class="navbar-item has-text-weight-bold">{{.Name}}</span>
    </div>
  </nav>
  <section class="section">
    <div class="container">
      <div class="columns">
        <div class="column is-3">
          <aside class="menu is-sticky" style="position:sticky;top:1.5rem">
            <p class="menu-label">Project</p>
            <ul class="menu-list">
              <li><a href="./" class="is-active">Overview</a></li>
              <li><a href="#current-aims">Current Aims</a></li>
              <li><a href="#future-scope">Future Scope</a></li>
              <li><a href="#non-goals">Non-Goals</a></li>
            </ul>
            <p class="menu-label">Demos</p>
            <ul class="menu-list">
              <li><a href="demo/">Model Bank (WASM)</a></li>
            </ul>
            <p class="menu-label">Research Notes</p>
            <ul class="menu-list">
              <li><a href="https://h3-lofigui.statichost.page/research-charts.html">Chart Renderer Comparison</a></li>
              <li><a href="research/blue-green-schemas.html">Blue-Green Schemas</a></li>
              <li><a href="research/expand-contract.html">Expand/Contract Migrations</a></li>
              <li><a href="research/project-hierarchy.html">Project Hierarchy</a></li>
              <li><a href="research/scaling-benchmarks.html">Scaling Benchmarks</a></li>
              <li><a href="research/bff-api.html">BFF API Reference</a></li>
              <li><a href="research/development-notes.html">Development Notes</a></li>
            </ul>
            <p class="menu-label">Docs</p>
            <ul class="menu-list">
              <li><a href="ROADMAP.html">Roadmap</a></li>
              <li><a href="README.html">README</a></li>
              <li><a href="CHANGELOG.html">CHANGELOG</a></li>
            </ul>
          </aside>
        </div>
        <div class="column is-9">
          <h1 class="title">{{.Name}} &mdash; {{.Tagline}}</h1>
          <p class="subtitle has-text-grey">{{.Description}}</p>

          <div class="box">
            <h3 class="title is-5">What is this?</h3>
            <p>{{.What}}</p>
          </div>

          <div class="box" id="current-aims">
            <h3 class="title is-5">Current Aims</h3>
            <table class="table is-fullwidth">
              {{- range .CurrentAims}}
              <tr><th>{{.Title}}{{if .Done}} <span class="tag is-success is-light">done</span>{{end}}</th><td>{{.Desc}}</td></tr>
              {{- end}}
            </table>
          </div>

          <div class="box" id="future-scope">
            <h3 class="title is-5">Future Scope</h3>
            <table class="table is-fullwidth">
              {{- range .FutureScope}}
              <tr><th>{{.Title}}</th><td>{{.Desc}}</td></tr>
              {{- end}}
            </table>
          </div>

          <div class="box" id="non-goals">
            <h3 class="title is-5">Non-Goals</h3>
            <table class="table is-fullwidth">
              {{- range .NonGoals}}
              <tr><th>{{.Title}}</th><td>{{.Desc}}</td></tr>
              {{- end}}
            </table>
          </div>
        </div>
      </div>
    </div>
  </section>
  <footer class="footer">
    <div class="content has-text-centered">
      <p><a href="{{.Repo}}">{{.Repo}}</a> &mdash; <a href="{{.GoLucaRepo}}">go-luca</a></p>
    </div>
  </footer>
</body>
</html>
`

func main() {
	dataPath := "cmd/demo/project_data.json"
	outPath := "docs/index.html"

	data, err := os.ReadFile(dataPath)
	if err != nil {
		log.Fatalf("reading %s: %v", dataPath, err)
	}

	var p projectData
	if err := json.Unmarshal(data, &p); err != nil {
		log.Fatalf("parsing %s: %v", dataPath, err)
	}

	t, err := template.New("docs").Parse(tmpl)
	if err != nil {
		log.Fatalf("parsing template: %v", err)
	}

	if err := os.MkdirAll("docs", 0o755); err != nil {
		log.Fatalf("creating docs dir: %v", err)
	}

	f, err := os.Create(outPath)
	if err != nil {
		log.Fatalf("creating %s: %v", outPath, err)
	}
	defer f.Close()

	if err := t.Execute(f, p); err != nil {
		log.Fatalf("executing template: %v", err)
	}

	log.Printf("generated %s from %s", outPath, dataPath)
}
