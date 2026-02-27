//go:build !(js && wasm)

package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/drummonds/lofigui"
)

func (b *BankDemo) render() {
	lofigui.HTML(b.buildSVG())

	b.mu.Lock()
	running := b.running
	b.mu.Unlock()

	// Status tag
	statusTag := `<span class="tag is-light">Stopped</span>`
	if running {
		statusTag = `<span class="tag is-success">Running</span>`
	}

	lofigui.HTML(fmt.Sprintf(`<div class="field is-grouped is-grouped-multiline mb-4">
  <div class="control">%s</div>
</div>`, statusTag))

	// Controls
	var startStopBtn string
	if running {
		startStopBtn = `<form action="/stop" method="post" style="display:inline"><button class="button is-danger" type="submit">Stop</button></form>`
	} else {
		startStopBtn = `<form action="/start" method="post" style="display:inline"><button class="button is-success" type="submit">Run</button></form>`
	}

	lofigui.HTML(fmt.Sprintf(`<div class="buttons">%s
  <form action="/advance" method="post" style="display:inline"><button class="button is-info" type="submit">Advance Day</button></form>
  <form action="/deposit" method="post" style="display:inline"><button class="button is-primary" type="submit">Deposit £100</button></form>
  <form action="/withdraw" method="post" style="display:inline"><button class="button is-warning" type="submit">Withdraw £100</button></form>
  <form action="/reset" method="post" style="display:inline"><button class="button is-light" type="submit">Reset</button></form>
</div>`, startStopBtn))
}

func main() {
	bank := NewBankDemo()

	app := lofigui.NewApp()
	app.Version = "Model Bank Demo v0.1"
	app.SetRefreshTime(1)
	app.SetDisplayURL("/")

	ctrl, err := lofigui.NewControllerWithLayout(lofigui.LayoutNavbar, "Model Bank")
	if err != nil {
		log.Fatalf("Failed to create controller: %v", err)
	}
	app.SetController(ctrl)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		lofigui.Reset()
		bank.render()
		app.HandleDisplay(w, r)
	})

	http.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		bank.Start()
		app.StartAction()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		bank.Stop()
		app.EndAction()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/advance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		bank.AdvanceDay()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/deposit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		bank.Deposit()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/withdraw", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		bank.Withdraw()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		bank.Reset()
		app.EndAction()
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	http.HandleFunc("/favicon.ico", lofigui.ServeFavicon)

	addr := ":1347"
	log.Printf("Starting Model Bank Demo on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
