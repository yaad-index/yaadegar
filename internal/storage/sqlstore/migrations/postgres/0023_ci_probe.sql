-- TEMPORARY PROBE — do not merge. Verifies that the Postgres CI job added in #335
-- can go red for the reason it was added (#330 acceptance).
--
-- BLOB is a native SQLite storage class and is not a type Postgres has at all, so
-- this file is accepted by one dialect and rejected outright by the other. That is
-- the realistic drift rather than an invented one: the two dialect directories are
-- written by copy-paste, and 0022's bodies are already byte-identical.
ALTER TABLE lists ADD COLUMN ci_probe BLOB;
