package main

import (
	"strings"
	"testing"
)

func TestParseMemoryLimit(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
	}{
		{"", defaultMemoryLimit},
		{"800MB", 800 << 20},
		{"4096MB", 4096 << 20},
		{"6GB", 6 << 30},
		{"1.5GB", 3 << 29},
		{"2GiB", 2 << 30},
		{"1048576", 1 << 20},
	}
	for _, c := range cases {
		got, err := parseMemoryLimit(c.in)
		if err != nil || got != c.want {
			t.Errorf("parseMemoryLimit(%q) = %d, %v; want %d", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"lots", "1TB", "-5MB", "0"} {
		if _, err := parseMemoryLimit(bad); err == nil {
			t.Errorf("parseMemoryLimit(%q) should fail", bad)
		}
	}
}

func TestMemoryLimitStopsSimulation(t *testing.T) {
	ds := NewDemoState()
	addFundedCustomer(ds)
	for range memCheckInterval {
		ds.AdvanceDay()
	}
	if ds.DashboardData().MemoryExceeded {
		t.Fatal("default limit should not trip on a tiny simulation")
	}

	ds.SetMemoryLimit(1) // one byte: any heap exceeds it
	for range memCheckInterval {
		ds.AdvanceDay()
	}
	if !ds.DashboardData().MemoryExceeded {
		t.Fatal("a 1-byte limit must pause the simulation at the next check")
	}
}

func TestAboutShowsConfiguredMemoryLimit(t *testing.T) {
	ds := NewDemoState()
	ds.SetMemoryLimit(6 << 30)
	html := ds.BuildRuntimeHTML()
	if want := formatBytes(6 << 30); !strings.Contains(html, want) {
		t.Errorf("runtime page should show the %s auto-stop threshold:\n%s", want, html)
	}
}
