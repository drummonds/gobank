package main

import "git.bytestone.uk/hum3/gobank/bff"

// The sessions component: the customer app's live sessions, kept in the
// database by the BFF (ADR-0002 stage 2, story f) so a restart or an
// upgrade keeps customers logged in. The BFF owns the table's code
// (bff.SQLSessions); the demo registers its schema so the table is
// versioned with the rest. The migration only adds the table.
var sessionsSchema = componentSchema{Name: "sessions", Migrations: []migration{
	{Version: 1, Statements: bff.SessionsSchema},
}}
