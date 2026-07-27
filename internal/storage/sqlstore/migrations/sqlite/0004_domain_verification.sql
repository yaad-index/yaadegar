-- Custom-domain verification (#12 / ADR-0004): a stable per-domain DNS TXT-token
-- proof-of-control challenge. Not a secret capability — stored in plaintext and
-- exposed to the owner so they can publish it as a TXT record. Empty until
-- addDomain mints one.

ALTER TABLE domains ADD COLUMN verification_token TEXT NOT NULL DEFAULT '';
