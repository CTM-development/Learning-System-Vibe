# Code Review - 2026-07-08

Scope: whole project review against `docs/plan.md`, `docs/paper-notes-plan.md`, maintainability, bugs, performance, security, and unnecessary complexity. This is a critical review; absence from this document does not mean a module is perfect.

## Executive summary

The project is much further along than the baseline plan suggests: V1, Phase 2, M9, and much of M10 exist in code. The strongest parts are the overall architecture, the markdown-as-source-of-truth model, the content-hash sync optimization, tests around parser/sync/store behavior, and the simple concrete API layering.

The main concerns are not broad architectural failure. They are correctness and integrity issues around boundaries:

- File mutation APIs do not consistently confine user-provided note paths.
- Core review scheduling and event logging are not atomic even though the plan makes `activity_events` the authoritative history.
- Timing/session analytics are implemented, but the frontend timing flush strategy can lose session attribution.
- Some M10 AI-safety promises are enforced by prompt/defusing, but not by a hard human-review gate.
- Several upload/generation flows can leave partial state or exceed their documented budget limits.

Recommended priority: fix the path-confinement and review-transaction issues first, then fix timing/session attribution, then tighten M10 and LLM budget semantics.

## Findings

### High - Note mutation APIs allow path traversal outside `notes_dir`

Evidence:

- `internal/mdsync/stage.go:32` joins `s.NotesDir` and caller-provided `rel` directly.
- `internal/mdsync/append.go:32` does the same for accepted generated cards.
- `internal/api/api.go:129-142` exposes `SetStage` through `/api/notes/stage`.
- `internal/api/llm.go:129-158` exposes `AppendQABlocks` through `/api/llm/accept-cards`.

Impact: a LAN caller can send paths containing `../` and cause the server to read/rewrite markdown-like files outside the notes tree, limited only by the server process permissions. This violates the local-first trust boundary and is more serious because stage changes and accepted-card appends intentionally write to disk.

Suggested fix: add one shared resolver in `mdsync` or `api` that rejects absolute paths, cleans slash paths, verifies the resolved absolute path remains under `NotesDir`, requires `.md`, and preferably verifies that the note exists in the store before mutation. Use it for `SetStage`, `AppendQABlocks`, any future note write helper, and API reads where path semantics matter.

### High - Review UI advances cards before persistence succeeds

Evidence:

- `web/src/views/Review.tsx:98-123` increments position/progress and hides the card before `/api/reviews` returns.
- `web/src/views/Review.tsx:154-160` advances before bury succeeds.

Impact: a network or server failure can skip a card in the UI while the schedule was not updated. That is dangerous in the core review loop because the user receives false feedback: the session appears to have progressed, but the database did not. It also makes retry behavior unclear.

Suggested fix: use explicit review/bury mutations, disable controls while pending, and only advance the queue after success. On failure, leave the current card visible and show a retryable error.

### High - Read timing is not flushed in a way that preserves session attribution

Evidence:

- `web/src/useReadTimer.ts:22-31` sends the entire read event only during React effect cleanup.
- `web/src/App.tsx:122-124` stops sessions independently; it does not flush active read timers before stopping.

Impact: if the user reads a note/PDF during a session, stops the session from the header, and only later navigates away, the eventual `note_read`/`pdf_read` event is logged after the session ended and loses its `session_id`. Hidden tabs, browser kills, or long-lived views can also drop or misattribute large chunks of time. This directly weakens the plan's "every action is timed" and session analytics promise.

Suggested fix: report debounced chunks periodically while visible, flush on `visibilitychange` and `pagehide`, and flush active timers before stopping a session. Alternatively, include a client-known active session id in timed events and have the server validate it.

### Medium - Review schedule update and event log insertion are not atomic

Evidence:

- `internal/api/review.go:91-104` updates `card_schedule` and then appends the `card_review` event.
- `internal/api/review.go:131-136` restores schedule and then logs `review_undo`.

Impact: if event insertion fails after the schedule write, FSRS state advances without append-only history. That breaks undo, new-card limit accounting, retention stats, session attribution, and the plan's core data-model contract: review history lives in `activity_events`.

Suggested fix: move review application into a store-layer transaction that updates schedule and inserts the event together. Do the same for undo. Return the event id from the transactional method.

### Medium - LLM daily token budget is only a pre-call threshold, not an enforceable budget

Evidence:

- `internal/api/llm.go:55-64` only rejects calls when already at or above budget.
- High-token calls then run normally, for example transcription at `internal/api/transcribe.go:87`.

Impact: any accepted call can push usage far beyond `llm_daily_tokens`, especially vision transcription or wiki generation. Concurrent LLM calls can overshoot further. This diverges from the README and plan language that the budget "hard-stops" further calls.

Suggested fix: estimate prompt tokens plus requested `max_tokens`, reject or cap calls that cannot fit in the remaining budget, and reserve budget atomically before concurrent calls. At minimum, rename/document the current behavior as a soft post-hoc limit.

### Medium - `http.Server` has no connection timeouts

Evidence:

