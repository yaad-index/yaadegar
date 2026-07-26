-- Reservation lifecycle fields (#6 wires them; #7 co-buying and #8 decay drive
-- the behavior). Existing rows backfill: last_activity_at = created_at,
-- is_group = 0, decay_state = 'active'.

ALTER TABLE reservations ADD COLUMN last_activity_at TEXT NOT NULL DEFAULT '';
ALTER TABLE reservations ADD COLUMN is_group INTEGER NOT NULL DEFAULT 0;
ALTER TABLE reservations ADD COLUMN decay_state TEXT NOT NULL DEFAULT 'active';

UPDATE reservations SET last_activity_at = created_at WHERE last_activity_at = '';
