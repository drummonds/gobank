//go:build !(js && wasm)

package main

import (
	"log/slog"

	"git.bytestone.uk/hum3/gobank/bff"
)

// newAppBFF builds the customer BFF over the demo's core adapter (ADR-0002
// stage 1). It is mounted on the demo's own port under /v1/ — the BFF's
// routes are root-relative /v1/... already — so the deployment opens no
// new port. password is GOBANK_APP_PASSWORD; empty leaves the routes up
// with every login refused.
func newAppBFF(ds *DemoState, password string, log *slog.Logger) *bff.Server {
	a := newCoreAdapter(ds, password)
	return bff.NewServer(bff.Config{Bank: a, Auth: a, Logger: log})
}
