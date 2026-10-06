package main

import (
	"database/sql"
	"log/slog"

	"git.bytestone.uk/hum3/gobank/bff"
	"git.bytestone.uk/hum3/gobank/core"
)

// newAppBFF builds the customer BFF over the bank (ADR-0002 stage 1). It
// is mounted on the demo's own port under /v1/ — the BFF's routes are
// root-relative /v1/... already — so the deployment opens no new port.
// The login's password is GOBANK_APP_PASSWORD; empty leaves the routes up
// with every login refused. Sessions live in the demo's database (the
// sessions component), so a restart keeps customers logged in.
func newAppBFF(bank core.StaffQueries, auth core.Authenticator, db *sql.DB, log *slog.Logger) *bff.Server {
	return bff.NewServer(bff.Config{Bank: bank, Auth: auth, SessionDB: db, Logger: log})
}
