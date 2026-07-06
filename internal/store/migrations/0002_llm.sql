-- Phase 2: LLM call accounting for the daily token budget and cost trail.

CREATE TABLE llm_calls (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ts         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    model      TEXT NOT NULL,
    purpose    TEXT NOT NULL,              -- generate_cards | wiki | grade | tutor
    tokens_in  INTEGER NOT NULL DEFAULT 0,
    tokens_out INTEGER NOT NULL DEFAULT 0,
    cost       REAL NOT NULL DEFAULT 0,    -- USD as reported by OpenRouter
    meta       TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_llm_calls_ts ON llm_calls(ts);
