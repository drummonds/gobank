package main

import (
	"database/sql"
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
	"strings"
)

// BuildRuntimeHTML renders runtime stats about the banking model.
func (ds *DemoState) BuildRuntimeHTML() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	ds.mu.Lock()
	currentDay := ds.currentDay.Format("2 Jan 2006")
	dayCount := ds.dayCount
	customerCount := len(ds.customers)
	productCount := len(ds.products)
	paymentCount := len(ds.payments)
	boeRate := ds.settings.BoEBaseRate * 100
	piiCount := ds.custStoreCount()
	dbBackend := ds.dbBackend
	if dbBackend == "" {
		dbBackend = "none (open failed)"
	}
	var dbStats sql.DBStats
	if ds.db != nil {
		dbStats = ds.db.Stats()
	}
	ds.mu.Unlock()

	var s strings.Builder

	s.WriteString(`<h2 class="title is-4">Runtime</h2>`)
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
	s.WriteString(`<p class="has-text-grey mb-3">Go runtime memory statistics from <code>runtime.MemStats</code>.</p>`)
	s.WriteString(`<table class="table is-fullwidth">`)
	s.WriteString(fmt.Sprintf(`<tr><th>Alloc</th><td>%s</td><td class="has-text-grey">Heap memory in use by live objects</td></tr>`, formatBytes(m.Alloc)))
	s.WriteString(fmt.Sprintf(`<tr><th>TotalAlloc</th><td>%s</td><td class="has-text-grey">Cumulative bytes allocated (never decreases)</td></tr>`, formatBytes(m.TotalAlloc)))
	s.WriteString(fmt.Sprintf(`<tr><th>Sys</th><td>%s</td><td class="has-text-grey">Total memory obtained from the OS</td></tr>`, formatBytes(m.Sys)))
	s.WriteString(fmt.Sprintf(`<tr><th>HeapObjects</th><td>%d</td><td class="has-text-grey">Number of allocated heap objects</td></tr>`, m.HeapObjects))
	s.WriteString(fmt.Sprintf(`<tr><th>NumGC</th><td>%d</td><td class="has-text-grey">Completed garbage collection cycles</td></tr>`, m.NumGC))
	s.WriteString(fmt.Sprintf(`<tr><th>Goroutines</th><td>%d</td><td class="has-text-grey">Active goroutines</td></tr>`, runtime.NumGoroutine()))

	// Memory limits
	gcLimit := debug.SetMemoryLimit(-1) // read without changing
	if gcLimit > 0 && gcLimit < math.MaxInt64 {
		s.WriteString(fmt.Sprintf(`<tr><th>GC memory limit</th><td>%s</td><td class="has-text-grey">Advisory limit from <code>debug.SetMemoryLimit</code></td></tr>`, formatBytes(uint64(gcLimit))))
	}
	s.WriteString(fmt.Sprintf(`<tr><th>Auto-stop threshold</th><td>%s</td><td class="has-text-grey">Simulation pauses when heap exceeds this</td></tr>`, formatBytes(memoryLimitBytes)))
	ds.mu.Lock()
	exceeded := ds.memoryExceeded
	ds.mu.Unlock()
	if exceeded {
		s.WriteString(`<tr><th>Status</th><td><span class="tag is-danger">Memory exceeded — simulation paused</span></td><td></td></tr>`)
	}

	s.WriteString(`</table></div>`)

	// Data store
	s.WriteString(`<div class="box">`)
	s.WriteString(`<h3 class="title is-5">Data Store</h3>`)
	s.WriteString(`<table class="table is-fullwidth">`)
	s.WriteString(fmt.Sprintf(`<tr><th>Type</th><td>%s</td></tr>`, dbBackend))
	s.WriteString(fmt.Sprintf(`<tr><th>PII records (encrypted)</th><td>%d</td></tr>`, piiCount))
	s.WriteString(fmt.Sprintf(`<tr><th>Max open connections</th><td>%d</td></tr>`, dbStats.MaxOpenConnections))
	s.WriteString(fmt.Sprintf(`<tr><th>Open connections</th><td>%d</td><td class="has-text-grey">In use: %d, Idle: %d</td></tr>`, dbStats.OpenConnections, dbStats.InUse, dbStats.Idle))
	s.WriteString(fmt.Sprintf(`<tr><th>Wait count</th><td>%d</td><td class="has-text-grey">Connections waited for due to pool limit</td></tr>`, dbStats.WaitCount))
	if dbStats.MaxIdleClosed > 0 || dbStats.MaxLifetimeClosed > 0 {
		s.WriteString(fmt.Sprintf(`<tr><th>Connections closed</th><td>idle: %d, lifetime: %d</td><td class="has-text-grey">Closed by pool maintenance</td></tr>`, dbStats.MaxIdleClosed, dbStats.MaxLifetimeClosed))
	}
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
