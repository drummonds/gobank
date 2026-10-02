//go:build !(js && wasm)

package main

import (
	"log/slog"

	"git.bytestone.uk/hum3/gobank/bff"
)

// newAppBFF builds the customer BFF over the demo's core adapter (ADR-0002
// stage 1). It is mounted on the demo's own port under /v1/ — the BFF's
// routes are root-relative /v1/... already — so the deployment opens no
// new port. The adapter's password is GOBANK_APP_PASSWORD; empty leaves
// the routes up with every login refused.
func newAppBFF(bank *coreAdapter, log *slog.Logger) *bff.Server {
	return bff.NewServer(bff.Config{Bank: bank, Auth: bank, Logger: log})
}
