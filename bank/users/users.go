// Package users is who may use the bank (ADR-0002 stage 7, story 1.7.1):
// a user is a login name, a password hash and the roles it holds, and
// names the customer it is when it is one (story 1.7.2). Staff and
// customers are one table told apart by role. It owns users and
// user_roles and reads nothing else of the bank.
//
// A user is representable without a password (the hash is nullable), so
// passkeys are a later table beside this one rather than a rework; the
// user's identity is its UUID v4 key, never its login name.
package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"git.bytestone.uk/hum3/gobank/bank/schema"
	"git.bytestone.uk/hum3/gobank/core"
)

// Admin is the role the first admin holds; the staff web knows the rest.
const Admin = "admin"

// ErrLoginTaken is a login name another user already holds.
var ErrLoginTaken = errors.New("users: login taken")

// Schema: the users and the roles each holds.
var Schema = schema.Component{Name: "users", Migrations: []schema.Migration{
	{Version: 1, Statements: []string{
		`CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			login VARCHAR(100) NOT NULL UNIQUE,
			password_hash VARCHAR(255),
			created_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS user_roles (
			user_id UUID NOT NULL,
			role VARCHAR(20) NOT NULL,
			PRIMARY KEY (user_id, role)
		)`,
	}},
}}

// Users is the component over the bank's database.
type Users struct {
	db *sql.DB

	// dummy is a hash no password matches, verified against when a login
	// is unknown or has no password, so a refusal costs the same either
	// way and the timing does not enumerate logins.
	dummyOnce sync.Once
	dummy     string
}

// Open is the component over db, whose tables are migrated (Schema).
func Open(db *sql.DB) *Users { return &Users{db: db} }

// Create adds a user with a password and the roles it holds, and returns
// it. A login another user holds is ErrLoginTaken.
func (u *Users) Create(ctx context.Context, login, password string, roles ...string) (core.User, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return core.User{}, err
	}
	tx, err := u.db.BeginTx(ctx, nil)
	if err != nil {
		return core.User{}, fmt.Errorf("users: create: %w", err)
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE login = $1`, login).Scan(&exists); err != nil {
		return core.User{}, fmt.Errorf("users: create: %w", err)
	}
	if exists > 0 {
		return core.User{}, ErrLoginTaken
	}
	var id string
	if err := tx.QueryRowContext(ctx, `INSERT INTO users (login, password_hash, created_at) VALUES ($1, $2, $3) RETURNING id`,
		login, hash, time.Now().UTC()).Scan(&id); err != nil {
		return core.User{}, fmt.Errorf("users: create: %w", err)
	}
	for _, role := range roles {
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, $2)`, id, role); err != nil {
			return core.User{}, fmt.Errorf("users: create: role %s: %w", role, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return core.User{}, fmt.Errorf("users: create: %w", err)
	}
	return core.User{ID: id, Login: login, Roles: append([]string(nil), roles...)}, nil
}

// FirstAdmin is the admin the deployment provides: the user "admin",
// holding the admin role, with this password. Created when absent; an
// admin on record gets the password, since it is the deployment's secret
// and a database restored from another environment keeps that
// environment's.
func (u *Users) FirstAdmin(ctx context.Context, password string) error {
	_, err := u.Create(ctx, Admin, password, Admin)
	if !errors.Is(err, ErrLoginTaken) {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	if _, err := u.db.ExecContext(ctx, `UPDATE users SET password_hash = $1 WHERE login = $2`, hash, Admin); err != nil {
		return fmt.Errorf("users: first admin: %w", err)
	}
	return nil
}

// AuthenticateUser implements core.UserAuthenticator.
func (u *Users) AuthenticateUser(ctx context.Context, login, password string) (core.User, error) {
	var id string
	var hash sql.NullString
	err := u.db.QueryRowContext(ctx, `SELECT id, password_hash FROM users WHERE login = $1`, login).Scan(&id, &hash)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		verifyPassword(u.dummyHash(), password)
		return core.User{}, core.ErrBadCredentials
	case err != nil:
		return core.User{}, fmt.Errorf("users: authenticate: %w", err)
	}
	if !hash.Valid {
		verifyPassword(u.dummyHash(), password)
		return core.User{}, core.ErrBadCredentials
	}
	if !verifyPassword(hash.String, password) {
		return core.User{}, core.ErrBadCredentials
	}
	roles, err := u.roles(ctx, id)
	if err != nil {
		return core.User{}, err
	}
	return core.User{ID: id, Login: login, Roles: roles}, nil
}

// User is the user with the given ID; an unknown ID is core.ErrNotFound.
func (u *Users) User(ctx context.Context, id string) (core.User, error) {
	var login string
	err := u.db.QueryRowContext(ctx, `SELECT login FROM users WHERE id = $1`, id).Scan(&login)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return core.User{}, core.ErrNotFound
	case err != nil:
		return core.User{}, fmt.Errorf("users: user: %w", err)
	}
	roles, err := u.roles(ctx, id)
	if err != nil {
		return core.User{}, err
	}
	return core.User{ID: id, Login: login, Roles: roles}, nil
}

func (u *Users) roles(ctx context.Context, id string) ([]string, error) {
	rows, err := u.db.QueryContext(ctx, `SELECT role FROM user_roles WHERE user_id = $1 ORDER BY role`, id)
	if err != nil {
		return nil, fmt.Errorf("users: roles: %w", err)
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, fmt.Errorf("users: roles: %w", err)
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

func (u *Users) dummyHash() string {
	u.dummyOnce.Do(func() { u.dummy, _ = hashPassword("") })
	return u.dummy
}

var _ core.Users = (*Users)(nil)
