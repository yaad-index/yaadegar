package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// newCapabilityToken mints a random capability token — returned to the giver
// exactly once — and its hash, which is the only value stored. The storage layer
// never sees the raw token; hashing lives here (ADR-0003 §3).
func newCapabilityToken() (raw, hash string, err error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b[:])
	return raw, hashToken(raw), nil
}

// hashToken hashes a raw capability token for storage/lookup.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
