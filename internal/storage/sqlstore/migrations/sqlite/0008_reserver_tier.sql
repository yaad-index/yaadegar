-- Reserver-identity tier (#81, ADR-0007 v1). The reservation state machine now
-- spans more than decay (it gains an email_confirmed pending_confirmation gate),
-- so decay_state/decay_state_at are renamed to state/state_at — the values are
-- unchanged, only the column names. confirm_token_hash holds the hash of the
-- one-time email-confirmation token (empty = none / full_guest); email_confirmed_at
-- stamps when a pending reservation was confirmed (NULL = never — the verified-email
-- signal, un-backfillable). lists.reserver_tier is the per-list tier override
-- (NULL = inherit the instance default).

ALTER TABLE reservations RENAME COLUMN decay_state TO state;
ALTER TABLE reservations RENAME COLUMN decay_state_at TO state_at;

ALTER TABLE reservations ADD COLUMN confirm_token_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE reservations ADD COLUMN email_confirmed_at TEXT;

ALTER TABLE lists ADD COLUMN reserver_tier TEXT;
