-- Email self-registration (ADR-0012 cut 1a). Two parts:
--
-- 1. users.status — the account lifecycle state. Existing accounts (and every
--    operator-created one) default to 'active', so behavior is unchanged; a
--    self-registered account is inserted 'pending' and flips to 'active' when it
--    verifies its email. A pending account cannot hold a session (the login gate
--    rejects it, like a ban).
--
-- 2. email_verification_tokens — a single-use, short-TTL token minted at register
--    and emailed once; only its sha256 hash is stored (the raw token is emailed
--    once, ADR-0003 §3), so a leaked database yields no usable tokens. Verify claims
--    the token atomically (used_at set only while NULL) and activates the account.
--    Same shape as password_reset_tokens but a distinct table and lifecycle.
--   token_hash — sha256 of the raw token; the lookup key. Globally unique.
--   expires_at — hard expiry; a token past it never validates (checked in Go).
--   used_at    — NULL until claimed; the single-use guard (the claim UPDATE only
--                fires while it is NULL, so a replay finds nothing to claim).
ALTER TABLE users ADD COLUMN status TEXT NOT NULL DEFAULT 'active';

CREATE TABLE email_verification_tokens (
    id         TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL,
    user_id    TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at    TEXT,
    created_at TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_email_verification_tokens_hash ON email_verification_tokens (token_hash);
CREATE INDEX idx_email_verification_tokens_user ON email_verification_tokens (tenant_id, user_id);
