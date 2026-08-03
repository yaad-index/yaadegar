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

// MinPasswordLen is the shared password policy's minimum length in bytes (ADR-0011
// §4). The policy is defined once here so every password-mutation path — the
// create-owner and set-password CLIs today, change-password and reset later —
// enforces the same floor. It is a minimum, not a strength meter: length is the
// single highest-signal rule, and pairing it with the argon2id hashing already in
// place is the pragmatic baseline.
const MinPasswordLen = 8

// ErrPasswordTooShort is returned by ValidatePasswordPolicy for a password below
// MinPasswordLen. Callers surface it to the operator/user; it names the rule that
// failed without echoing the password.
var ErrPasswordTooShort = fmt.Errorf("auth: password must be at least %d characters", MinPasswordLen)

// ValidatePasswordPolicy enforces the shared password policy (ADR-0011 §4). It is
// the one place the rules live; extend it (not each call site) as the policy grows.
func ValidatePasswordPolicy(plaintext string) error {
	if len(plaintext) < MinPasswordLen {
		return ErrPasswordTooShort
	}
	return nil
}

// HashNewPassword is the single funnel every password-setting path routes through
// (ADR-0011 §4): it applies the shared policy, then hashes. No caller hashes a new
// password directly — going through here is what guarantees a password can never be
// stored in violation of the policy. The credential_version bump is the storage
// layer's half of the same invariant (SetPasswordHash / a fresh Create).
func HashNewPassword(plaintext string) (string, error) {
	if err := ValidatePasswordPolicy(plaintext); err != nil {
		return "", err
	}
	return HashPassword(plaintext)
}

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

// dummyHash is a valid argon2id hash used only to spend the same verification cost
// on the account-not-found path as on a wrong-password path, so login response
// timing cannot distinguish an unknown account from a known one (#62).
var dummyHash, _ = HashPassword("yaadegar-constant-time-equalizer")

// VerifyDummy runs a password verification against a throwaway hash and discards
// the result. Callers use it on the account-not-found branch of login so the
// timing matches the found-but-wrong-password branch.
func VerifyDummy(plaintext string) {
	_, _ = VerifyPassword(plaintext, dummyHash)
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
