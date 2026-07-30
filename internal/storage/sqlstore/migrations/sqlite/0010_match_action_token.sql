-- Purpose-scoped, expiring match-action token for cross-device co-buy confirm/
-- decline (#96). Hash + expiry live on the contribution; empty hash never matches
-- (the default for contributions not currently in a proposed match).
ALTER TABLE contributions ADD COLUMN match_action_token_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE contributions ADD COLUMN match_action_token_expires_at TEXT;
