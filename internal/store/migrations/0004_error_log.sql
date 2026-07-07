-- M9: the error log. A diagnosis attached to a failure event: root cause
-- (fixed taxonomy) plus an optional repair action that stays open on the
-- Today dashboard until resolved.

CREATE TABLE error_log (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id         INTEGER NOT NULL UNIQUE REFERENCES activity_events(id),
    card_id          TEXT,    -- denormalized from the event's ref
    root_cause       TEXT NOT NULL CHECK (root_cause IN (
        'memory', 'concept', 'manipulation', 'classification',
        'prerequisite', 'overconfidence', 'careless', 'source')),
    note             TEXT NOT NULL DEFAULT '',  -- free-text diagnosis
    repair_action    TEXT NOT NULL DEFAULT '',  -- what to do about it
    repair_note_path TEXT,                      -- note to rework (optional)
    repair_due       TEXT,                      -- YYYY-MM-DD (optional)
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    resolved_at      TEXT
);
CREATE INDEX idx_error_log_resolved ON error_log(resolved_at);
CREATE INDEX idx_error_log_cause ON error_log(root_cause);
CREATE INDEX idx_error_log_card ON error_log(card_id);
