// Command gendocs generates docs/index.html from bff/staff/project_data.json
// and docs/adr/index.html from the embedded ADRs, so the static site
// carries the records with the same index the running system serves.
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"os"
	"os/exec"
	"strings"

	"git.bytestone.uk/hum3/gobank/adr"
	"git.bytestone.uk/hum3/gobank/api"
	"git.bytestone.uk/hum3/gobank/internal/family"
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
  <link rel="icon" type="image/svg+xml" href="favicon.svg">
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
              <li><a href="https://lofigui.docs.bytestone.uk/research-charts.html">Chart Renderer Comparison</a></li>
              <li><a href="https://gobank-db.docs.bytestone.uk/contract-views.html">Contract Views (gobank-db)</a></li>
              <li><a href="https://gobank-db.docs.bytestone.uk/blue-green-schemas.html">Blue-Green Schemas (gobank-db)</a></li>
              <li><a href="https://gobank-db.docs.bytestone.uk/expand-contract.html">Expand/Contract Migrations (gobank-db)</a></li>
              <li><a href="research/scaling-benchmarks.html">Scaling Benchmarks</a></li>
              <li><a href="sizing.html">Sizing: 300k, 1M, 10M customers</a></li>
              <li><a href="research/development-notes.html">Development Notes</a></li>
            </ul>
            <p class="menu-label">Docs</p>
            <ul class="menu-list">
              <li><a href="ROADMAP.html">Roadmap</a></li>
              <li><a href="adr/">Architecture decisions</a></li>
              <li><a href="benchmark.html">Performance</a></li>
              <li><a href="upgrade-drill.html">Upgrade drill</a></li>
              <li><a href="app.html">Customer app</a></li>
              <li><a href="family.html">The family</a></li>
              <li><a href="api.html">API reference</a></li>
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
	dataPath := "bff/staff/project_data.json"
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

	if err := writeADRIndex("docs/adr/index.html"); err != nil {
		log.Fatal(err)
	}
	if err := writeFamily("docs/family.d2", "cmd/demo"); err != nil {
		log.Fatal(err)
	}
	if err := writeAPI("docs/api.html", "docs/openapi.json"); err != nil {
		log.Fatal(err)
	}
}

// writeAPI writes the API reference page from the generated surface and
// puts the OpenAPI document beside it.
func writeAPI(pagePath, docPath string) error {
	s, err := api.Load()
	if err != nil {
		return err
	}
	t, err := template.New("api").Parse(apiTmpl)
	if err != nil {
		return err
	}
	f, err := os.Create(pagePath)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := t.Execute(f, api.Page(s, api.Options{SourceRef: "branch/main"})); err != nil {
		return err
	}
	if err := os.WriteFile(docPath, api.Document, 0o644); err != nil {
		return err
	}
	log.Printf("generated %s and %s from the API surface", pagePath, docPath)
	return nil
}

const apiTmpl = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>gobank &mdash; API reference</title>
  <link rel="icon" type="image/svg+xml" href="favicon.svg">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bulma@1.0.4/css/bulma.min.css">
</head>
<body>
  <nav class="navbar is-primary">
    <div class="navbar-brand">
      <a class="navbar-item has-text-weight-bold" href="./">gobank</a>
    </div>
  </nav>
  <section class="section">
    <div class="container">
      <p class="is-size-7 has-text-grey">The OpenAPI document this page is rendered from: <a href="openapi.json">openapi.json</a>. In a running bank it is served at <code>/v1/openapi.json</code> and this page under About.</p>
      {{.}}
    </div>
  </section>
</body>
</html>
`

// writeFamily writes the family map (internal/family) as d2, read from
// the module at dir: the demo, which is the build that ships.
func writeFamily(outPath, dir string) error {
	run := func(args ...string) (string, error) {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("go %s in %s: %w", strings.Join(args, " "), dir, err)
		}
		return string(out), nil
	}
	graph, err := run("mod", "graph")
	if err != nil {
		return err
	}
	buildList, err := run("list", "-m", "all")
	if err != nil {
		return err
	}
	m, err := family.Parse(graph, buildList)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, []byte(m.D2()), 0o644); err != nil {
		return err
	}
	log.Printf("generated %s from go mod graph in %s", outPath, dir)
	return nil
}

const adrTmpl = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>gobank &mdash; Architecture decision records</title>
  <link rel="icon" type="image/svg+xml" href="../favicon.svg">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bulma@1.0.4/css/bulma.min.css">
</head>
<body>
  <nav class="navbar is-primary">
    <div class="navbar-brand">
      <a class="navbar-item has-text-weight-bold" href="../">gobank</a>
    </div>
  </nav>
  <section class="section">
    <div class="container">
      <h1 class="title">Architecture decision records</h1>
      <p class="subtitle has-text-grey">The decisions the Model Bank is built to, each with its diagrams.</p>
      <table class="table is-fullwidth is-striped">
        <thead><tr><th>ADR</th><th>Title</th><th>Status</th><th>Date</th></tr></thead>
        <tbody>
        {{- range .}}
        <tr><td>{{printf "%04d" .Number}}</td><td><a href="{{printf "%04d" .Number}}-{{.Slug}}.html">{{.Title}}</a></td><td>{{.Status}}</td><td>{{.Date}}</td></tr>
        {{- end}}
        </tbody>
      </table>
    </div>
  </section>
</body>
</html>
`

// writeADRIndex writes the ADR index beside the records md2html renders,
// one HTML file per numbered markdown file.
func writeADRIndex(outPath string) error {
	t, err := template.New("adr").Parse(adrTmpl)
	if err != nil {
		return err
	}
	if err := os.MkdirAll("docs/adr", 0o755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := t.Execute(f, adr.All()); err != nil {
		return err
	}
	log.Printf("generated %s from the ADRs", outPath)
	return nil
}
