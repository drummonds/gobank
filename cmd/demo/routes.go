package main

import (
	"log"
	"log/slog"
	"net/http"
	"time"

	"git.bytestone.uk/hum3/gobank/bff/staff"
)

// newHandler is the demo as one http.Handler over its state: the BFF
// (ADR-0002 stage 6), which serves the customer app and web under /v1/
// and the staff web (bff/staff) everywhere else, with the simulation
// console driven through staff.Console, which DemoState implements. The
// server listens on it; the WASM build serves it in the tab from a
// service worker. scope is the path the handler is mounted under ("/" on
// the server, the service worker's scope in the tab).
func newHandler(state *DemoState, version, scope string) http.Handler {
	site := staff.New(siteConfig(state, version, scope))

	// The customer BFF under /v1/: the app's screens and the customer
	// web, which is the BFF's own HTML (story 1.6.2). The password is the
	// deployment's (GOBANK_APP_PASSWORD on a server, a fixed one in the
	// tab) and so is the session's keeping (a cookie on a server, the
	// tab's jar in the tab).
	password := appPassword()
	server := newAppBFF(state.Bank, newAppLogin(state.Bank, password), state.DB, scope, loginNote(), site, slog.Default())
	go func() {
		for range time.Tick(time.Minute) {
			server.Sessions().Sweep()
		}
	}()
	if password == "" {
		log.Printf("app BFF mounted at /v1/ with GOBANK_APP_PASSWORD unset: app login is off")
	}
	return customerSessions(server)
}

// siteConfig is the staff web over the demo: the bank (ADR-0002 stage 5)
// read and written through the core, the console, and the component
// registry its documentation page renders.
func siteConfig(state *DemoState, version, scope string) staff.Config {
	return staff.Config{
		Bank: state.Bank, Commands: state.Bank, Console: state,
		Scope: scope, Version: version,
		Components: components, Debt: contractDebt,
	}
}
