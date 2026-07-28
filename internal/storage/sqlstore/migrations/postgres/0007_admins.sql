-- Instance-level superadmin (ADR-0005 §6 / #30 Cut A2b). Not tenant-scoped: an
-- admin authenticates on the separate /admin surface and is deliberately kept out
-- of the tenant-scoped `users` table. password_hash is an argon2id hash.

CREATE TABLE admins (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL
);
