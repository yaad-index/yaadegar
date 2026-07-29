-- Per-list override for the email_confirmed confirm-window (ADR-0007), mirroring
-- decay_days: integer minutes, NOT NULL with the -1 sentinel meaning "inherit the
-- instance default (--reserver-confirm-window)".
ALTER TABLE lists ADD COLUMN reserver_confirm_window INTEGER NOT NULL DEFAULT -1;
