-- Forgot-password reset tokens (ADR-0011 cut 3). A reset-request mints a single-use,
-- short-TTL token; only its sha256 hash is stored (the raw token is emailed once,
-- ADR-0003 §3), so a leaked database yields no usable tokens. Confirm claims the
-- token atomically (used_at set only while NULL) and routes the new password through
-- the shared funnel, bumping credential_version.
--   token_hash — sha256 of the raw token; the lookup key. Globally unique.
--   expires_at — hard expiry; a token past it never validates (checked in Go).
--   used_at    — NULL until claimed; the single-use guard (the claim UPDATE only
--                fires while it is NULL, so a replay finds nothing to claim).
CREATE TABLE password_reset_tokens (
    id         TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL,
    user_id    TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at    TEXT,
    created_at TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_password_reset_tokens_hash ON password_reset_tokens (token_hash);
CREATE INDEX idx_password_reset_tokens_user ON password_reset_tokens (tenant_id, user_id);
