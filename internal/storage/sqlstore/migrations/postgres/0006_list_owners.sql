-- List ownership moves to a join table (ADR-0005 §7 / #30 Cut A2a): the schema is
-- multi-owner-capable, but v1 enforces exactly one owner per list at the service
-- layer, so collaborative co-ownership (#25) later just relaxes that rule with no
-- further migration. list_owners is the source of truth; the scalar lists.owner_id
-- is backfilled into it and then dropped (no denormalized cache to drift).
--
-- The FK constraints are enforced under Postgres; the repository also cleans up
-- ownership rows explicitly on list delete so behavior matches the SQLite dialect
-- (which this project runs without foreign-key enforcement).

CREATE TABLE list_owners (
    list_id  TEXT NOT NULL REFERENCES lists (id) ON DELETE CASCADE,
    user_id  TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    added_at TEXT NOT NULL,
    PRIMARY KEY (list_id, user_id)
);
CREATE INDEX idx_list_owners_user ON list_owners (user_id);

-- Backfill the existing single owner of each list.
INSERT INTO list_owners (list_id, user_id, added_at)
SELECT id, owner_id, created_at FROM lists;

DROP INDEX idx_lists_tenant_owner;
ALTER TABLE lists DROP COLUMN owner_id;
