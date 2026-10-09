//go:build !(js && wasm)

package main

import "os"

// appPassword is the one password every customer logs in to the app with:
// the deployment sets it (gobank-deploy, per environment). Empty means
// app login is off.
func appPassword() string { return os.Getenv("GOBANK_APP_PASSWORD") }

// loginNote is the note under the login form; a server says nothing
// beyond the BFF's default.
func loginNote() string { return "" }

// adminPassword is the first admin's password on the staff web: the
// deployment sets it (gobank-deploy, per environment). Empty means nobody
// signs in to the staff web.
func adminPassword() string { return os.Getenv("GOBANK_ADMIN_PASSWORD") }

// staffLoginNote is the note under the staff login form; a server says
// nothing.
func staffLoginNote() string { return "" }