- `cmd/server/main.go:106-117` constructs an `http.Server` with `Addr` and `Handler` only.

Impact: slow clients on the LAN can hold sockets/goroutines open. The app is LAN-only, but the server also stores sensitive learning data and an OpenRouter key, so basic HTTP hardening is still warranted.

Suggested fix: set `ReadHeaderTimeout`, `IdleTimeout`, and a considered `ReadTimeout`. Keep upload routes protected with existing `MaxBytesReader` limits rather than relying on a very short global body timeout.

### Medium - Multipart scan uploads can exceed the documented scan limits at the HTTP layer

Evidence:

- `internal/api/sources.go:40` caps every multipart upload at `sources.MaxUploadBytes + 1 MiB`, which is about 129 MiB.
- `internal/sources/sources.go:26-31` permits up to 100 scan pages at 32 MiB each, implying a much larger logical cap.

Impact: the M10 phone-camera scan feature may fail for legitimate bundles well below the per-page and page-count limits. The error will present as a generic multipart parse problem before `SaveScan` can produce the more specific validation errors.

Suggested fix: use a separate request cap for scan uploads, or lower `MaxScanPages`/`MaxScanPageBytes` to match the actual HTTP limit. If large bundles are not intended, make the constants honest.

### Medium - AI scan drafts can be saved without an explicit human-review gate

Evidence:

- `docs/paper-notes-plan.md` says nothing AI-written is saved without human review.
- `web/src/views/Workbench.tsx:74-80` inserts AI draft text and records `transcribedBy`.
- `web/src/views/Workbench.tsx:279-284` allows saving as soon as the title is present and save is not pending.

Impact: the UI warns the user, but the workflow does not enforce the M10 safeguard. A user can persist an unchecked transcription, including hallucinated or misread content, as an ordinary searchable note.

Suggested fix: after AI drafting, require a checkbox or explicit confirmation such as "reviewed against original" before enabling save. Reset that confirmation whenever a new draft replaces the body.

### Medium - Source upload can leave partial state after indexing failure

Evidence:

- `internal/sources/sources.go:72-87` writes the PDF, inserts the source row, then indexes text.
- `internal/sources/sources.go:156-164` does the same for scans.

Impact: if FTS indexing fails, the API returns an error even though a source row and files may remain. A retry can create duplicate sources, and the user sees "upload failed" while the system contains a partially usable source. This weakens the PDF/search and scan-source flows from the plan.

Suggested fix: wrap source row creation and indexing in a DB transaction and clean up files on any post-write failure. Another acceptable design is to return success with explicit `indexed: false` and surface extraction/indexing status.

### Medium - Note paths are not URL-encoded in frontend routes/API calls

Evidence:

- `web/src/views/Notes.tsx:151-153` interpolates note paths directly into routes.
- `web/src/views/NoteReader.tsx:32-35` interpolates the route path directly into `/api/notes/${path}`.
- Similar direct interpolation appears in backlinks and wikilink output in `web/src/views/NoteReader.tsx:112` and `195`.

Impact: markdown filenames containing `#`, `?`, `%`, spaces, or other reserved URL characters can become unreachable or fetch the wrong note. Since local markdown files are first-class content, the app should be robust to common filesystem names.

Suggested fix: add shared helpers that encode each path segment with `encodeURIComponent` while preserving `/`. Use them for note routes and API URLs; decode only at the server/router boundary.

### Medium - PDF/source timing can be recorded for the wrong kind

Evidence:

- `web/src/views/SourceViewer.tsx:17-20` starts `useReadTimer` before source data is loaded, defaulting to `pdf_read` unless kind is known to be `scan`.
- The same hook runs for `url` and `book` sources even though those are reference cards, not in-app PDF/scan reading.

Impact: stats can show `pdf_read` time for URL/book references, and slow scan loads can initially accrue as PDF time. This pollutes the time analytics that the plan treats as central.

Suggested fix: start timing only after `source.data` is available, and only for `pdf` and `scan`. Pass no timer for `url`/`book`, or use a distinct `source_ref_view` kind if that activity matters.

### Medium - Plan requires per-session analytics, but `/api/stats/time` omits them

Evidence:

- `docs/plan.md` calls for time-per-activity/topic/session and per-session breakdowns.
- `internal/api/stats.go:50-65` returns only `by_kind` and `by_deck`.
- `internal/store/stats.go:152-171` implements kind/deck aggregations only.

Impact: the manual productivity/learning session toggle cannot answer the key question it introduces: what happened in each session. The Stats view can list recent sessions, but it cannot break down time or activity by session.

Suggested fix: add `TimeBySession` grouped by `session_id`, session kind, start/end, and optionally top activity kinds. Return it from `/api/stats/time` and render it in Stats.

### Medium - Sync/write sequences are not transactionally grouped

Evidence:

- `internal/mdsync/sync.go:169-204` writes anchors to the markdown file, then upserts note/card/link/open-question rows through separate store calls.
- `internal/store/queries.go:36-65`, `86-124`, and `197-240` each run their own statements.

