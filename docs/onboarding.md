# Developer onboarding — Learning System

This document gets a new developer productive: what the system is, how it's
laid out, why it's built this way, and how the important flows actually work.
For the product vision, the research grounding, and the milestone roadmap, read
[plan.md](plan.md) — this doc is the *how*, that one is the *why* and *what
next*. The paper-notes feature has its own spec in
[paper-notes-plan.md](paper-notes-plan.md).

---

## 1. What this system is

A single-user, self-hosted **learning environment** for a postgraduate learner.
It runs as one Go binary on the home LAN and serves an embedded React app. The
learner writes markdown notes, and the system turns them into spaced-repetition
flashcards (FSRS scheduling), times every activity, indexes everything for
search, ingests PDFs and phone-camera scans as citable sources, and — when an
OpenRouter key is configured — adds LLM features (card generation, a
notes-grounded wiki, answer grading, a tutor).

It is deliberately **not** a note-taking app with flashcards bolted on. The
model is a closed loop: *ingest → understand → represent → retrieve → solve →
write → diagnose → schedule next*. Read plan.md §"Vision" and §"Design
principles" for the learning-science reasoning; you don't need it to work on the
code, but it explains why features exist.

### The one big idea: markdown is the source of truth

Your notes are plain `.md` files in a directory you control (and should keep in
git). The SQLite database is a **derived index** of those files — with one
critical exception: the **review history and schedules** (what you studied,
when, how it went) exist only in the DB, because they can't be reconstructed
from files. That asymmetry drives two rules that everything else follows:

