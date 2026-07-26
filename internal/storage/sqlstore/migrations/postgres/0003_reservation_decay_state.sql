-- Reservation-decay tracking (#8). Mirrors the SQLite migration (ADR-0003 §5).
-- decay_state_at stamps when the current decay_state was entered; it drives the
-- owner->reserver grace window and the expire window. decay_release_token_hash
-- holds the hash of the single-purpose one-click release token minted at
-- reserver_notified (empty = none). Existing rows backfill from last_activity_at.

ALTER TABLE reservations ADD COLUMN decay_state_at TEXT NOT NULL DEFAULT '';
ALTER TABLE reservations ADD COLUMN decay_release_token_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE reservations ADD COLUMN decay_keep_token_hash TEXT NOT NULL DEFAULT '';

UPDATE reservations SET decay_state_at = last_activity_at WHERE decay_state_at = '';

-- Owner email (server-side), kept for the real auth mechanism (#30) and OAuth
-- login (#21). Not used by the decay flow.
ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT '';
