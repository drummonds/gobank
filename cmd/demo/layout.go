//go:build !(js && wasm)

package main

// LayoutModelBank is a custom navbar layout with multi-page navigation.
const LayoutModelBank = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Model Bank</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bulma@1.0.4/css/bulma.min.css">
  <script src="https://unpkg.com/htmx.org@2.0.4"></script>
</head>
<body>
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
        <a class="navbar-item" href="/settings">Settings</a>
        <div class="navbar-item has-dropdown is-hoverable">
          <a class="navbar-link">Reports</a>
          <div class="navbar-dropdown">
            <a class="navbar-item" href="/reports/bbsi">BBSI</a>
            <a class="navbar-item" href="/reports/customer-view">Customer View</a>
          </div>
        </div>
        <div class="navbar-item has-dropdown is-hoverable">
          <a class="navbar-link">About</a>
          <div class="navbar-dropdown">
            <a class="navbar-item" href="/about">About</a>
            <a class="navbar-item" href="/about/models">Models</a>
          </div>
        </div>
      </div>
      <div class="navbar-end">
        <div class="navbar-item">
          <span class="tag {% if polling == "Running" %}is-warning{% else %}is-success{% endif %}">{{ polling }}</span>
        </div>
      </div>
    </div>
  </nav>
  <section class="section">
    <div class="container">
      <div id="results"
        {% if polling == "Running" %}
        hx-get="{{ request.URL.Path }}" hx-trigger="every 1s" hx-swap="innerHTML"
        {% endif %}
      >{{ results | safe }}</div>
    </div>
  </section>
  <footer class="footer">
    <div class="content has-text-centered">
      <p>{{ version }}</p>
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
