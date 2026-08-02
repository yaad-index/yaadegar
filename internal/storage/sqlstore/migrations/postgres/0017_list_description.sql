-- Owner-editable list description (#143). Additive, defaults to empty so migrated
-- data keeps its current behaviour:
--   lists.description  — the list-level description body (NOT NULL, '' = none).
ALTER TABLE lists ADD COLUMN description TEXT NOT NULL DEFAULT '';
