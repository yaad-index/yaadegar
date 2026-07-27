// Package auth is the owner-authentication core (ADR-0005): argon2id password
// hashing, HS256 JWT issuance/validation with algorithm pinning, the role/claims
// model, and the operator-configurable, fail-closed method configuration. It holds
// no HTTP or storage concerns — the API layer wires it in.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters — the OWASP baseline preset (ADR-0005 §3): 19 MiB memory,
// 2 iterations, 1 lane, a 16-byte salt and a 32-byte key. Fixed so hashing is not
// a per-operator tuning burden; the cost is encoded in each hash so a future bump
// stays verifiable against old hashes.
const (
	argonMemoryKiB = 19 * 1024
	argonTime      = 2
	argonParallel  = 1
	argonSaltLen   = 16
	argonKeyLen    = 32
)

// ErrInvalidHash marks a stored hash that is not a well-formed argon2id PHC string.
var ErrInvalidHash = errors.New("auth: malformed password hash")

// HashPassword returns an argon2id PHC-encoded hash of the plaintext
// (`$argon2id$v=19$m=...,t=...,p=...$salt$hash`), with a fresh random salt. The
// plaintext is never stored or logged.
func HashPassword(plaintext string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: read salt: %w", err)
	}
	key := argon2.IDKey([]byte(plaintext), salt, argonTime, argonMemoryKiB, argonParallel, argonKeyLen)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonTime, argonParallel,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports whether plaintext matches the argon2id PHC-encoded hash.
// The comparison recomputes with the hash's own encoded parameters and is
// constant-time. A malformed hash is an error, not a silent false.
func VerifyPassword(plaintext, encoded string) (bool, error) {
	params, salt, want, err := decodeArgon2idHash(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(plaintext), salt, params.time, params.memory, params.parallel, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

type argon2Params struct {
	memory   uint32
	time     uint32
	parallel uint8
}

func decodeArgon2idHash(encoded string) (argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argon2Params{}, nil, nil, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return argon2Params{}, nil, nil, ErrInvalidHash
	}
	var p argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.parallel); err != nil {
		return argon2Params{}, nil, nil, ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2Params{}, nil, nil, ErrInvalidHash
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2Params{}, nil, nil, ErrInvalidHash
	}
	return p, salt, hash, nil
}
