//go:build !(js && wasm)

package main

// LayoutBankApp is a phone-frame layout for the customer-facing bank app.
const LayoutBankApp = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Model Bank App</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bulma@1.0.4/css/bulma.min.css">
  <script src="https://unpkg.com/htmx.org@2.0.4"></script>
  <style>
    body { background: #2c2c2e; margin: 0; display: flex; justify-content: center; align-items: center; min-height: 100vh; }
    .phone-frame {
      width: 375px; height: 812px;
      border-radius: 44px;
      border: 6px solid #1c1c1e;
      background: #fff;
      box-shadow: 0 20px 60px rgba(0,0,0,0.4);
      overflow: hidden;
      position: relative;
      display: flex; flex-direction: column;
    }
    .phone-notch {
      width: 150px; height: 28px;
      background: #1c1c1e;
      border-radius: 0 0 18px 18px;
      margin: 0 auto;
      position: relative; z-index: 10;
    }
    .phone-header {
      background: #00947e;
      color: #fff;
      padding: 8px 16px 12px;
      text-align: center;
    }
    .phone-header .title { color: #fff; margin: 0; font-size: 1.1rem; }
    .phone-header .subtitle { color: rgba(255,255,255,0.8); margin: 0; font-size: 0.8rem; }
    .phone-body {
      flex: 1;
      overflow-y: auto;
      padding: 12px 16px;
      -webkit-overflow-scrolling: touch;
    }
    .phone-nav {
      background: #fafafa;
      border-top: 1px solid #eee;
      display: flex;
      padding: 8px 0 12px;
    }
    .phone-nav a {
      flex: 1; text-align: center; color: #7a7a7a; font-size: 0.7rem; text-decoration: none;
    }
    .phone-nav a.is-active { color: #00947e; font-weight: bold; }
    .phone-nav a span { display: block; font-size: 1.2rem; }
  </style>
</head>
<body>
  <div class="phone-frame">
    <div class="phone-notch"></div>
    {{ .results }}
  </div>
</body>
</html>`
