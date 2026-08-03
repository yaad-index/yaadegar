-- Password-lifecycle session invalidation (ADR-0011). Each user carries a
-- credential_version that every password mutation increments; the issued session
-- JWT pins the value it was minted at, and the owner middleware rejects a token
-- whose version no longer matches the stored one. That turns any password change,
-- reset, or operator set-password into immediate revocation of all prior sessions.
--   credential_version — starts at 1; additive, so existing accounts keep every
--                        current session invalid only once the next password write
--                        moves the counter.
ALTER TABLE users ADD COLUMN credential_version INTEGER NOT NULL DEFAULT 1;
