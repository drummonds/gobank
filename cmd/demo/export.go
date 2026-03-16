//go:build !(js && wasm)

package main

import (
	"bytes"
	"io"
	"net/http"

	luca "github.com/drummonds/go-luca"
)

func (ds *DemoState) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var buf bytes.Buffer
	ds.mu.Lock()
	err := ds.sim.ExportGoluca(&buf)
	ds.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="gobank.goluca"`)
	w.Write(buf.Bytes())
}

func (ds *DemoState) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Read error", http.StatusBadRequest)
		return
	}

	ds.mu.Lock()
	err = ds.sim.Ledger.Import(bytes.NewReader(data), &luca.ImportOptions{
		AutoCreateAccounts: true,
		DefaultCurrency:    "GBP",
	})
	ds.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
