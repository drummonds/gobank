package users

import (
	"strings"
	"testing"
)

// A password hashes to argon2id at the settled parameters (the OWASP
// minimum: 19 MiB, t=2, p=1), with a fresh salt each time, and verifies
// only against itself.
func TestPasswordHashIsArgon2idAtTheSettledParameters(t *testing.T) {
	h1, err := hashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h1, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("hash = %q, want argon2id at m=19456,t=2,p=1", h1)
	}
	h2, _ := hashPassword("correct horse")
	if h1 == h2 {
		t.Error("two hashes of one password are the same: no salt")
	}
	if !verifyPassword(h1, "correct horse") || !verifyPassword(h2, "correct horse") {
		t.Error("the password does not verify against its own hash")
	}
	if verifyPassword(h1, "correct horsE") || verifyPassword(h1, "") {
		t.Error("another password verifies")
	}
	for _, bad := range []string{"", "$bcrypt$x", "$argon2id$v=19$m=19456,t=2,p=1$notb64!$x", h1[:len(h1)-4]} {
		if verifyPassword(bad, "correct horse") {
			t.Errorf("verifyPassword(%q) = true, want false", bad)
		}
	}
}
