# Learning System — Spec, Critique & Build Plan

## Context

Greenfield personal learning/studying system in `/home/onestone/Projects/Learning-System-Vibe` (empty repo, README only). Requirements from user:

- React frontend, Go backend, accessible on home network
- Local markdown files as first-class content
- Anki-style cards, spaced-repetition scheduling, performance logging for **any** kind of question
- Phase 2: LLMs via OpenRouter, Karpathy-style generated wiki scoped to studying

Decisions already made with user:
- **Card authoring**: inline card syntax inside md notes **+** standalone card-only md files ("deck files") **+** LLM-proposed cards (phase 2)
- **Card identity**: system writes small ID anchors back into md files (`<!-- srs:xxxx -->`); DB keeps full card snapshots so cards could migrate to DB-only storage later without data loss
- **Single user, no login**
- **Phasing**: SRS core first, LLM features in phase 2

---

## Critique of the constraints (requested feedback)

1. **React + Go — good, but ship as one binary.** Embed the built Vite app into the Go binary via `go:embed`. One process, no CORS, no reverse proxy; `systemctl start learning` and open `http://<lan-ip>:8844` from any device. 
2. **"Home network access" hides a security decision.** No login is fine on a trusted LAN, but never port-forward this box — in phase 2 the server holds your OpenRouter key. If you later want access away from home, use Tailscale rather than exposing the port.
3. **Md-as-source-of-truth's hard problem is card identity, not parsing.** Editing a card's wording must not reset its scheduling history. ID anchors solve this (decided). Corollary rule: the system only ever *appends anchors* — it never rewrites your prose.
4. **"Anki-style scheduling" — use FSRS, not SM-2.** Modern Anki itself moved to FSRS; it gives measurably better retention-per-review. The `open-spaced-repetition/go-fsrs` library implements it. Keep the review log **append-only** so FSRS parameters can later be optimized against your own history.
5. **"Performance logging for any kind of question" is a schema decision, not a feature.** Design one generic `question_attempts` event table now (question type, prompt ref, response, grade, latency, timestamp). Card reviews are one event type; phase-2 free-text/LLM-graded answers are another. Retrofitting this later is painful; doing it now is cheap.
6. **The LLM wiki's best trick: persist generated pages as md files** in your notes tree (`wiki/` subfolder). Generated articles then become ordinary notes — searchable, linkable, and *card-generatable*. That closes the loop: ask → article → cards → scheduled review. This is the flywheel that makes the Karpathy wiki a study tool instead of a chat toy.
7. **Missing from your constraints, worth adding**: math rendering (KaTeX — essential for studying), images in cards, full-text search (SQLite FTS5, free), backups (everything is files: notes dir + one SQLite file), mobile-friendly UI (you'll review from your phone on the couch), keyboard-driven review flow.

## Features an ideal system has (scoped into this plan)

**V1 (SRS core)**: inline Q/A + cloze cards, deck files, decks/tags from folder structure + frontmatter, FSRS scheduling with Again/Hard/Good/Easy, daily queue with new-cards/day limit and interval fuzz, keyboard-first review UI, md note browser with KaTeX + code highlighting, append-only review log, stats dashboard (review heatmap, due forecast, retention, time-per-card), card browser (suspend/bury/orphan handling), FTS5 search, file-watcher auto-sync.

**Phase 2 (LLM)**: OpenRouter client with model picker + daily token budget, LLM card generation with human accept/edit step, Karpathy-style wiki (generate article → save as md → `[[wikilinks]]` to nonexistent pages offer generation; grounded in your own notes via FTS retrieval), free-text answer grading against rubric, tutor chat scoped to a note.

**Backlog (explicitly out of scope for now)**: Anki `.apkg` import, image occlusion cards, multi-user, PWA/offline, vector embeddings retrieval, FSRS parameter auto-optimization (the data for it is being collected from day one).

---

## Architecture

### Repo layout (in `Learning-System-Vibe/`)
```
cmd/server/main.go        # flags/config, serve
internal/config           # yaml + env: notes_dir, port, db_path
internal/store            # sqlite (modernc.org/sqlite, cgo-free), embedded migrations
internal/mdsync           # md parser, card extraction, ID anchor write-back, fsnotify watcher
internal/srs              # FSRS wrapper (go-fsrs/v3), queue building
internal/api              # net/http 1.22+ mux, JSON handlers
web/                      # Vite + React 19 + TS; build output embedded via go:embed
testdata/notes/           # fixture notes for parser/sync tests
```

### Card syntax in md
- **Basic card** (in any note or deck file):
  ```
  Q: What does FSRS optimize?
  A: Retention per review effort.        <!-- srs:a1b2c3 -->
  ```
  `A:` runs until a blank line; multiline supported.
- **Cloze**: any paragraph containing `{{c1::hidden text}}` (Anki syntax) becomes cloze card(s); anchor appended to the paragraph.
- **Deck files**: an md file that is just a list of cards; identical syntax, no special casing needed.
- Deck = relative folder path; extra tags via YAML frontmatter `tags:`.

### Sync algorithm (`internal/mdsync`)
Parse file → for each card block: anchor present → upsert by ID (update content snapshot in DB); no anchor → generate ID, insert, append anchor to file. Cards whose anchors vanish from files → mark **orphaned** (soft-delete; history retained; restorable). Triggered by fsnotify (debounced ~1s) and a manual "Sync" button. Never touches non-card text.

### Data model (SQLite)
- `notes(path, title, frontmatter, mtime, content_fts…)` + FTS5 index
- `cards(id, note_path, type basic|cloze, front, back, deck, tags, state, suspended, orphaned_at)`
- `card_schedule(card_id, due, stability, difficulty, reps, lapses, last_review)` — current FSRS state
- `question_attempts(id, ts, kind card_review|free_text|…, card_id?, rating?, response?, grade?, elapsed_ms, schedule_before/after JSON)` — **append-only**, the universal performance log
- phase 2: `llm_calls(ts, model, purpose, tokens_in/out, cost)` for budget tracking

### API sketch
`GET /api/queue` · `POST /api/reviews {card_id, rating, elapsed_ms}` · `GET/POST /api/sync` · `GET /api/notes/*path` (rendered content + raw) · `GET /api/cards?deck=&q=` · `PATCH /api/cards/{id}` (suspend etc.) · `GET /api/stats/…` · phase 2: `/api/llm/generate-cards`, `/api/wiki/{slug}`, `/api/grade`.

### Frontend
Vite + React 19 + TypeScript, TanStack Query, react-router, Tailwind. `react-markdown` + `remark-gfm` + `rehype-katex` + code highlighting. Review screen: Space = reveal, 1–4 = rate, fully usable one-handed on a phone. Views: **Review**, **Notes** (browser/reader), **Cards** (table w/ filters), **Stats**, phase 2: **Wiki**, **Generate**.

---

## Milestones

- **M1 — Skeleton**: Go server + SQLite + migrations + config; Vite app embedded; single binary serves on `0.0.0.0:8844`; systemd unit example in README.
- **M2 — Md ingestion**: parser (Q/A + cloze), sync with anchor write-back, orphan handling, fsnotify watcher; Notes browser UI with KaTeX. *Parser/sync get real table-driven tests against `testdata/notes/` — this is the riskiest code.*
- **M3 — Review loop**: FSRS integration, queue building (due + new/day + fuzz), review UI with keyboard flow, `question_attempts` logging. System is daily-usable here.
- **M4 — Stats & management**: dashboard (heatmap, due forecast, retention, time), card browser, suspend/restore, FTS search.
- **M5 — OpenRouter + card generation** (phase 2 starts): server-side key, model picker, budget guard; select note → proposed cards → accept/edit → written to md with anchors.
- **M6 — LLM wiki**: generate article → saved to `notes/wiki/*.md` → red-link generation flow; retrieval grounding via FTS.
- **M7 — Free-text grading + tutor chat**: rubric-graded open questions logged to `question_attempts`; per-note tutor.

## Verification

- `go test ./...` — table-driven tests for md parser, sync/anchor write-back (incl. edit/rename/delete scenarios), FSRS state transitions.
- End-to-end per milestone: run the binary against `testdata/notes` copy → sync → review a card → confirm anchor written to file, schedule advanced, attempt row logged.
- LAN check: open from phone on home wifi; review flow usable on mobile.
- Phase 2: generate cards from a fixture note with a cheap OpenRouter model; verify budget accounting.