Impact: the mutex prevents concurrent note-file writers, but a failure after anchor write-back can leave files and DB out of sync until a later sync repairs it. A failure midway through DB updates can also leave a note updated but links/questions/cards stale. This is recoverable, but it makes the highest-risk path less robust than the docs imply.

Suggested fix: after any file write succeeds, group all DB reconciliation for that file in a transaction. For full `SyncAll`, consider one transaction for the DB phase after parsing/writes, or per-file transactions plus a final orphan/link-resolution transaction.

### Medium - `CreateNote` can overwrite an existing filename under concurrent requests

Evidence:

- `internal/api/notes_create.go:155-171` chooses the first non-existing path.
- `internal/api/notes_create.go:128-136` later writes it with `os.WriteFile`, which truncates if another request created the same path in the meantime.

Impact: two concurrent creates with the same title can race and one can overwrite the other's note. This is realistic from double-submit or two browser tabs.

Suggested fix: create files with `O_CREATE|O_EXCL` and retry suffix selection on `EEXIST`. Also disable duplicate form submission client-side after first submit; the server fix is still required.

### Low - `SetStage` edits YAML frontmatter with a regex

Evidence:

- `internal/mdsync/stage.go:14` matches `stage:` anywhere in the frontmatter text.
- `internal/mdsync/stage.go:40-49` inserts/replaces a line without parsing YAML.

Impact: this is intentionally scoped, but it can still corrupt or mis-edit valid YAML edge cases: quoted multi-line values, comments that contain `stage:`, or nested keys named `stage`. It is acceptable for narrow personal use but brittle for "markdown files as first-class content."

Suggested fix: parse frontmatter with `yaml.v3`, update the top-level `stage`, and preserve the body. If comment preservation matters, operate on the YAML node tree rather than a map.

### Low - Relative markdown links are treated as assets instead of note navigation

Evidence:

- `web/src/Markdown.tsx:25-32` rewrites all relative URLs through `/api/notes-assets/...`.

Impact: `[other note](other.md)` opens raw markdown as an asset instead of navigating to the note reader. This weakens ordinary markdown compatibility and forces users toward wikilinks.

Suggested fix: if a relative URL ends in `.md`, resolve it against the current note directory and route to `/notes/<encoded-path>`. Keep images and other assets on `/api/notes-assets`.

### Low - M10 migration rebuilds `sources` but does not recreate an index on `key`

Evidence:

- `internal/store/migrations/0005_scans_thoughts.sql:10-22` rebuilds `sources`.
- The unique constraint on `key` remains, so correctness is preserved.

Impact: this is not currently a correctness bug because `key` remains unique, but table rebuild migrations are easy places to drop indexes or triggers silently. As the source table grows, repeated `SourceKeyExists` calls can become slower if future schema changes accidentally remove the implicit unique index.

Suggested fix: add migration tests that inspect expected indexes/constraints after every rebuild. Keep a schema snapshot test for `sources`.

### Low - Search/card browsing uses simple `LIKE`, not FTS

Evidence:

- `internal/store/cards_browse.go:41-44` filters card front/back with `%LIKE%`.

Impact: fine for small decks, but card browsing can become slow as generated cards and cloze cards grow. Notes and sources already use FTS, so this is an inconsistency rather than an immediate defect.

Suggested fix: add a `cards_fts` table if card search starts to lag, or explicitly document that card browsing is capped/simple for now.

### Low - Frontend bundle is large and not code-split

Evidence:

- `npm run build` succeeds but Vite reports `dist/assets/index-*.js` at about 970 kB minified, 290 kB gzip, with a chunk-size warning.

Impact: for a LAN app this is acceptable, but mobile review is a first-class use case in the plan. Initial load on older phones will be slower than necessary.

Suggested fix: lazily load heavy views (`Wiki`, `Generate`, `Workbench`, `Stats`, `Errors`) with `React.lazy`, leaving Today/Review/Notes in the initial bundle.

## Positive notes

- The content-hash sync design in `internal/mdsync/sync.go` matches the onboarding guidance and avoids mtime-related dropped edits.
- The parser is pure and tested, which is appropriate because card identity and anchor write-back are high-risk.
- The store layer is simple and readable. For this project size, concrete dependencies and real SQLite tests are pragmatic.
- Search snippets are escaped server-side before `dangerouslySetInnerHTML`, which keeps the frontend rendering choice reasonable.
- The M10 scan implementation stores original scan pages as citable sources and keeps generated transcription as ordinary markdown, which matches the plan's architecture.

## Verification performed

- `npm run build` in `web/` passed.
- Vite emitted a chunk-size warning for the main JS bundle.
- `go test ./...` could not be run because `go` is not installed on PATH in this environment.

## Suggested fix order

1. Add shared note-path confinement and apply it to all note-file mutation helpers.
2. Make review/undo schedule changes and event logging transactional.
3. Fix frontend review/bury optimistic progression.
4. Rework read timers so session attribution is reliable.
5. Enforce M10 human-review confirmation for AI transcriptions.
6. Tighten LLM budget semantics or rename it as a soft limit.
7. Add per-session stats and clean up source upload partial-state behavior.
