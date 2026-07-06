-- V1 schema. Everything the roadmap needs on day one so nothing is retrofitted:
-- notes, cards, card_schedule, append-only activity_events, sessions, sources,
-- open_questions, plus FTS5 indexes for notes and extracted PDF text.

CREATE TABLE notes (
    path        TEXT PRIMARY KEY,          -- relative to notes_dir
    title       TEXT NOT NULL DEFAULT '',
    frontmatter TEXT NOT NULL DEFAULT '{}',-- full frontmatter as JSON
    stage       TEXT,                      -- skim | deep | synthesis
    status      TEXT,
    tags        TEXT NOT NULL DEFAULT '[]',-- JSON array of strings
    sources     TEXT NOT NULL DEFAULT '[]',-- JSON array of source keys
    mtime       INTEGER NOT NULL DEFAULT 0,
    content     TEXT NOT NULL DEFAULT ''
);

CREATE VIRTUAL TABLE notes_fts USING fts5(
    path UNINDEXED,
    title,
    content,
    tokenize = 'porter unicode61'
);

CREATE TABLE cards (
    id          TEXT PRIMARY KEY,          -- the srs:xxxx anchor id
    note_path   TEXT NOT NULL,
    type        TEXT NOT NULL DEFAULT 'basic', -- basic | cloze | (phase 3 types)
    front       TEXT NOT NULL,
    back        TEXT NOT NULL DEFAULT '',
    deck        TEXT NOT NULL DEFAULT '',  -- relative folder path of the note
    tags        TEXT NOT NULL DEFAULT '[]',
    suspended   INTEGER NOT NULL DEFAULT 0,
    orphaned_at TEXT,                      -- soft delete; history retained
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX idx_cards_note_path ON cards(note_path);
CREATE INDEX idx_cards_deck ON cards(deck);

-- Current FSRS state per card; the authoritative history lives in activity_events.
CREATE TABLE card_schedule (
    card_id        TEXT PRIMARY KEY REFERENCES cards(id),
    due            TEXT NOT NULL,
    stability      REAL NOT NULL DEFAULT 0,
    difficulty     REAL NOT NULL DEFAULT 0,
    elapsed_days   INTEGER NOT NULL DEFAULT 0,
    scheduled_days INTEGER NOT NULL DEFAULT 0,
    reps           INTEGER NOT NULL DEFAULT 0,
    lapses         INTEGER NOT NULL DEFAULT 0,
    state          INTEGER NOT NULL DEFAULT 0, -- FSRS: 0 new, 1 learning, 2 review, 3 relearning
    last_review    TEXT
);
CREATE INDEX idx_card_schedule_due ON card_schedule(due);

CREATE TABLE sessions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    kind       TEXT NOT NULL CHECK (kind IN ('productivity', 'learning')),
    started_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    ended_at   TEXT,
    note       TEXT NOT NULL DEFAULT ''
);

-- Append-only universal activity log. Card reviews, free-text answers, note
-- edits, PDF reading, syncs, problem attempts: all one table, discriminated
-- by kind, details in payload JSON.
CREATE TABLE activity_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ts         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    kind       TEXT NOT NULL,              -- card_review | free_text_answer | note_edit | essay_write | pdf_read | sync | ...
    ref        TEXT,                       -- card id, note path, source id, ...
    elapsed_ms INTEGER,
    session_id INTEGER REFERENCES sessions(id),
    payload    TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_activity_events_ts ON activity_events(ts);
CREATE INDEX idx_activity_events_kind ON activity_events(kind);
CREATE INDEX idx_activity_events_session ON activity_events(session_id);

CREATE TABLE sources (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    kind     TEXT NOT NULL CHECK (kind IN ('pdf', 'url', 'book')),
    key      TEXT NOT NULL UNIQUE,         -- citation key used in note frontmatter
    path     TEXT NOT NULL DEFAULT '',     -- relative to attachments_dir (pdf kind)
    title    TEXT NOT NULL DEFAULT '',
    meta     TEXT NOT NULL DEFAULT '{}',
    added_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE VIRTUAL TABLE sources_fts USING fts5(
    source_id UNINDEXED,
    title,
    content,
    tokenize = 'porter unicode61'
);

CREATE TABLE open_questions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    note_path  TEXT NOT NULL,
    text       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'carded', 'folded', 'dropped')),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX idx_open_questions_note ON open_questions(note_path);
CREATE INDEX idx_open_questions_status ON open_questions(status);
