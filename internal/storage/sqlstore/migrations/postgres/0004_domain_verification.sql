-- Custom-domain verification (#12 / ADR-0004). Mirrors the SQLite migration. A
-- stable per-domain DNS TXT-token proof-of-control challenge — not a secret
-- capability (stored plaintext, exposed to the owner). Empty until addDomain
-- mints one.

ALTER TABLE domains ADD COLUMN verification_token TEXT NOT NULL DEFAULT '';
