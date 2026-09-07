-- Owner page (#308): one shareable link listing all of an owner's listed lists.
-- Two parts.
--
-- 1. users.owner_key — the opaque, unguessable handle for an owner's public index,
--    the owner-level counterpart to lists.share_slug. Same generator and entropy
--    (ADR-0002 §9). It defaults to '' meaning "no key minted yet": neither dialect
--    generates per-row randomness portably, and this schema keeps to portable types,
--    so a key is minted in Go the first time an owner asks for their link rather
--    than backfilled here. The unique index is therefore partial — '' is the
--    not-yet-minted sentinel and many rows carry it, so only real keys are
--    constrained. Rotation overwrites the column in place, which is what makes a
--    circulated link revocable.
--
-- 2. lists.visibility normalisation — the owner page is the FIRST consumer of this
--    column. It has existed since the initial schema with the semantic "governs
--    discovery, not the share link", but nothing has ever branched on its value, so
--    any row already holding 'public' got that value without it meaning anything: it
--    was never a decision to publish. Shipping the owner page would turn those rows
--    into listed lists that no owner opted into, so they are normalised to the same
--    default Create has always applied to a new list. Publishing stays an explicit
--    act. This is empirically a no-op on both known instances (every list on each is
--    already 'private'); it exists so that stops being something anyone has to check.
ALTER TABLE users ADD COLUMN owner_key TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_users_owner_key ON users (tenant_id, owner_key)
    WHERE owner_key <> '';

UPDATE lists SET visibility = 'private' WHERE visibility = 'public';
