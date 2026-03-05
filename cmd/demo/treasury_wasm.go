//go:build js && wasm

package main

import "time"

// GiltHolding represents a gilt holding (stub for WASM builds).
type GiltHolding struct {
	ID           int
	Tenor        string
	FaceValue    float64
	PurchaseDate time.Time
	Yield        float64
}

// getGiltHoldings is a no-op on WASM — no DB available.
func (ds *DemoState) getGiltHoldings() []GiltHolding { return nil }
