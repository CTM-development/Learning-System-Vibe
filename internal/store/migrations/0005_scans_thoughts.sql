-- M10: paper-note scans as sources + the Thoughts note type.

-- notes.type: 'reading' (default) or 'thought', parsed from frontmatter.
ALTER TABLE notes ADD COLUMN type TEXT NOT NULL DEFAULT 'reading';

-- sources.kind gains 'scan' (one row per capture bundle; ordered page
-- images under attachments/scans/<key>/, filenames listed in meta).
-- SQLite cannot alter a CHECK constraint, so rebuild the table; nothing
-- references sources by foreign key.
CREATE TABLE sources_new (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    kind     TEXT NOT NULL CHECK (kind IN ('pdf', 'url', 'book', 'scan')),
    key      TEXT NOT NULL UNIQUE,
    path     TEXT NOT NULL DEFAULT '',
    title    TEXT NOT NULL DEFAULT '',
    meta     TEXT NOT NULL DEFAULT '{}',
    added_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
INSERT INTO sources_new (id, kind, key, path, title, meta, added_at)
    SELECT id, kind, key, path, title, meta, added_at FROM sources;
DROP TABLE sources;
ALTER TABLE sources_new RENAME TO sources;
