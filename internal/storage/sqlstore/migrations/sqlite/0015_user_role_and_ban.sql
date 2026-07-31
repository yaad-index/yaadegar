-- Identity/roles model (ADR-0009 Cut 1). Two additive, zero-loss columns on users:
--   role   — the per-tenant role: 'owner' (list owner, today's only kind) or
--            'giver' (a first-class reserver account). Defaults to 'owner' so every
--            existing user stays an owner and nothing changes behaviour.
--   banned — an instance-admin ban flag; a banned account cannot log in or hold a
--            session. Defaults off.
ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'owner';
ALTER TABLE users ADD COLUMN banned INTEGER NOT NULL DEFAULT 0;
