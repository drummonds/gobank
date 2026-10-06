package main

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

// defaultMemoryLimit is the auto-stop threshold when GOBANK_MEMORY_LIMIT is
// unset: what a browser tab can hold, so the WASM build keeps it. Deployed
// instances are sized to their box by the deployer.
const defaultMemoryLimit = 800 << 20

// parseMemoryLimit reads a size such as "800MB", "1.5GB", "2GiB" or a bare
// byte count. Empty means the default.
func parseMemoryLimit(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultMemoryLimit, nil
	}
	units := []struct {
		suffix string
		mult   float64
	}{
		{"GIB", 1 << 30}, {"GB", 1 << 30}, {"MIB", 1 << 20}, {"MB", 1 << 20},
	}
	upper := strings.ToUpper(s)
	mult := 1.0
	num := upper
	for _, u := range units {
		if before, ok := strings.CutSuffix(upper, u.suffix); ok {
			mult, num = u.mult, before
			break
		}
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(num), 64)
	if err != nil || v <= 0 || math.IsInf(v, 0) {
		return 0, fmt.Errorf("memory limit %q: want a size like 800MB or 6GB", s)
	}
	return uint64(v * mult), nil
}

// memoryLimitFromEnv is the configured auto-stop threshold, or an error the
// caller should refuse to start on.
func memoryLimitFromEnv() (uint64, error) {
	return parseMemoryLimit(os.Getenv("GOBANK_MEMORY_LIMIT"))
}

// SetMemoryLimit sets the heap size at which the simulation pauses itself,
// and tells the GC to try to stay under it with a little headroom.
func (ds *DemoState) SetMemoryLimit(limit uint64) {
	ds.mu.Lock()
	ds.memoryLimit = limit
	ds.mu.Unlock()
	if limit < math.MaxInt64/9*8 {
		debug.SetMemoryLimit(int64(limit + limit/8))
	}
}

// MemoryLimit is the configured auto-stop threshold.
func (ds *DemoState) MemoryLimit() uint64 {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	return ds.memoryLimit
}

// memCheckInterval is how often, in simulated days, the heap is checked.
const memCheckInterval = 10

// memoryPause is asked by the simulation before each day: every
// memCheckInterval days the heap is checked against the limit, and when it
// is over the run stops and the console says so.
func (ds *DemoState) memoryPause() bool {
	ds.mu.Lock()
	limit, exceeded, dayCount := ds.memoryLimit, ds.memoryExceeded, ds.dayCount
	ds.mu.Unlock()
	if exceeded {
		return true
	}
	if dayCount%memCheckInterval != 0 || !heapExceeds(limit) {
		return false
	}
	ds.mu.Lock()
	ds.memoryExceeded = true
	ds.mu.Unlock()
	return true
}

// heapExceeds reports whether the live heap is above limit.
func heapExceeds(limit uint64) bool {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc > limit
}
