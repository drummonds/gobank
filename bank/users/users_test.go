package users

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	_ "git.bytestone.uk/hum3/go-postgres"
	"git.bytestone.uk/hum3/gobank/core"
)

func open(t *testing.T) *Users {
	t.Helper()
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, m := range Schema.Migrations {
		for _, stmt := range m.Statements {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("schema v%d: %v", m.Version, err)
			}
		}
	}
	return Open(db)
}

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// The first admin signs in with the deployment's password and holds the
// admin role; its identity is a UUID, not its login name. An unknown
// login and a wrong password are refused alike.
func TestFirstAdminSignsIn(t *testing.T) {
	u := open(t)
	ctx := context.Background()
	if err := u.FirstAdmin(ctx, "s3cret"); err != nil {
		t.Fatal(err)
	}
	admin, err := u.AuthenticateUser(ctx, "admin", "s3cret")
	if err != nil {
		t.Fatalf("AuthenticateUser(admin): %v", err)
	}
	if !uuidV4.MatchString(admin.ID) {
		t.Errorf("admin ID = %q, want a UUID v4", admin.ID)
	}
	if admin.Login != "admin" || !admin.HasRole(Admin) || len(admin.Roles) != 1 {
		t.Errorf("admin = %+v, want login admin holding the admin role alone", admin)
	}
	for _, c := range [][2]string{{"admin", "S3CRET"}, {"admin", ""}, {"nobody", "s3cret"}, {"", ""}} {
		if _, err := u.AuthenticateUser(ctx, c[0], c[1]); !errors.Is(err, core.ErrBadCredentials) {
			t.Errorf("AuthenticateUser(%q, %q) = %v, want ErrBadCredentials", c[0], c[1], err)
		}
	}
}

// The first admin is one user whose password is the deployment's: setting
// it again changes the password, not the user.
func TestFirstAdminIsOneUserWhosePasswordFollowsTheDeployment(t *testing.T) {
	u := open(t)
	ctx := context.Background()
	if err := u.FirstAdmin(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	before, _ := u.AuthenticateUser(ctx, "admin", "first")
	if err := u.FirstAdmin(ctx, "second"); err != nil {
		t.Fatal(err)
	}
	after, err := u.AuthenticateUser(ctx, "admin", "second")
	if err != nil {
		t.Fatalf("after the password was set again: %v", err)
	}
	if after.ID != before.ID {
		t.Errorf("admin ID changed from %s to %s", before.ID, after.ID)
	}
	if _, err := u.AuthenticateUser(ctx, "admin", "first"); !errors.Is(err, core.ErrBadCredentials) {
		t.Error("the old password still signs in")
	}
	if n := u.count(t); n != 1 {
		t.Errorf("%d users, want 1", n)
	}
}

// A user is created with its roles; a login is held once.
func TestCreateHoldsEachLoginOnce(t *testing.T) {
	u := open(t)
	ctx := context.Background()
	alice, err := u.Create(ctx, "alice", "pw", "auditor", "readonly")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := u.Create(ctx, "alice", "other", Admin); !errors.Is(err, ErrLoginTaken) {
		t.Errorf("second create of alice = %v, want ErrLoginTaken", err)
	}
	got, err := u.AuthenticateUser(ctx, "alice", "pw")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != alice.ID || !got.HasRole("auditor") || !got.HasRole("readonly") || got.HasRole(Admin) {
		t.Errorf("alice = %+v, want auditor and readonly", got)
	}
	if n := u.count(t); n != 1 {
		t.Errorf("%d users, want 1", n)
	}
}

// A session names its user by ID; the user is read back by it.
func TestUserByID(t *testing.T) {
	u := open(t)
	ctx := context.Background()
	alice, err := u.Create(ctx, "alice", "pw", "cs")
	if err != nil {
		t.Fatal(err)
	}
	got, err := u.User(ctx, alice.ID)
	if err != nil || got.Login != "alice" || !got.HasRole("cs") || got.ID != alice.ID {
		t.Errorf("User(%s) = %+v, %v", alice.ID, got, err)
	}
	if _, err := u.User(ctx, "00000000-0000-4000-8000-000000000000"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("User(unknown) = %v, want ErrNotFound", err)
	}
}

func (u *Users) count(t *testing.T) int {
	t.Helper()
	var n int
	if err := u.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
