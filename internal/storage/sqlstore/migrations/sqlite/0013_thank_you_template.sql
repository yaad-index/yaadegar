-- Owner→giver thank-you note (#22). Two additive levels, both defaulting to "no
-- note" so migrated data keeps its current behaviour:
--   lists.thank_you_template  — the list-level default body (NOT NULL, '' = off).
--   items.thank_you_template  — a per-item override; NULL means "inherit the list".
ALTER TABLE lists ADD COLUMN thank_you_template TEXT NOT NULL DEFAULT '';
ALTER TABLE items ADD COLUMN thank_you_template TEXT;
