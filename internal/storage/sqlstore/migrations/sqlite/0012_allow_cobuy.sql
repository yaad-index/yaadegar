-- Owner opt-out of group-buying (#100). Two additive levels, both defaulting to
-- co-buying enabled so migrated data keeps its current behaviour:
--   lists.allow_cobuy  — the list-level default (NOT NULL, default enabled).
--   items.allow_cobuy  — a per-item override; NULL means "inherit the list".
ALTER TABLE lists ADD COLUMN allow_cobuy INTEGER NOT NULL DEFAULT 1;
ALTER TABLE items ADD COLUMN allow_cobuy INTEGER;
