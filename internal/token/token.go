// Package token mints opaque capability tokens and hashes them for storage. The
// raw token is shown to a client once; only its hash is ever persisted (ADR-0003
// §3), so a leaked database yields no usable tokens. Used for reservation and
// contribution capability tokens and for the one-click decay-release token.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// New mints a random token and returns the raw value (to hand to the client once)
// and its hash (the only value to store).
func New() (raw, hash string, err error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b[:])
	return raw, Hash(raw), nil
}

// Hash returns the storage/lookup hash of a raw token.
func Hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
