-- Incremental sync: notes.content_hash is the SHA-256 of the file bytes as
-- of the last sync. SyncAll skips a note whose file hashes to the stored
-- value, avoiding a reparse + FTS reindex of the whole tree on every run.
-- Existing rows default to '' so the first sync after upgrade re-indexes
-- them once (no hash matches '') and fills the column in.
ALTER TABLE notes ADD COLUMN content_hash TEXT NOT NULL DEFAULT '';
