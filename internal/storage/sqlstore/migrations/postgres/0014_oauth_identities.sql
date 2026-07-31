-- Owner login via Google OAuth/OIDC (#21, ADR-0008). Link-only account model: a
-- verified Google identity attaches to an EXISTING owner within one tenant.
--
--   oauth_identities              — one row per (tenant, provider, subject) link.
--   tenants.oauth_google_enabled  — per-tenant on/off toggle (default off). The
--                                   method stays inert unless the instance also
--                                   has a configured Google client (env config,
--                                   fail-closed startup).
--
-- UNIQUE(tenant_id, provider, subject) is deliberately tenant-scoped, NOT global:
-- one Google account may legitimately own in two tenants on a single instance
-- (ADR-0008 §5). The takeover protection is the verified-email + tenant-scope
-- guards, not global subject uniqueness.
CREATE TABLE oauth_identities (
    id         TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL REFERENCES tenants(id),
    user_id    TEXT NOT NULL REFERENCES users(id),
    provider   TEXT NOT NULL,
    subject    TEXT NOT NULL,
    email      TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (tenant_id, provider, subject)
);

-- Look up an existing link for an owner (tenant, user, provider): the callback
-- rejects a second Google account linking to an already-linked owner.
CREATE INDEX idx_oauth_identities_user
    ON oauth_identities (tenant_id, user_id, provider);

ALTER TABLE tenants ADD COLUMN oauth_google_enabled INTEGER NOT NULL DEFAULT 0;
