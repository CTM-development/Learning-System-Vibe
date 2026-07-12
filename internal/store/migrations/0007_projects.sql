-- Projects: named groups of note directories ("decks"), optionally with a
-- deadline (local date) that later milestones use to squeeze review
-- intervals and pace new-card introduction.
CREATE TABLE projects (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    deadline   TEXT,  -- local date YYYY-MM-DD; NULL = no deadline
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE project_dirs (
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    dir        TEXT NOT NULL,  -- deck prefix relative to notes dir; '' = root
    PRIMARY KEY (project_id, dir)
);

CREATE INDEX idx_project_dirs_dir ON project_dirs(dir);
