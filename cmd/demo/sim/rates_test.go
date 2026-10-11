package sim

import (
	"testing"
	"time"
)

// The series is replayed as the Bank of England published it (ADR-0009):
// each change a publication of GBP's rate in basis points at noon London
// time on its day, oldest first.
func TestTheSeriesIsPublicationsAtNoonLondon(t *testing.T) {
	pubs := parsePublications("2020-03-11,0.0025\n2020-01-01,0.0075\n\nbad line\n2022-02-03,0.0050\n")
	if len(pubs) != 3 {
		t.Fatalf("%d publications, want 3: %+v", len(pubs), pubs)
	}
	london, _ := time.LoadLocation("Europe/London")
	for i, want := range []struct {
		day time.Time
		bps int64
	}{
		{time.Date(2020, 1, 1, 12, 0, 0, 0, london), 75},
		{time.Date(2020, 3, 11, 12, 0, 0, 0, london), 25},
		{time.Date(2022, 2, 3, 12, 0, 0, 0, london), 50},
	} {
		p := pubs[i]
		if p.Currency != "GBP" || p.Bps != want.bps || !p.At.Equal(want.day) {
			t.Errorf("publication %d = %+v, want GBP %d at %v", i, p, want.bps, want.day)
		}
	}
	if len(BoERates) < 20 || BoERates[0].Bps != 75 {
		t.Errorf("the embedded series: %d publications, first %+v", len(BoERates), BoERates[0])
	}
}

// The rate in force on a day is the last publication on or before it:
// what the simulation publishes at reset for the opening day.
func TestRateInForce(t *testing.T) {
	pubs := parsePublications("2020-01-01,0.0075\n2020-03-11,0.0025\n")
	if bps, ok := pubs.InForce(time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC)); !ok || bps != 75 {
		t.Errorf("February 2020 = %d, %v; want 75", bps, ok)
	}
	if bps, ok := pubs.InForce(time.Date(2020, 3, 11, 0, 0, 0, 0, time.UTC)); !ok || bps != 75 {
		t.Errorf("the morning of a decision day = %d, %v; want the old 75", bps, ok)
	}
	if bps, ok := pubs.InForce(time.Date(2020, 3, 11, 13, 0, 0, 0, time.UTC)); !ok || bps != 25 {
		t.Errorf("the afternoon of a decision day = %d, %v; want 25", bps, ok)
	}
	if _, ok := pubs.InForce(time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)); ok {
		t.Error("before the series begins there is a rate")
	}
}
