package bff_test

import (
	"database/sql"
	"testing"
	"time"

	_ "git.bytestone.uk/hum3/go-postgres"
	"git.bytestone.uk/hum3/gobank/bff"
)

// Every session store honours the same contract: a token is good until
// the absolute lifetime or the idle timeout, whichever comes first, and
// gone once revoked or swept.
func TestSessionsContract(t *testing.T) {
	const ttl, idle = time.Hour, 10 * time.Minute
	for name, open := range map[string]func(t *testing.T, now func() time.Time) bff.Sessions{
		"memory": func(_ *testing.T, now func() time.Time) bff.Sessions { return bff.NewSessionStore(ttl, idle, now) },
		"sql": func(t *testing.T, now func() time.Time) bff.Sessions {
			return newSQLSessions(t, sessionsDB(t), ttl, idle, now)
		},
	} {
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
			s := open(t, func() time.Time { return now })

			token, expires := s.Create("cust-001")
			if token == "" || !expires.Equal(now.Add(ttl)) {
				t.Fatalf("Create = %q, %s; want a token expiring at %s", token, expires, now.Add(ttl))
			}
			if id, ok := s.Lookup(token); !ok || id != "cust-001" {
				t.Fatalf("Lookup = %q, %v; want cust-001, true", id, ok)
			}
			if _, ok := s.Lookup(token + "x"); ok {
				t.Error("a token nobody issued was accepted")
			}
			if _, ok := s.Lookup(""); ok {
				t.Error("the empty token was accepted")
			}

			// Idle: each lookup restarts the idle clock.
			now = now.Add(9 * time.Minute)
			if _, ok := s.Lookup(token); !ok {
				t.Fatal("session idled out before the idle timeout")
			}
			now = now.Add(9 * time.Minute)
			if _, ok := s.Lookup(token); !ok {
				t.Fatal("session idled out although it was used 9 minutes ago")
			}
			now = now.Add(11 * time.Minute)
			if _, ok := s.Lookup(token); ok {
				t.Fatal("session still live 11 minutes after its last use")
			}

			// Absolute lifetime, however busy.
			now = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
			token, _ = s.Create("cust-002")
			for range 7 {
				now = now.Add(9 * time.Minute)
				s.Lookup(token)
			}
			if _, ok := s.Lookup(token); ok {
				t.Fatal("session still live after its absolute lifetime")
			}

			// Revoke.
			token, _ = s.Create("cust-003")
			s.Revoke(token)
			if _, ok := s.Lookup(token); ok {
				t.Fatal("revoked session still live")
			}
			s.Revoke("never-issued") // ignored
		})
	}
}

// Sweep drops the rows of expired sessions and keeps the live ones.
func TestSQLSessionsSweep(t *testing.T) {
	const ttl, idle = time.Hour, 10 * time.Minute
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	db := sessionsDB(t)
	s := newSQLSessions(t, db, ttl, idle, func() time.Time { return now })
	live, _ := s.Create("cust-001")
	s.Create("cust-002") // never used again
	now = now.Add(idle - time.Minute)
	s.Lookup(live)
	now = now.Add(2 * time.Minute) // cust-002 idle for 11 minutes, cust-001 for 2
	s.Sweep()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows after sweep = %d, %v; want 1", n, err)
	}
	if _, ok := s.Lookup(live); !ok {
		t.Error("sweep dropped the live session")
	}
}

// Sessions kept in the database outlive the process that issued them
// (ADR-0002: every fact is stored): a restart keeps customers logged in.
func TestSQLSessionsSurviveARestart(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	db := sessionsDB(t)
	first := newSQLSessions(t, db, time.Hour, 10*time.Minute, clock)
	token, _ := first.Create("cust-001")

	second := newSQLSessions(t, db, time.Hour, 10*time.Minute, clock)
	if id, ok := second.Lookup(token); !ok || id != "cust-001" {
		t.Fatalf("after restart Lookup = %q, %v; want cust-001, true", id, ok)
	}
	second.Revoke(token)
	if _, ok := first.Lookup(token); ok {
		t.Error("a session revoked in one process is still live in another")
	}
}

// The program that owns the database may replace it (the demo's reset and
// import close the in-memory database and open a fresh one), so the store
// asks for the database on each use rather than holding a handle: the old
// run's sessions are gone with it, and new ones are kept in the new one.
func TestSQLSessionsFollowTheDatabase(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	current := sessionsDB(t)
	s := bff.NewSQLSessions(bff.CustomerSessionsTable, func() *sql.DB { return current }, time.Hour, 10*time.Minute, clock)
	old, _ := s.Create("cust-001")

	current.Close()
	current = sessionsDB(t)
	if _, ok := s.Lookup(old); ok {
		t.Error("a session of the replaced database is still live")
	}
	token, _ := s.Create("cust-002")
	if id, ok := s.Lookup(token); !ok || id != "cust-002" {
		t.Fatalf("after the database was replaced Lookup = %q, %v; want cust-002, true", id, ok)
	}
}

// The server keeps its sessions in the database it is given.
func TestServerUsesTheSessionDatabase(t *testing.T) {
	db := sessionsDB(t)
	f := newFixtureWith(t, bff.Config{SessionDB: func() *sql.DB { return db }})
	_, m := f.login("cust-001", "password")
	token := m["token"].(string)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("sessions rows after login = %d, %v; want 1", n, err)
	}
	if res, _ := f.do("GET", "/v1/screen/accounts", token, "", ""); res.StatusCode != 200 {
		t.Fatalf("screen with a stored session: %d", res.StatusCode)
	}
}

func sessionsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, stmt := range append(bff.SessionsSchema, bff.StaffSessionsSchema...) {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// Staff sessions are kept apart from customers' (story 1.7.1): a staff
// token is no customer's session and a customer's is no staff session,
// and a staff session's subject is the user's ID.
func TestStaffSessionsAreKeptApart(t *testing.T) {
	db := sessionsDB(t)
	customers := newSQLSessions(t, db, time.Hour, 10*time.Minute, nil)
	staff := bff.NewSQLSessions(bff.StaffSessionsTable, func() *sql.DB { return db }, time.Hour, 10*time.Minute, nil)
	const userID = "4b1b3b2e-2c7a-4d2e-9f1a-0f2d3c4b5a69"
	token, _ := staff.Create(userID)
	if _, ok := customers.Lookup(token); ok {
		t.Error("a staff token is a customer session")
	}
	if id, ok := staff.Lookup(token); !ok || id != userID {
		t.Errorf("staff Lookup = %q, %v; want the user ID", id, ok)
	}
	custToken, _ := customers.Create("cust-001")
	if _, ok := staff.Lookup(custToken); ok {
		t.Error("a customer token is a staff session")
	}
	staff.Revoke(token)
	if _, ok := staff.Lookup(token); ok {
		t.Error("a revoked staff session is still live")
	}
}

func newSQLSessions(t *testing.T, db *sql.DB, ttl, idle time.Duration, now func() time.Time) bff.Sessions {
	t.Helper()
	return bff.NewSQLSessions(bff.CustomerSessionsTable, func() *sql.DB { return db }, ttl, idle, now)
}
