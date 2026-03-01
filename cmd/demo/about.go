package main

import (
	"fmt"
	"runtime"
	"strings"
)

// BuildAboutHTML renders runtime stats about the banking model.
func (ds *DemoState) BuildAboutHTML() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	ds.mu.Lock()
	currentDay := ds.currentDay.Format("2 Jan 2006")
	dayCount := ds.dayCount
	customerCount := len(ds.customers)
	productCount := len(ds.products)
	paymentCount := len(ds.payments)
	boeRate := ds.settings.BoEBaseRate * 100
	piiCount := ds.piiStore.Count()
	ds.mu.Unlock()

	var s strings.Builder

	s.WriteString(`<h2 class="title is-4">About</h2>`)
	s.WriteString(`<p class="subtitle is-6 has-text-grey">Runtime stats for the Model Bank simulation</p>`)

	// Environment
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Environment</h3>`)
	s.WriteString(`<table class="table is-fullwidth">`)
	s.WriteString(fmt.Sprintf(`<tr><th>Version</th><td>%s</td></tr>`, version))
	s.WriteString(fmt.Sprintf(`<tr><th>Runtime</th><td>%s</td></tr>`, runtimeEnv))
	s.WriteString(fmt.Sprintf(`<tr><th>Go version</th><td>%s</td></tr>`, runtime.Version()))
	s.WriteString(fmt.Sprintf(`<tr><th>GOARCH</th><td>%s</td></tr>`, runtime.GOARCH))
	s.WriteString(fmt.Sprintf(`<tr><th>GOOS</th><td>%s</td></tr>`, runtime.GOOS))
	s.WriteString(`</table></div>`)

	// Memory
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Memory</h3>`)
	s.WriteString(`<table class="table is-fullwidth">`)
	s.WriteString(fmt.Sprintf(`<tr><th>Alloc</th><td>%s</td></tr>`, formatBytes(m.Alloc)))
	s.WriteString(fmt.Sprintf(`<tr><th>TotalAlloc</th><td>%s</td></tr>`, formatBytes(m.TotalAlloc)))
	s.WriteString(fmt.Sprintf(`<tr><th>Sys</th><td>%s</td></tr>`, formatBytes(m.Sys)))
	s.WriteString(fmt.Sprintf(`<tr><th>NumGC</th><td>%d</td></tr>`, m.NumGC))
	s.WriteString(`</table></div>`)

	// Data store
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Data Store</h3>`)
	s.WriteString(`<table class="table is-fullwidth">`)
	s.WriteString(`<tr><th>Type</th><td>In-memory</td></tr>`)
	s.WriteString(fmt.Sprintf(`<tr><th>PII records (encrypted)</th><td>%d</td></tr>`, piiCount))
	s.WriteString(`</table></div>`)

	// Simulation
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Simulation</h3>`)
	s.WriteString(`<table class="table is-fullwidth">`)
	s.WriteString(fmt.Sprintf(`<tr><th>Current day</th><td>%s</td></tr>`, currentDay))
	s.WriteString(fmt.Sprintf(`<tr><th>Days elapsed</th><td>%d</td></tr>`, dayCount))
	s.WriteString(fmt.Sprintf(`<tr><th>Customers</th><td>%d</td></tr>`, customerCount))
	s.WriteString(fmt.Sprintf(`<tr><th>Products</th><td>%d</td></tr>`, productCount))
	s.WriteString(fmt.Sprintf(`<tr><th>Payments</th><td>%d</td></tr>`, paymentCount))
	s.WriteString(fmt.Sprintf(`<tr><th>BoE base rate</th><td>%.2f%%</td></tr>`, boeRate))
	s.WriteString(`</table></div>`)

	return s.String()
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMG"[exp])
}
