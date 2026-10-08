package main

import (
	"database/sql"
	"log/slog"

	"git.bytestone.uk/hum3/gobank/bff"
	"git.bytestone.uk/hum3/gobank/core"
)

// newAppBFF builds the customer BFF over the bank (ADR-0002 stage 1). It
// is mounted on the demo's own port under /v1/ — the BFF's routes are
// root-relative /v1/... already — so the deployment opens no new port;
// scope is the path the demo itself is mounted under, which the BFF's
// pages, redirects and cookie carry. An empty password leaves the routes
// up with every login refused. Sessions live in the demo's database (the
// sessions component), so a restart keeps customers logged in; db gives
// the database as it stands, since a reset or an import replaces it.
func newAppBFF(bank core.StaffQueries, auth core.Authenticator, db func() *sql.DB, scope, loginNote string, log *slog.Logger) *bff.Server {
	return bff.NewServer(bff.Config{Bank: bank, Auth: auth, SessionDB: db, Scope: scope, LoginNote: loginNote, Logger: log})
}
