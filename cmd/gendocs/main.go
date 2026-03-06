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
      <h1 class="title">{{.Name}} &mdash; {{.Tagline}}</h1>
      <p class="subtitle has-text-grey">{{.Description}}</p>

      <div class="box">
        <h3 class="title is-5">What is this?</h3>
        <p>{{.What}}</p>
      </div>

      <div class="box">
        <h3 class="title is-5">Demos</h3>
        <div class="content">
          <ul>
            <li><a href="demo/">Model Bank Demo</a> &mdash; Interactive bank simulation with accounts and interest (WASM)</li>
            <li><a href="research/chart-comparison.html">Chart Renderer Comparison</a> &mdash; Side-by-side comparison of hand-rolled SVG, go-analyze/charts, and margaid</li>
          </ul>
        </div>
      </div>

      <div class="box">
        <h3 class="title is-5">Current Aims</h3>
        <table class="table is-fullwidth">
          {{- range .CurrentAims}}
          <tr><th>{{.Title}}{{if .Done}} <span class="tag is-success is-light">done</span>{{end}}</th><td>{{.Desc}}</td></tr>
          {{- end}}
        </table>
      </div>

      <div class="box">
        <h3 class="title is-5">Future Scope</h3>
        <table class="table is-fullwidth">
          {{- range .FutureScope}}
          <tr><th>{{.Title}}</th><td>{{.Desc}}</td></tr>
          {{- end}}
        </table>
      </div>

      <div class="box">
        <h3 class="title is-5">Non-Goals</h3>
        <table class="table is-fullwidth">
          {{- range .NonGoals}}
          <tr><th>{{.Title}}</th><td>{{.Desc}}</td></tr>
          {{- end}}
        </table>
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
