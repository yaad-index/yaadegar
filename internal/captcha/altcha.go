package captcha

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	altchalib "github.com/altcha-org/altcha-lib-go"
)

// ProviderAltcha is the self-hosted proof-of-work provider (ADR-0013 cut 2). Unlike
// the managed token providers it makes no outbound call and needs no site key: the
// server issues an HMAC-signed challenge, the browser solves the proof-of-work, and
// Verify re-derives the solution and checks the signature locally with the shared
// secret as the HMAC key — zero third parties.
const ProviderAltcha = "altcha"

// altchaChallengeTTL bounds how long an issued challenge stays solvable. It also caps
// the window a redeemed solution must be remembered against replay (#182): Verify
// checks signature + solution + expiry and then spends the nonce single-use, and the
// used-nonce store drops each entry once this TTL lapses. Generous enough for a human
// to complete the sub-second solve and submit the reserve.
const altchaChallengeTTL = 5 * time.Minute

// altchaMaxNumber is the proof-of-work search space: the secret number is drawn from
// [0, altchaMaxNumber]. Sized so the browser solve stays well under a second while
// still costing a bot real work on every reserve attempt.
const altchaMaxNumber = 50_000

// Challenge is a server-issued, HMAC-signed proof-of-work challenge for the browser
// widget. Its fields mirror the Altcha challenge shape; the api layer maps it to the
// wire type, so this package needn't know the generated schema and the api package
// needn't know the Altcha library.
type Challenge struct {
	Algorithm string
	Challenge string
	MaxNumber int64
	Salt      string
	Signature string
}

// Challenger is implemented by verifiers whose scheme requires a server-issued
// challenge before the client can produce a token — the Altcha proof-of-work flow.
// The managed token providers do not implement it: their challenge is issued by the
// vendor SDK in the browser, so the challenge endpoint reports 404 for them.
type Challenger interface {
	// Challenge returns a freshly signed challenge for the widget to solve.
	Challenge(ctx context.Context) (Challenge, error)
}

// usedSolutionStore records the nonces of already-redeemed solutions so each solved
// challenge can be spent at most once (#182). The default is in-memory (memUsedStore),
// which suits the single-instance deployment; the interface is the seam for a shared
// or DB-backed store should an operator ever run several instances behind one secret.
type usedSolutionStore interface {
	// consume records nonce (dropping it once expiry passes) and reports whether it
	// was already present and still live as of now — i.e. this submission is a replay.
	consume(nonce string, expiry, now time.Time) bool
}

// memUsedStore is the in-memory usedSolutionStore: a nonce→expiry map guarded by a
// mutex. Entries are swept lazily on each consume, so the map never holds more than
// the challenges live within one TTL window — no background goroutine, no unbounded
// growth.
type memUsedStore struct {
	mu   sync.Mutex
	seen map[string]time.Time // nonce -> when the entry may be dropped
}

func newMemUsedStore() *memUsedStore {
	return &memUsedStore{seen: make(map[string]time.Time)}
}

func (s *memUsedStore) consume(nonce string, expiry, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, exp := range s.seen {
		if !exp.After(now) {
			delete(s.seen, k)
		}
	}
	if _, ok := s.seen[nonce]; ok {
		return true // still-live entry means this nonce was already redeemed
	}
	s.seen[nonce] = expiry
	return false
}

// altchaVerifier implements both Verifier and Challenger for the Altcha proof-of-work
// scheme. It holds the HMAC key (the instance secret) and a store of spent solution
// nonces; everything else is derived per challenge, and verification is fully local.
type altchaVerifier struct {
	hmacKey string
	used    usedSolutionStore
	now     func() time.Time // injectable for tests; defaults to time.Now
}

// Challenge issues a new signed challenge with a bounded expiry. The library embeds
// the expiry in the salt and signs the whole challenge, so Verify can enforce it
// without any server-side challenge store.
func (a *altchaVerifier) Challenge(_ context.Context) (Challenge, error) {
	expires := a.now().Add(altchaChallengeTTL)
	c, err := altchalib.CreateChallenge(altchalib.ChallengeOptions{
		HMACKey:   a.hmacKey,
		MaxNumber: altchaMaxNumber,
		Expires:   &expires,
	})
	if err != nil {
		return Challenge{}, fmt.Errorf("captcha: create altcha challenge: %w", err)
	}
	return Challenge{
		Algorithm: c.Algorithm,
		Challenge: c.Challenge,
		MaxNumber: c.MaxNumber,
		Salt:      c.Salt,
		Signature: c.Signature,
	}, nil
}

// Verify checks the base64 solution payload the widget submits: it re-derives the
// challenge from the payload's salt+number, confirms it matches, checks the HMAC
// signature against our key (constant-time), rejects an expired challenge, and spends
// the solution single-use so a valid payload cannot be replayed within its TTL
// (#182) — all locally, with no outbound call. remoteIP is unused: Altcha is
// self-contained.
func (a *altchaVerifier) Verify(_ context.Context, token, _ string) error {
	ok, err := altchalib.VerifySolutionSafe(token, a.hmacKey, true)
	if err != nil {
		return fmt.Errorf("captcha: altcha verify: %w", err)
	}
	if !ok {
		return fmt.Errorf("captcha: altcha solution rejected")
	}
	// The solution is valid and unexpired; now enforce single use. The signature is
	// the server-minted, per-challenge HMAC — unforgeable and unique to this issued
	// challenge — so it is the replay nonce. Its own expiry bounds how long we remember
	// it. A second submission of the same signed challenge is refused.
	nonce, expiry, err := a.solutionNonce(token)
	if err != nil {
		return fmt.Errorf("captcha: altcha verify: %w", err)
	}
	if a.used.consume(nonce, expiry, a.now()) {
		return fmt.Errorf("captcha: altcha solution already redeemed")
	}
	return nil
}

// solutionNonce extracts the replay nonce (the challenge signature) and its expiry
// from a solution payload that has already passed VerifySolutionSafe. The expiry is
// the challenge's own, embedded in the salt's expires parameter; if it is missing we
// fall back to now+TTL so the entry is still bounded.
func (a *altchaVerifier) solutionNonce(token string) (nonce string, expiry time.Time, err error) {
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", time.Time{}, err
	}
	var p altchalib.Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", time.Time{}, err
	}
	expiry = a.now().Add(altchaChallengeTTL)
	if e := altchalib.ExtractParams(p).Get("expires"); e != "" {
		if ts, perr := strconv.ParseInt(e, 10, 64); perr == nil {
			expiry = time.Unix(ts, 0)
		}
	}
	return p.Signature, expiry, nil
}
