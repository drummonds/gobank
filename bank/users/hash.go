package users

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Passwords are hashed with argon2id at the OWASP minimum (settled
// 2026-10-09: 19 MiB, t=2, p=1; p stays 1 because the tab has one thread)
// and stored in the PHC string format, so the parameters travel with the
// hash and can be raised later without rehashing what is stored.
const (
	argonMemory  uint32 = 19 * 1024 // KiB
	argonTime    uint32 = 2
	argonThreads uint8  = 1
	saltLen             = 16
	keyLen       uint32 = 32
)

var b64 = base64.RawStdEncoding

// hashPassword is the PHC string of a fresh argon2id hash of password.
func hashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("users: salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// verifyPassword reports whether password is the one encoded hashes, at
// the parameters the hash carries. Anything that is not an argon2id PHC
// string verifies nothing.
func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil || memory == 0 || time == 0 || threads == 0 {
		return false
	}
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false
	}
	key, err := b64.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1
}
