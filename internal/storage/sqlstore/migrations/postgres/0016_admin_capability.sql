-- Admin as a per-user capability (ADR-0010). The instance admin stops being a
-- separate identity and becomes a flag on an existing owner account.
--   is_admin — the instance-admin capability: an owner carrying it reaches the
--              non-tenant-scoped /admin surface. Additive, default off, so no
--              existing account gains admin on upgrade — the operator grants it to
--              an owner via the CLI (grant-admin). Orthogonal to role/banned.
ALTER TABLE users ADD COLUMN is_admin INTEGER NOT NULL DEFAULT 0;

-- The separate superadmin identity is retired: its login, session, and env
-- credential are gone, so the backing table has no reader left. No pre-1.0 data to
-- preserve.
DROP TABLE admins;
