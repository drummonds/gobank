//go:build !(js && wasm)

package main

// LayoutModelBank is a custom navbar layout with multi-page navigation.
const LayoutModelBank = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Model Bank</title>
  <link rel="icon" type="image/svg+xml" href="/favicon.svg">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bulma@1.0.4/css/bulma.min.css">
  <script src="https://unpkg.com/htmx.org@2.0.4"></script>
</head>
<body hx-boost="true">
  <nav class="navbar is-primary" role="navigation" aria-label="main navigation">
    <div class="navbar-brand">
      <a class="navbar-item has-text-weight-bold" href="/">Model Bank</a>
      <a role="button" class="navbar-burger" data-target="mainNavbar" aria-label="menu" aria-expanded="false">
        <span aria-hidden="true"></span>
        <span aria-hidden="true"></span>
        <span aria-hidden="true"></span>
      </a>
    </div>
    <div id="mainNavbar" class="navbar-menu">
      <div class="navbar-start">
        <a class="navbar-item" href="/">Dashboard</a>
        <div class="navbar-item has-dropdown is-hoverable">
          <a class="navbar-link">Accounting</a>
          <div class="navbar-dropdown">
            <a class="navbar-item" href="/accounting/pnl">P&amp;L</a>
            <a class="navbar-item" href="/accounting/balance-sheet">Balance Sheet</a>
          </div>
        </div>
        <div class="navbar-item has-dropdown is-hoverable">
          <a class="navbar-link">Products</a>
          <div class="navbar-dropdown">
            <a class="navbar-item" href="/products/savings">Savings</a>
            <a class="navbar-item" href="/products/lending">Lending</a>
          </div>
        </div>
        <a class="navbar-item" href="/customers">Customers</a>
        <a class="navbar-item" href="/payments">Payments</a>
        <div class="navbar-item has-dropdown is-hoverable">
          <a class="navbar-link">Treasury</a>
          <div class="navbar-dropdown">
            <a class="navbar-item" href="/treasury/cash">Cash Position</a>
            <a class="navbar-item" href="/treasury/capital">Capital Requirements</a>
            <a class="navbar-item" href="/treasury/gilts">Gilt Purchases</a>
          </div>
        </div>
        <div class="navbar-item has-dropdown is-hoverable">
          <a class="navbar-link">Reports</a>
          <div class="navbar-dropdown">
            <a class="navbar-item" href="/reports/bbsi">BBSI</a>
            <a class="navbar-item" href="/reports/customer-view">Customer View</a>
          </div>
        </div>
        {{if eq .role "admin"}}
        <div class="navbar-item has-dropdown is-hoverable">
          <a class="navbar-link">Internal</a>
          <div class="navbar-dropdown">
            <a class="navbar-item" href="/settings">Settings</a>
            <a class="navbar-item" href="/internal/explorer">DB Explorer</a>
          </div>
        </div>
        {{end}}
        <div class="navbar-item has-dropdown is-hoverable">
          <a class="navbar-link">About</a>
          <div class="navbar-dropdown">
            <a class="navbar-item" href="/about">Project</a>
            <a class="navbar-item" href="/about/runtime">Runtime</a>
            <a class="navbar-item" href="/about/models">Models</a>
          </div>
        </div>
      </div>
      <div class="navbar-end">
        <div class="navbar-item">
          <form action="/role" method="post" hx-boost="false">
            <input type="hidden" name="redirect" value="{{ .request.URL.Path }}">
            <div class="select is-small">
              <select name="role" onchange="this.form.submit()">
                <option value="admin"{{if eq .role "admin"}} selected{{end}}>Admin</option>
                <option value="auditor"{{if eq .role "auditor"}} selected{{end}}>Auditor</option>
                <option value="cs"{{if eq .role "cs"}} selected{{end}}>Customer Service</option>
                <option value="readonly"{{if eq .role "readonly"}} selected{{end}}>Read Only</option>
              </select>
            </div>
          </form>
        </div>
        <a class="navbar-item" href="/app/" target="_blank">Bank App</a>
        <div class="navbar-item">
          <span class="tag {{if eq .polling "Running"}}is-warning{{else}}is-success{{end}}">{{ .polling }}</span>
        </div>
      </div>
    </div>
  </nav>
  <section class="section">
    <div class="container">
      <div id="results"
        {{if eq .polling "Running"}}
        hx-get="{{ .request.URL.Path }}" hx-trigger="every 1s" hx-swap="innerHTML"
        {{end}}
      >{{ .results }}</div>
    </div>
  </section>
  <style>.dash-box{min-height:4.5rem}</style>
  <footer class="footer">
    <div class="content has-text-centered">
      <p>{{ .version }}</p>
    </div>
  </footer>
  <script>
    document.addEventListener('DOMContentLoaded', () => {
      const burger = document.querySelector('.navbar-burger');
      if (burger) {
        burger.addEventListener('click', () => {
          burger.classList.toggle('is-active');
          document.getElementById(burger.dataset.target).classList.toggle('is-active');
        });
      }
    });
  </script>
</body>
</html>`
