package main

import "git.bytestone.uk/hum3/gobank/bff"

// The sessions component: the customer app's live sessions, kept in the
// database by the BFF (ADR-0002 stage 2, story f) so a restart or an
// upgrade keeps customers logged in, and from story 1.7.1 the staff web's
// beside them. The BFF owns the tables' code (bff.SQLSessions); the demo
// registers their schema so they are versioned with the rest. Each
// migration only adds a table.
var sessionsSchema = componentSchema{Name: "sessions", Migrations: []migration{
	{Version: 1, Statements: bff.SessionsSchema},
	{Version: 2, Statements: bff.StaffSessionsSchema},
}}
