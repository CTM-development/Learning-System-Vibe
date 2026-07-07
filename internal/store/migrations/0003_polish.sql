-- V1 polish: bury support and the note link graph (wikilinks/backlinks).

ALTER TABLE card_schedule ADD COLUMN buried_until TEXT;

-- [[wikilinks]] parsed out of notes. to_path is the resolved note path,
-- NULL while the target has no matching note ("red link").
CREATE TABLE note_links (
    from_path TEXT NOT NULL,
    target    TEXT NOT NULL,
    to_path   TEXT,
    PRIMARY KEY (from_path, target)
);
CREATE INDEX idx_note_links_to ON note_links(to_path);
