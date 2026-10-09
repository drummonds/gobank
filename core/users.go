package core

import "context"

// The users of the bank (ADR-0002 stage 7, story 1.7.1): who may sign in
// to the staff web and, from story 1.7.2, to the customer app.

// User is who is signed in: a login the users component holds. ID is the
// user's identity, a UUID v4, never the login name (it is the WebAuthn
// user handle when passkeys come). Roles are the names of the roles the
// user holds: a member of staff holds one of the staff roles, a customer
// holds "customer".
type User struct {
	ID    string
	Login string
	Roles []string
}

// HasRole reports whether the user holds the role.
func (u User) HasRole(role string) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// Users is the users component as the staff web reads it. AuthenticateUser
// checks a user's password: an unknown login, a wrong password and a user
// with no password set are ErrBadCredentials alike, so the response cannot
// be used to enumerate logins. User is the user a session names, by its
// ID; an unknown ID is ErrNotFound.
type Users interface {
	AuthenticateUser(ctx context.Context, login, password string) (User, error)
	User(ctx context.Context, id string) (User, error)
}
