-- Personal access tokens (ADR-0016). An account holder creates a named, long-lived
-- token for a non-browser client; only its sha256 hash is stored (the raw value is
-- shown once at creation, ADR-0003 §3), so a leaked database yields no usable
-- tokens.
--   token_hash   — sha256 of the raw token; the lookup key. Globally unique.
--   last4        — the raw token's last four characters, for display only.
--   expires_at   — NULL for a token created without expiry (an explicit choice);
--                  otherwise a token past it never validates (checked in Go).
--   last_used_at — NULL until first use; written at most once per few minutes.
--   revoked_at   — NULL until revoked, individually or by an account recovery.
CREATE TABLE access_tokens (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id),
    user_id      TEXT NOT NULL REFERENCES users(id),
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL,
    last4        TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    expires_at   TEXT,
    last_used_at TEXT,
    revoked_at   TEXT
);

CREATE UNIQUE INDEX idx_access_tokens_hash ON access_tokens (token_hash);
CREATE INDEX idx_access_tokens_user ON access_tokens (tenant_id, user_id);
