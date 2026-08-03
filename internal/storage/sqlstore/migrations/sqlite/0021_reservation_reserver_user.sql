-- Registered-tier reserve (ADR-0012 cut 3). A reservation made by an authenticated
-- account is bound to that account so it can appear on the reserver's own "things
-- I've reserved" dashboard (#20) and gate the `registered` reserver tier.
--
--   reserver_user_id — the account that made the reservation, or NULL for an
--     anonymous (full_guest / email_confirmed) reserve. System-only: it never
--     surfaces on any owner-facing response, so owner anonymity is unchanged
--     (ADR-0002 §5). Existing rows default to NULL (anonymous), so behavior is
--     unchanged for every reservation made before this.
ALTER TABLE reservations ADD COLUMN reserver_user_id TEXT;

CREATE INDEX idx_reservations_reserver ON reservations (tenant_id, reserver_user_id);
