---
name: verify
description: Build, launch and drive this app end-to-end to verify changes at its real surface (HTTP API + embedded SPA).
---

# Verifying Learning-System-Vibe

Single Go binary serving both the JSON API and the embedded React SPA. Notes
are markdown files in a directory; SQLite is derived state.

## Build

```bash
export PATH="$HOME/.local/go/bin:$PATH"     # Go 1.26 lives here, not on PATH
cd web && npm run build && cd ..            # SPA embeds via go:embed web/dist
go build -o /tmp/.../learnserver ./cmd/server
```

Frontend-only changes still require `npm run build` before `go build`, or the
binary serves the stale embedded bundle.

## Launch (isolated)

```bash
LEARN_PORT=8199 \
LEARN_NOTES_DIR=$TMP/notes \
LEARN_DB_PATH=$TMP/learn.db \
LEARN_ATTACHMENTS_DIR=$TMP/att \
LEARN_BACKUPS_DIR= \
LEARN_NEW_PER_DAY=1 \
  ./learnserver > $TMP/server.log 2>&1 &
```

- Server runs an initial sync of `LEARN_NOTES_DIR` on boot and logs
  `{Notes:N CardsCreated:N ...}` — check the log to confirm seeding worked.
- `LEARN_NEW_PER_DAY=1` makes new-card limit behavior observable.
- Seed notes: plain markdown; `Q: ...\nA: ...` lines become cards;
  `[[wikilinks]]` resolve by stem/title; the note's folder path is its deck.
- Gotcha: `VAR=x cmd &` on one line backgrounds the whole chain including
  shell variable assignments on the same line — set helper vars separately.
- Gotcha: `pgrep -f learnserver` matches the bash process running pgrep
  itself; use `pgrep -f "scratchpad/[l]earnserver"` or probe the port.

## Drive

Useful flows (all curl-able, no auth):
- `POST /api/sync` then `GET /api/notes`, `GET /api/notes/{path}` (links/backlinks)
- `POST /api/projects {"name","dirs":["dir1"],"deadline":"YYYY-MM-DD"}`;
  `GET /api/queue?project=ID` (pacing extras: days_left/target_new_today);
  `?deck=` and `?project=` are mutually exclusive (400)
- `POST /api/reviews {"card_id","rating":1-4}` — deadline cap observable in
  the returned `schedule.due` and in the latest `activity_events` payload
  (`sqlite3 $TMP/learn.db "SELECT payload FROM activity_events ORDER BY id DESC LIMIT 1"`)
- `POST /api/reviews/undo` restores the pre-review snapshot
- SPA: `GET /notes` returns index.html (SPA fallback); hashed chunks under `/assets/`

No browser/Playwright available on this machine — pixel-level UI behavior
(mermaid render, hover cards, tree interaction) can only be verified by the
user in a real browser; verify the data contracts these components consume
instead.
