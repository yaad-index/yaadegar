// Package token mints opaque capability tokens and hashes them for storage. The
// raw token is shown to a client once; only its hash is ever persisted (ADR-0003
// §3), so a leaked database yields no usable tokens. Used for reservation and
// contribution capability tokens, for the one-click decay-release token, and for
// personal access tokens (ADR-0016).
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
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

// PATPrefix marks a personal access token (ADR-0016 §1). It routes a bearer
// credential to the token check without parsing it as a JWT, and makes a leaked
// token findable by secret scanners.
const PATPrefix = "ydg_pat_"

// NewPAT mints a personal access token: PATPrefix followed by a random token. It
// returns the raw value (to show once) and the hash of the whole raw value (the
// only value to store).
func NewPAT() (raw, hash string, err error) {
	r, _, err := New()
	if err != nil {
		return "", "", err
	}
	raw = PATPrefix + r
	return raw, Hash(raw), nil
}

// IsPAT reports whether a bearer credential carries the personal access token
// prefix.
func IsPAT(credential string) bool {
	return strings.HasPrefix(credential, PATPrefix)
}
