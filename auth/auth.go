// Package auth is who can come in: the users of goddard, the links that let
// them in once and the sessions that keep them in. It owns its tables the way
// every other part owns its own, and heimdall keeps its tokens: those are
// machines asking for secrets, not people signing in.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// User is somebody with an email. The id is the ULID the database minted and
// is what the rest of goddard keeps; the email is how they sign in.
type User struct {
	ID    string
	Email string
	Name  string
}

// LoginTTL and SessionTTL are how long a link and a session last, in seconds.
const (
	LoginTTL   = 900
	SessionTTL = 30 * 24 * 60 * 60
)

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("no hay azar: %v", err)
	}
	return hex.EncodeToString(buf), nil
}

func hash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func nameOf(email string) string {
	name, _, found := strings.Cut(email, "@")
	if !found || name == "" {
		return email
	}
	return name
}
