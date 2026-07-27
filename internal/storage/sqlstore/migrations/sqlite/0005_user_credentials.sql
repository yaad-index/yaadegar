-- Owner password credentials for the real auth mechanism (ADR-0005 / #30 Cut A1).
-- `username` is the password-login handle, unique per tenant when set; `password_hash`
-- is the argon2id PHC-encoded hash. Both optional: `username` is NULL and
-- `password_hash` is '' for a user with no password credential (e.g. a future
-- OAuth/magic-link-only account), so the partial unique index below still allows
-- many credential-less users per tenant.

ALTER TABLE users ADD COLUMN username TEXT;
ALTER TABLE users ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_users_tenant_username
    ON users (tenant_id, username) WHERE username IS NOT NULL;