1. **Sync only ever *appends* to your files** — specifically, it appends
   `<!-- srs:xxxx -->` ID anchors to new card blocks. It never rewrites your
   prose. (The one deliberate exception: the "change stage" action edits the
   single `stage:` frontmatter line. That's it.)
2. **The DB is disposable except for history.** Delete `learning.db`, restart,
   re-sync, and you get all your notes and cards back — but you lose scheduling
   and activity history. Hence the daily DB backups (see §9).

If you internalise nothing else, internalise this: **files are authoritative for
content, the DB is authoritative for history, and the two are reconciled by
`SyncAll`.**

---

## 2. Tech stack and the choices behind it

| Layer     | Choice | Why |
|-----------|--------|-----|
| Backend   | Go, `net/http` 1.22+ mux, stdlib-first | One static binary, trivial deploy |
| DB        | SQLite via `modernc.org/sqlite` | **cgo-free** — cross-compiles, no build toolchain pain |
| Scheduler | `open-spaced-repetition/go-fsrs/v3` | FSRS beats SM-2 on retention-per-review |
| Search    | SQLite FTS5 (built in) | Free full-text search over notes + PDF text |
| Frontend  | Vite + React 19 + TS, TanStack Query, react-router, Tailwind v4 | Standard SPA; no server-side rendering |
| Markdown  | `react-markdown` + `remark-gfm`/`remark-math` + `rehype-katex`/`rehype-highlight` | KaTeX math + code highlighting, rendered client-side |
| LLM       | OpenRouter (OpenAI-compatible chat completions) | Model-agnostic; key stays server-side |

Everything ships as **one binary**: the Vite build output is embedded via
`go:embed` (`web/embed.go`), so there is no separate frontend server, no CORS,
and no reverse proxy in production. `systemctl start learning`, open
`http://<lan-ip>:8844`, done.

**Security posture:** no login. It's built for a *trusted LAN only* — never
port-forward it (in phase 2+ the box holds an OpenRouter key and sensitive
learning data). For remote access the README recommends Tailscale.

---

## 3. Repository map

```
cmd/server/main.go     Entry point: load config, open+migrate DB, wire deps,
                       start watcher + backup ticker, serve, graceful shutdown.

internal/config        Config struct + Load(): YAML file → env overrides → defaults.

internal/store         The DB layer. Owns the connection, embedded SQL migrations,
                       and every query. One *store.Store type; methods split
                       across files by concern:
    store.go             connection, pragmas, migration runner
    migrations/*.sql     embedded, versioned, applied in filename order
    queries.go           notes + cards upsert, activity_events log, sync helpers
    review.go            FSRS schedule load/save, queue queries, sessions, undo
    notes_read.go        note reads, list/filter, stale-note detection
    cards_browse.go      card browser (filters, suspend/restore, leeches)
    stats.go             dashboard aggregations (heatmap, forecast, retention, time)
    sources.go           sources rows + FTS indexing of extracted text
    errors.go            error-log: diagnoses, triage, repairs, cause×deck stats
    llm.go               llm_calls logging + daily budget accounting
    links.go             wikilink graph (note_links) + red-link resolution
    backup.go            daily DB snapshot, keep last 7

internal/mdsync        Markdown ⇆ store reconciliation. The riskiest code; has
                       the most tests.
    parse.go             pure parser: frontmatter, Q/A + cloze cards, open
                         questions, wikilinks. No I/O.
    sync.go              SyncAll: walk files, skip unchanged (content hash),
                         write anchors, upsert, orphan vanished cards. Has the
                         concurrency mutex.
    append.go            append Q/A blocks (accepted LLM cards) + capture inbox
    stage.go             SetStage: edit the stage: frontmatter line only
    watch.go             fsnotify watcher, debounced, calls SyncAll

internal/srs           Thin FSRS wrapper: rating → next schedule. Stateless.

internal/sources       PDF/scan/URL/book ingestion: storage under attachments/,
                       PDF text extraction (pdftotext or pure-Go fallback),
                       image sniffing, slugified keys, path confinement.

internal/llm           OpenRouter client + prompt builders + response parsers.
                       One file per feature: cards.go, wiki.go, grade.go,
                       tutor.go, transcribe.go; client.go is the HTTP client.
                       Prompts encode the AI safeguards (grounding, no cards, etc.).

internal/api           HTTP layer. Server struct holds all deps; Handler() wires
                       routes. One file per feature area (review.go, llm.go,
                       wiki.go, tutor.go, sources.go, errors.go, stats.go,
                       notes_create.go, transcribe.go, polish.go, api.go).

web/                   Vite + React frontend.
    embed.go             go:embed of dist/ into the binary
    src/App.tsx          nav + routing + the session toggle in the header
    src/api.ts           typed fetch wrappers + all shared TS types
    src/views/*.tsx      one component per screen (Review, Notes, Stats, …)
    src/components/       shared bits (ScanPager)
    src/Markdown.tsx      the KaTeX + highlight markdown renderer

testdata/              fixture notes and PDFs for parser/extraction tests
docs/                  this file, plan.md, paper-notes-plan.md
```

### Dependency direction (who imports whom)

```
cmd/server ─► api ─► store        (all DB access)
                 ├─► mdsync ─► store
                 ├─► srs
                 ├─► sources ─► store
                 └─► llm
                 └─► config
```

`store` is the leaf everyone depends on; it imports nothing from the project
except `srs` (for the `Schedule` type it stores). `mdsync`, `sources`, and `srs`
never import `api`. The `api.Server` struct holds **concrete** dependencies
(`*store.Store`, `*mdsync.Syncer`, etc.) — there are no interfaces. That's a
deliberate, pragmatic choice for a codebase this size; handlers are tested
against a real SQLite DB and `httptest`, not mocks (see §11).

---

## 4. Data model

Schema lives in `internal/store/migrations/`, applied in order and tracked in
`schema_migrations`. Each migration runs in its own transaction. **Never edit an
applied migration — add a new one.**

| Table | What it holds | Authoritative? |
|-------|---------------|----------------|
| `notes` | one row per `.md` file: title, frontmatter (JSON), stage, status, type, tags, sources, mtime, full content, **content_hash** | Derived from files |
| `notes_fts` | FTS5 index over note title + content | Derived |
| `cards` | one row per card: id (= anchor), note_path, type, front, back, deck, tags, suspended, `orphaned_at` | Derived from files |
| `card_schedule` | current FSRS state per card (due, stability, difficulty, reps, lapses, state, buried_until) | **Authoritative (history)** |
| `activity_events` | **append-only universal log** — every action | **Authoritative (history)** |
| `sessions` | manual productivity/learning sessions | Authoritative |
| `sources` | pdf / url / book / scan; key is the citation key used in frontmatter | Authoritative (files under attachments/) |
| `sources_fts` | FTS5 over source title + extracted text | Derived |
| `open_questions` | `## Open questions` items, lifecycle open→carded→folded→dropped | Partly derived (text from files, status is state) |
| `llm_calls` | token/cost ledger per OpenRouter call (budget) | Authoritative |
| `note_links` | wikilink graph for red-link resolution | Derived |
| `error_log` | diagnosed failures: root cause, repair action, due date, resolved_at | Authoritative |

### The two load-bearing schema ideas

**`activity_events` is one table for everything.** Card reviews,
free-text answers, note-edit stints, PDF reads, syncs, captures, LLM
generations, tutor chats, undo markers — all rows here, discriminated by `kind`,
with details in a JSON `payload`. This was a day-one decision (plan.md critique
#5): retrofitting a universal log later is painful, doing it now is cheap. When
you add a new kind of trackable action, **log an event, don't add a table.**
Because it's append-only, "undo" is itself an event (`review_undo` referencing
the original event id) rather than a delete — see `handleUndoReview`.

**`cards.orphaned_at` is a soft delete.** When a card's anchor disappears from
the files (you deleted the card text), sync marks it orphaned — it vanishes from
queues but its schedule and history survive, and re-adding the anchor restores
it. Cards are never hard-deleted by sync.

---

## 5. The core flow: sync (`internal/mdsync`)

Sync is the heart of the system and where the subtle invariants live. `SyncAll`
runs on startup, on every file change (debounced ~1s by the watcher), and after
any action that writes a note (capture, accept LLM cards, generate wiki, create
note, set stage).

What one `SyncAll` does (`sync.go`):

1. **List** every `.md` file under `notes_dir` (skipping hidden dirs), sorted.
2. **Preload** three things once: existing card base-ids (for collision-free
   anchor generation), every note's stored `content_hash`, and active card ids
   grouped by note.
3. **Per file (`syncFile`):**
   - Read the file, compute its SHA-256.
   - **Incremental skip:** if the hash matches what's stored, the file is
     byte-for-byte unchanged — mark its cards "seen" (so they aren't orphaned)
     and return without parsing. This is the fast path; on a typical sync almost
     every file takes it.
   - Otherwise `Parse` it (pure function, no I/O): frontmatter, Q/A + cloze
     cards, `## Open questions`, `[[wikilinks]]`.
   - **Assign anchors:** for each card block with no `<!-- srs:xxxx -->` anchor,
     generate one (cloze cards sharing a paragraph share an anchor) and append
     it to the file. This is the *only* write to your prose.
   - Upsert the note (refreshing FTS and storing the new content hash), the
     cards (content snapshot only — **scheduling is never touched on update**),
     the wikilink set, and reconcile open questions.
4. **Delete** note rows whose files vanished.
5. **Orphan** active cards not seen this run (anchor removed or file deleted).
6. **Resolve** wikilinks now that every note is present.
7. **Log** one `sync` event.

### Card identity — the hard problem, and how it's solved

Editing a card's wording must **not** reset its scheduling history. The anchor
is the stable identity: `cards.id` *is* the anchor hex (cloze cards are
`anchor#index`). On re-sync, `UpsertCard` updates the content snapshot by id but
the `ON CONFLICT` clause touches nothing when content is unchanged and *never*
touches `card_schedule`. New cards get a schedule row due immediately (state 0 =
new); existing cards keep theirs. Read `store/queries.go:UpsertCard` — the
conditional upsert is doing careful work.

### Two non-obvious mechanisms you must respect

- **Concurrency: `Syncer.mu`.** `SyncAll` and the three file-mutating helpers
  (`AppendQABlocks`, `AppendOpenQuestion`, `SetStage`) are guarded by a mutex.
  The reason: the fsnotify watcher and multiple API handlers can all trigger
  syncs/writes concurrently, and the single SQLite connection serialises
  *statements* but not the read→parse→write-anchor→upsert *sequence* or the
  `os.WriteFile`. Without the lock, two runs could parse the same anchor-less
  card, mint different anchors, and race on the file write — corrupting anchors
  and duplicating cards. **If you add another method that writes into the notes
  tree, take `s.mu`.**
- **Incremental invalidation is content-hash, not mtime.** An earlier version
  used file mtime; it was replaced because mtime is second-granular and two
  edits in the same wall-clock second (which the write-then-sync handler flows
  do routinely) are indistinguishable, silently dropping the second edit.
  Hashing the bytes is also immune to git checkouts and backup restores (which
  reset mtimes but not content). `mtime` is still stored, but only for the
  "stale notes" display — it no longer drives sync. Don't reintroduce mtime as a
  cache key.

---

## 6. The review loop

1. **Queue** (`GET /api/queue`, `store/review.go` + `api/review.go`): due cards
   (state ≠ new, `due <= now`, not buried/suspended/orphaned) oldest-first, plus
   up to `new_per_day` never-seen cards. The new-cards limit counts how many
   *new* cards were introduced today from `activity_events` (excluding undone
   ones). `cram=1&deck=…` ignores due dates and returns the whole deck weakest-
   memory-first, for exam prep.
2. **Rate** (`POST /api/reviews {card_id, rating, elapsed_ms}`): load current
   schedule → `srs.Scheduler.Review` computes the next state → persist it → log
   a `card_review` event whose payload carries the **before and after schedule
   snapshots** and the rating. The event id comes back so the client can later
   file a failure into the error log. Latency (`elapsed_ms`) comes from the
   review UI.
3. **Undo** (`POST /api/reviews/undo`): finds the most recent not-yet-undone
   `card_review`, restores the schedule from the payload's `before` snapshot, and
   logs a `review_undo` event pointing at it. The log stays append-only.
4. **Bury** (`POST /api/cards/{id}/bury`): hide until local tomorrow without
   touching FSRS state.

`srs` itself is stateless and trivial: it converts a stored `Schedule` to an
`fsrs.Card`, calls `Next`, and converts back. Fuzz is enabled so due dates
spread instead of clumping.

---

## 7. Sessions and universal timing

A header toggle starts a `productivity` or `learning` session (`App.tsx`
→ `POST /api/sessions/start`). While one is active, **every** logged event is
stamped with its `session_id` — the store reads the active session id
(`ActiveSessionID()`) at log time. Starting a session ends any currently-active
one. Client-side stints (note reading/editing time from focus heartbeats) are
reported via `POST /api/events`. This is how "time per activity / topic /
session" in Stats is populated. The invariant: **route timed actions through
`LogEvent` so they inherit session attribution for free.**

---

## 8. Sources, LLM features, and the error log

**Sources (`internal/sources`)** — `POST /api/sources` accepts a PDF (magic-byte
checked, size-capped), a multi-file scan bundle (phone camera, stored as ordered
`page-NN.jpg` under `attachments/scans/<key>/`), or a fileless URL/book
reference. PDFs get text-extracted (`pdftotext` if present, else a pure-Go
fallback that recovers from the library's panics) and indexed into `sources_fts`
so their content is searchable alongside notes. Notes cite sources by `key` in
frontmatter `sources:`. All file paths are confined to `attachments_dir`
(`FilePath` rejects traversal).

**LLM (`internal/llm` + `internal/api/llm.go`, `wiki.go`, `tutor.go`)** — off
unless `openrouter_api_key` is set. Every feature follows the same safeguards,
which are worth understanding because they're the product's ethical spine:

- **Grounded**, not free-associating. Card generation uses only the note's
  content; the wiki retrieves the FTS-best notes and source text and must cite
  every claim by origin and separate "from your notes" from external knowledge;
  the tutor is scoped to one note; grading judges only against the card's
  reference answer.
- **Human in the loop.** Generated cards are *proposals* — nothing is written
  until the user accepts, at which point `AppendQABlocks` writes Q/A blocks and
  sync assigns anchors. The wiki writer even *defuses* stray `Q:`/cloze syntax so
  AI text can't sneak past the accept step (`wiki.go:wikiFileContent`). Grading
  only *suggests* a rating; the learner still presses the button.
- **Budgeted.** `checkBudget` blocks calls once the day's tokens hit
  `llm_daily_tokens`; every call is logged to `llm_calls` with tokens and cost.
- **Provenance.** Accepting cards / generating a wiki logs an event linking
  model → note/topic → the created ids, and generated wiki files carry
  `generated_by` / `grounding` frontmatter.

Prompt text and response parsing live entirely in `internal/llm` (one file per
feature); the `api` handlers do orchestration (budget check, load content, call,
log, persist). When you touch prompts, that's the package.

**Error log (`internal/store/errors.go` + `api/errors.go`)** — turns failures
into structured learning data. Reviews rated *Again* and answers graded
*incorrect* surface in a **triage queue**; the user optionally classifies each
into one of eight root causes and attaches a repair (a note to rework + a due
date). Open repairs surface on the Today dashboard until resolved. Every entry
references the `activity_events` row that captured the failure, so the diagnosis
stays joined to the evidence.

---

## 9. Runtime wiring and operations (`cmd/server/main.go`)

On start: load config → `MkdirAll` the notes/attachments/db dirs → open and
migrate the DB → run an initial `SyncAll` → launch the fsnotify **watcher**
goroutine → launch the daily **backup** goroutine (snapshot `learning.db` into
`backups_dir`, keep the last 7 — this protects the one thing not reconstructable
from files) → serve on `0.0.0.0:<port>` → block until SIGINT/SIGTERM, then
graceful shutdown.

Config precedence (`internal/config`): **defaults ← YAML file (`-config`) ←
environment variables**. All keys and their `LEARN_*` env equivalents are in the
README table. The OpenRouter key can come from either but stays server-side.

---

## 10. Invariants you must not break

These are the rules the codebase quietly depends on. Breaking one causes data
loss or corruption, not just a failing test.

1. **Sync only appends anchors; it never rewrites prose.** The sole exception is
   `SetStage` editing the `stage:` line.
2. **Never touch `card_schedule` when a card's content changes.** Editing
   wording must not reset scheduling.
3. **`activity_events` is append-only.** Model corrections/undo as new events.
   Don't `UPDATE` or `DELETE` history.
4. **Cards are soft-deleted (orphaned), never hard-deleted, by sync.**
5. **Take `Syncer.mu` before writing into the notes tree.**
6. **Don't use mtime as a sync cache key** — content hash only (see §5).
7. **Route timed/loggable actions through `LogEvent`** so they get session
   attribution and land in the universal log.
8. **Confine file paths** to their root (`sources.FilePath`, `handleNoteAsset`)
   — these serve user-supplied paths.
9. **Add migrations, never edit applied ones.**
10. **LLM writes go through the human-accept path**, never straight to disk as
    cards.

---

## 11. Building, running, testing

```sh
make build     # builds web/ (Vite), then embeds dist/ into bin/learning-server
make run       # build + run
make test      # go vet ./... && go test ./...
./bin/learning-server -config config.yaml
```

Go lives at `~/.local/go/bin` on this machine and is **not on PATH** by default:

```sh
export PATH=$PATH:$HOME/.local/go/bin
```

**Dev loop:** run `go run ./cmd/server` and `cd web && npm run dev` side by side.
The Vite dev server proxies `/api` to `:8844`, so you get frontend hot-reload
against the live backend without rebuilding the binary.

**Tests.** The backend is well-tested; the parser/sync code (the riskiest) has
table-driven tests against `testdata/`. Handler tests spin up a real SQLite DB
and `httptest` server rather than mocking the store — so a test exercises the
real query path. LLM handler tests point `llm_base_url` at a fake OpenRouter
served by `httptest`. Run the race detector on anything touching sync or shared
state:

```sh
go test -race ./internal/mdsync/ ./internal/store/ ./internal/api/
```

---

## 12. Practical recipes

**Add an API endpoint.** Add the handler to the relevant `internal/api/*.go`
file (or a new one), register it in `api.go:Handler`, and add a typed wrapper +
types in `web/src/api.ts`. Handlers follow a house style: decode/validate the
body, map `store.ErrNotFound` to 404, return `writeError(w, status, err)` on
failure and `writeJSON(w, status, v)` on success.

**Add a DB column or table.** Create the next
`internal/store/migrations/NNNN_name.sql` (numeric prefix, applied in order).
Add the read/write query methods to the appropriate `store/*.go` file. If it's a
`notes`/`cards` column populated from files, wire it through `mdsync` (parse →
`NoteRow`/`CardRow` → upsert). Follow the `0006_note_content_hash` migration as a
template for an additive column with a safe default and self-healing backfill.

**Add a trackable action.** Call `store.LogEvent(kind, ref, elapsedMs,
ActiveSessionID(), payload)` with a new `kind` string. No schema change. Add
stats/aggregation queries against `activity_events` if you want to surface it.

**Add an LLM feature.** Add a prompt builder + parser in `internal/llm`
(one file), an orchestrating handler in `internal/api` that calls `checkBudget`,
`LLM.Chat`, `LogLLMCall`, and (if it produces artifacts) logs a provenance
event. Keep all prompt text in `internal/llm`.

**Add a new card type (phase 3).** The parser currently recognises `basic` and
`cloze` only. The plan intends new types to reuse the anchor mechanism with a
`type:` marker line; there is **not yet an extension seam** for this — you'll
extend `Parse` in `mdsync/parse.go` and the `type` handling in the store. Expect
to touch the parser, not just add a plugin.

---

## 13. Known rough edges (as of this writing)

- **No interfaces / god-object store.** `*store.Store` has a large method
  surface and is passed whole to every handler. Conventional for the size, but
  refactoring friction grows with it. Tests compensate by being integration-style.
- **Full file read on every sync.** The content-hash skip avoids the *expensive*
  work (parse, FTS reindex, upserts) for unchanged files, but every file is
  still read and hashed each run. Fine at hundreds/thousands of notes; revisit
  if the tree gets huge.
- **Card-type extensibility** is not scaffolded yet (see recipe above).

For anything about *what to build next* and why, go back to
[plan.md](plan.md) — the milestone roadmap (M1–M10 are done; phases 3–4 are
mapped) and the priority table are the source of truth for direction.
```

