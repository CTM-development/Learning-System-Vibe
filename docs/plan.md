# Learning System — Spec, Critique & Build Plan

## Context

Greenfield personal learning/studying system in `/home/onestone/Projects/Learning-System-Vibe` (empty repo, README only). Requirements from user:

- React frontend, Go backend, accessible on home network
- Local markdown files as first-class content
- Anki-style cards, spaced-repetition scheduling, performance logging for **any** kind of question
- Phase 2: LLMs via OpenRouter, Karpathy-style generated wiki scoped to studying

**Vision (added):** this is a **closed-loop learning environment** for a postgrad/doctoral learner, not a note-taking app plus flashcards. It should support the full cycle **ingest → understand → represent → retrieve → solve → write → diagnose → schedule next work**. The central object is neither the note nor the flashcard but the **learnable concept** connected to evidence, practice, writing, and diagnostic history. Markdown essays are not dead documents — they decompose into concepts, claims, examples, questions, dependencies, and testable units.

Decisions already made with user:
- **Card authoring**: inline card syntax inside md notes **+** standalone card-only md files ("deck files") **+** LLM-proposed cards (phase 2)
- **Card identity**: system writes small ID anchors back into md files (`<!-- srs:xxxx -->`); DB keeps full card snapshots so cards could migrate to DB-only storage later without data loss
- **Single user, no login**
- **Phasing**: SRS core first, LLM features in phase 2
- **V1 scope (new)**: stay lean, but V1 additionally covers three concrete requirements — (1) universal action timing + a manual productivity/learning **session toggle**, (2) **PDF upload** with PDFs acting as citable, searchable sources for md files, (3) first-class support for the user's **study flow** (skim → deep read → fold into essays/cards)
- **PDF depth (new)**: near term = upload + store + cite + text extraction for search; the full structured paper workflow (claims/critique YAML, BibTeX, highlight extraction) is phase 4

---

## Design principles

The system design is anchored in six empirically supported learning mechanisms. Each principle names where this plan implements it.

1. **Retrieval practice over passive review** (testing effect; Roediger & Karpicke 2006, [PubMed](https://pubmed.ncbi.nlm.nih.gov/16507066/)). Testing improves later retention, not just measures it. → Cards are the primary interaction (V1); essays generate cards; "rewrite from memory" tasks (phase 3).
2. **Spaced practice by default** (Cepeda et al. 2006 meta-analysis, [PubMed](https://pubmed.ncbi.nlm.nih.gov/16719566/); Dunlosky et al. 2013 rate practice testing + distributed practice highest-utility, [PubMed](https://pubmed.ncbi.nlm.nih.gov/26173288/)). → FSRS scheduling in V1; spacing later extends beyond cards to problems, essays, and paper summaries (phases 3–4).
3. **Interleaving**, especially for math/CS (Rohrer & Taylor, shuffled mathematics practice, [ResearchGate](https://www.researchgate.net/publication/227181272_The_shuffling_of_mathematics_problems_improves_learning)). The hard part is problem *classification*, not execution. → Problem bank with mixed sets and "choose the method" tasks (phase 3).
4. **Self-explanation and worked examples** (Chi et al. 1989, [Wiley](https://onlinelibrary.wiley.com/doi/abs/10.1207/s15516709cog1302_1)). Stronger learners connect example steps to principles. → Derivation/explain/proof-step card types; worked-example fading (phase 3).
5. **Cognitive load management** (Sweller 1988, [ScienceDirect](https://www.sciencedirect.com/science/article/pii/0364021388900237)). Unconstrained problem solving can crowd out schema acquisition. → Scaffolding progression: full worked solution → missing justifications → partial → hints → independent → transfer (phase 3).
6. **Self-regulated learning** (Zimmerman's forethought → performance → self-reflection cycle, [PDF](https://www.leiderschapsdomeinen.nl/wp-content/uploads/2016/12/Zimmerman-B.-2002-Becoming-Self-Regulated-Learner.pdf)). → Sessions + universal timing (V1), error log with root-cause taxonomy (phase 3), planner and open learner model (phase 4).

**The most important design choice:** the system does not optimize for "having many notes." It optimizes for six questions — Can you retrieve it? Use it in a problem? Explain it precisely? Distinguish it from nearby concepts? Write about it with sources? Transfer it to a new context?

---

## Critique of the constraints (requested feedback)

1. **React + Go — good, but ship as one binary.** Embed the built Vite app into the Go binary via `go:embed`. One process, no CORS, no reverse proxy; `systemctl start learning` and open `http://<lan-ip>:8844` from any device.
2. **"Home network access" hides a security decision.** No login is fine on a trusted LAN, but never port-forward this box — in phase 2 the server holds your OpenRouter key. If you later want access away from home, use Tailscale rather than exposing the port. This matters more now: the system will hold sensitive personal learning data (performance history, weaknesses, error patterns). Follow data-minimization and purpose-limitation principles (GDPR core principles, [EC overview](https://commission.europa.eu/law/law-topic/data-protection/data-protection-explained_en)); local-first, exportable, version-controlled storage is the default philosophy.
3. **Md-as-source-of-truth's hard problem is card identity, not parsing.** Editing a card's wording must not reset its scheduling history. ID anchors solve this (decided). Corollary rule: the system only ever *appends anchors* — it never rewrites your prose.
4. **"Anki-style scheduling" — use FSRS, not SM-2.** Modern Anki itself moved to FSRS; it gives measurably better retention-per-review. The `open-spaced-repetition/go-fsrs` library implements it. Keep the review log **append-only** so FSRS parameters can later be optimized against your own history.
5. **"Performance logging for any kind of question" is a schema decision, not a feature.** Design one generic append-only event table now. This generalizes further (see Timing & sessions below): card reviews, free-text answers, note edits, PDF reading, and problem attempts are all event kinds in one `activity_events` table. Retrofitting this later is painful; doing it now is cheap.
6. **The LLM wiki's best trick: persist generated pages as md files** in your notes tree (`wiki/` subfolder). Generated articles then become ordinary notes — searchable, linkable, and *card-generatable*. That closes the loop: ask → article → cards → scheduled review. This is the flywheel that makes the Karpathy wiki a study tool instead of a chat toy.
7. **Missing from your constraints, worth adding**: math rendering (KaTeX — essential for studying), images in cards, full-text search (SQLite FTS5, free), backups (everything is files: notes dir + one SQLite file + attachments), mobile-friendly UI (you'll review from your phone on the couch), keyboard-driven review flow.

---

## Target system model (long-term shape)

The phases below build toward a six-layer system:

1. **Knowledge base** — markdown essays, notes, papers, definitions, theorems, proofs, code snippets, diagrams.
2. **Practice layer** — cards, problem sets, proof tasks, coding tasks, concept questions, essay prompts.
3. **Learning model** — tracks mastery, confidence, forgetting, misconceptions, transfer ability.
4. **Scheduler** — chooses what to review, solve, rewrite, or test next.
5. **AI tutor layer** — grounded explanation, Socratic questioning, code/proof feedback, card generation, essay critique.
6. **Research/writing layer** — claims, sources, literature notes, BibTeX, drafts, synthesis essays.

Essays decompose into **atomic knowledge objects**: concept, definition, theorem, lemma, proof, algorithm, example, counterexample, misconception, claim, source note, problem type, card, essay prompt, code exercise. Graduate learning is relational — how definitions, examples, procedures, intuitions, and proofs depend on each other (concept mapping has empirical support, [Springer](https://link.springer.com/article/10.1007/s10648-024-09877-y)). A single sentence like "Bayes' rule states posterior ∝ likelihood × prior" links to a definition card, derivation card, example problem, the misconception "posterior = likelihood", the Bayesian-networks essay, and its source.

An ideal essay page follows this template (V1 supports it by convention; later phases parse it):

```md
# Variational Inference
## One-sentence intuition
## Formal definition
## Derivation
## Worked example
## Common misconceptions
## Connections
## Test cards generated from this essay
## Open questions
```

Richer frontmatter (V1 parses `stage`, `status`, `tags`, `sources`; the rest is forward-compatible metadata):

```yaml
title: Bayesian Neural Networks
domain: machine-learning
stage: synthesis        # skim | deep | synthesis
status: draft
prerequisites: [Bayesian inference, Neural networks, Variational inference]
sources: [bishop2006prml, blundell2015weight_uncertainty]
learning_objectives:
  - explain posterior over weights
  - derive predictive distribution
review_priority: high
```

---

## Study flow support (V1)

The system must support the user's existing three-step flow as a first-class workflow, not fight it:

1. **Skim** — skim material, write high-level notes, capture questions. → Note with `stage: skim`; an `## Open questions` section is parsed into an open-question queue (these are future cards/essay prompts).
2. **Deep read** — summary note taking, QnA. → `stage: deep`. Crucially, **QnA during deep reading uses the Q/A card syntax directly** — deep-read question-answering *is* card authoring; no separate transcription step.
3. **Fold** — consolidate notes into topic essays + quiz cards and other final material. → `stage: synthesis`; essay follows the template above; its cards are the durable retrieval layer.

UI support: Notes view filters by stage ("to deepen", "ready to fold"); stage transitions are one click (edits frontmatter); the open-question queue is visible so skim-stage questions don't get lost.

---

## V1 additions (three concrete requirements)

### Timing & sessions
- **Every action is timed.** The generic event table becomes `activity_events(id, ts, kind, ref, elapsed_ms, session_id?, payload JSON)` — append-only. Event kinds: `card_review` (with rating, schedule before/after), `free_text_answer`, `note_edit`, `essay_write`, `pdf_read`, `sync`, later `problem_solve`. Review latency comes from the review UI; editor/reading time via client focus heartbeats (debounced, coalesced into one event per continuous stint).
- **Manual session toggle.** `sessions(id, kind productivity|learning, started_at, ended_at, note)`; a start/stop toggle with a live timer sits in the UI header. While a session is active, all events get its `session_id`. Stats gain time-per-activity, time-per-topic, and per-session breakdowns.

### PDF sources
- Upload endpoint stores originals under `attachments/pdfs/`; `sources(id, kind pdf|url|book, path, title, meta JSON, added_at)`.
- Server-side text extraction → FTS5 index, so PDF content is searchable alongside notes.
- Notes cite sources via frontmatter `sources:` and inline refs; the note reader lists linked sources and opens the PDF in the browser.
- Deferred to phase 4: structured paper YAML, BibTeX import/export, highlight extraction.

Both land in M1's schema so nothing needs retrofitting (consistent with critique point 5).

---

## Phased roadmap

**V1 (SRS core + the three additions)**: inline Q/A + cloze cards, deck files, decks/tags from folder structure + frontmatter, FSRS scheduling with Again/Hard/Good/Easy, daily queue with new-cards/day limit and interval fuzz, keyboard-first review UI, md note browser with KaTeX + code highlighting, append-only activity log, stats dashboard (review heatmap, due forecast, retention, time-per-card, **time-per-activity/session**), card browser (suspend/bury/orphan handling), FTS5 search (notes **+ PDF text**), file-watcher auto-sync, **sessions + universal timing**, **PDF sources**, **note stages + open-question capture**.

**Phase 2 (LLM)**: OpenRouter client with model picker + daily token budget, LLM card generation with human accept/edit step, Karpathy-style wiki (generate article → save as md → `[[wikilinks]]` to nonexistent pages offer generation; grounded in your own notes via FTS retrieval), free-text answer grading against rubric, tutor chat scoped to a note. **AI-tutor safeguards** (LLM-in-education reviews flag over-reliance, hallucination, and reliability risks, [ScienceDirect](https://www.sciencedirect.com/science/article/pii/S2666920X25001699)): retrieval-grounded against the user's own notes and cited sources; cites its sources; distinguishes "from your notes" vs "external knowledge"; marks uncertainty; prefers hints before answers and asks the learner to predict before explaining; keeps a provenance trail on every AI-generated card. The AI acts as an adaptive examiner, not a content firehose.

**Phase 3 (practice depth)**:
- **Richer card types** beyond basic/cloze: derivation ("derive the normal equations"), proof-step ("why does compactness matter here?"), concept-discrimination ("Bayesian network vs Bayesian neural network"), error-correction (diagnose a wrong proof/code snippet), example-generation ("give a non-convex function with a local minimum"), transfer ("apply marginalization to HMMs"). For math/CS the highest-value cards are procedure-selection, derivation, and counterexample cards — not facts.
- **Confidence rating** at review time (feeds calibration and the learner model).
- **Problem bank**: problems tagged by concept, method, difficulty, prerequisite; **interleaved mixed sets** across related topics (eigenvalues + diagonalization + SVD + quadratic forms + Markov chains + PCA, not 20 eigenvalue drills); "choose the method before solving" tasks; delayed re-solving of failed problems; transfer tasks.
- **Worked-example fading**: full worked solution → missing justifications → missing steps → hints only → independent problem → transfer problem. Especially for measure theory, optimization, probability derivations, Bayesian inference, algorithms, formal proofs.
- **Error log / misconception model**: every failed card/proof/problem prompts a root-cause classification — memory failure, conceptual confusion, algebraic manipulation, wrong problem-type classification, missing prerequisite, overconfidence, careless execution, source misunderstanding — plus a repair action (e.g. "review matrix-calculus essay, solve 5 JVP cards, rewrite derivation from memory in 3 days") that feeds the scheduler. Failure becomes structured learning data.
- **Essay-as-learning mode**: write essay sections from memory, compare against source notes, extract claims into cards, extract open questions, track essay maturity (notes → outline → rough → rigorous → publishable), generate oral-exam questions from an essay.

**Phase 4 (learner model & research workflow)**:
- **Open learner model dashboard**: mastery by concept, forgetting risk, weak prerequisites, overconfidence zones, error patterns, neglected domains. Dashboards must be *actionable*, not just aware (LAD reviews find limited gains otherwise, [arXiv](https://arxiv.org/abs/2312.15042)) — not "you are weak in probability" but "your failed Bayesian-network problems mostly involve marginalization over hidden variables; solve three variable-elimination tasks, then rewrite the marginalization essay from memory."
- **Knowledge tracing**: simple, interpretable first (BKT-style; Corbett & Anderson, [Springer](https://link.springer.com/article/10.1007/BF01099821)); mastery ≈ f(correctness, confidence, latency, recency, difficulty, error type, prerequisite mastery). Interpretability beats predictive accuracy for a personal system.
- **Concept/prerequisite graph** across essays, cards, problems.
- **Paper workflow**: per-paper structured YAML (research question, method, dataset, main claims, evidence, limitations, relation to other work, my critique, cards generated, follow-up questions); BibTeX import/export; highlight extraction; comparison matrix; "what would falsify this claim?" / "explain without jargon" / "explain formally" prompts; literature-gap tracker.
- **Planner** at three levels — daily (today's reviews, one hard problem block, one writing block, one repair task from the error log), weekly (domain balance across math/CS/neuroscience, weak-prerequisite repair, reading and revision targets), long-term (competency map, milestone tracking, decay prevention for old topics). Implements Zimmerman's forethought → performance → reflection cycle.

**Backlog / research-grade (explicitly out of scope for now)**: Anki `.apkg` import, image occlusion, multi-user, PWA/offline, vector-embedding retrieval, FSRS parameter auto-optimization (data collected from day one), formal proof integration (Lean), CAS integration (SymPy/Sage), executable/unit-tested code exercises, oral-exam simulation, transformer-based knowledge tracing, automated misconception detection, adaptive experimental scheduling, causal evaluation of interventions, automated literature maps, domain packs (math theorem/counterexample templates; CS algorithm-tracing/complexity/debugging/paper-to-code workflows; neuroscience diagram-labeling, mechanism maps, molecular→cellular→systems→behavior concept links).

---

## Architecture

### Repo layout (in `Learning-System-Vibe/`)
```
cmd/server/main.go        # flags/config, serve
internal/config           # yaml + env: notes_dir, attachments_dir, port, db_path
internal/store            # sqlite (modernc.org/sqlite, cgo-free), embedded migrations
internal/mdsync           # md parser, card extraction, ID anchor write-back, fsnotify watcher
internal/srs              # FSRS wrapper (go-fsrs/v3), queue building
internal/sources          # PDF upload, storage, text extraction, FTS indexing
internal/api              # net/http 1.22+ mux, JSON handlers
web/                      # Vite + React 19 + TS; build output embedded via go:embed
testdata/notes/           # fixture notes for parser/sync tests
testdata/pdfs/            # fixture PDFs for extraction tests
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
- Phase 3 card types reuse the same anchor mechanism with a `type:` marker line.

### Sync algorithm (`internal/mdsync`)
Parse file → for each card block: anchor present → upsert by ID (update content snapshot in DB); no anchor → generate ID, insert, append anchor to file. Cards whose anchors vanish from files → mark **orphaned** (soft-delete; history retained; restorable). Also extracted per note: frontmatter (`stage`, `status`, `tags`, `sources`), `## Open questions` items → open-question queue. Triggered by fsnotify (debounced ~1s) and a manual "Sync" button. Never touches non-card text.

### Data model (SQLite)
- `notes(path, title, frontmatter, stage, mtime, content_fts…)` + FTS5 index
- `cards(id, note_path, type basic|cloze|…, front, back, deck, tags, state, suspended, orphaned_at)`
- `card_schedule(card_id, due, stability, difficulty, reps, lapses, last_review)` — current FSRS state
- `activity_events(id, ts, kind, ref, elapsed_ms, session_id?, payload JSON)` — **append-only universal log**; card reviews are one kind (payload: rating, schedule before/after), free-text answers, note edits, PDF reading, syncs are others
- `sessions(id, kind productivity|learning, started_at, ended_at, note)`
- `sources(id, kind pdf|url|book, path, title, meta JSON, added_at)` + PDF text in FTS index
- `open_questions(id, note_path, text, status open|carded|folded|dropped)`
- phase 2: `llm_calls(ts, model, purpose, tokens_in/out, cost)` for budget tracking
- phase 3: `error_log(event_id, root_cause, repair_action, resolved_at)`, `problems`, `problem_attempts` (as event kinds)
- phase 4: `concepts(id, name, definition_ref, prerequisites…)`, paper-workflow tables

### API sketch
`GET /api/queue` · `POST /api/reviews {card_id, rating, elapsed_ms}` · `GET/POST /api/sync` · `GET /api/notes/*path` (rendered + raw) · `GET /api/cards?deck=&q=` · `PATCH /api/cards/{id}` (suspend etc.) · `GET /api/stats/…` · **`POST /api/sessions/start` / `POST /api/sessions/stop` · `GET /api/sessions`** · **`POST /api/sources` (PDF upload) · `GET /api/sources` · `GET /api/sources/{id}` (file)** · **`POST /api/events` (client-reported timed activities)** · `GET /api/questions` (open-question queue) · phase 2: `/api/llm/generate-cards`, `/api/wiki/{slug}`, `/api/grade`.

### Frontend
Vite + React 19 + TypeScript, TanStack Query, react-router, Tailwind. `react-markdown` + `remark-gfm` + `rehype-katex` + code highlighting. Review screen: Space = reveal, 1–4 = rate, fully usable one-handed on a phone. **Header: session toggle with live timer.** Views: **Review**, **Notes** (browser/reader with stage filters + linked sources + open-question queue), **Sources** (PDF list/viewer/upload), **Cards** (table w/ filters), **Stats** (incl. time/session analytics), phase 2: **Wiki**, **Generate**.

### AI/RAG layer (phase 2+)
Local notes retrieval (FTS first, embeddings later) · source-grounded generation · prompt templates · citation tracking · provenance on generated artifacts · hallucination checks.

---

## Milestones

- **M1 — Skeleton**: Go server + SQLite + migrations + config; Vite app embedded; single binary serves on `0.0.0.0:8844`; systemd unit example in README. **Schema includes `activity_events`, `sessions`, `sources` from day one.**
- **M2 — Md ingestion**: parser (Q/A + cloze), sync with anchor write-back, orphan handling, fsnotify watcher; **stage frontmatter + open-question extraction**; Notes browser UI with KaTeX and stage filters. *Parser/sync get real table-driven tests against `testdata/notes/` — this is the riskiest code.*
- **M3 — Review loop**: FSRS integration, queue building (due + new/day + fuzz), review UI with keyboard flow, reviews logged to `activity_events` with latency; **session toggle UI + session attribution of events; focus-heartbeat timing for note editing/reading**. System is daily-usable here.
- **M4 — Stats & management**: dashboard (heatmap, due forecast, retention, time-per-card, **time-per-activity/topic/session**), card browser, suspend/restore, FTS search.
- **M5 — PDF sources**: upload endpoint + `attachments/pdfs/` storage, text extraction → FTS, `sources:` frontmatter linking, source list + in-browser PDF viewer, `pdf_read` timing events.
- **M6 — OpenRouter + card generation** (phase 2 starts): server-side key, model picker, budget guard; select note → proposed cards → accept/edit → written to md with anchors + provenance.
- **M7 — LLM wiki**: generate article → saved to `notes/wiki/*.md` → red-link generation flow; retrieval grounding via FTS (notes + PDF text); safeguards from the phase-2 list.
- **M8 — Free-text grading + tutor chat**: rubric-graded open questions logged to `activity_events`; per-note tutor with hint-before-answer behavior.

Phases 3 (practice depth) and 4 (learner model & research workflow) remain roadmap sections; they get numbered milestones when phase 2 ships.

---

## Priority mapping (ideal-system spec → this plan)

| Spec tier | Items | Lands in |
|---|---|---|
| Must-have | md essays, LaTeX, cards, spaced repetition, tags, source/citation tracking, search, version control (Git), export | V1 (export: md/files are native; Anki/PDF/BibTeX export → phase 4) |
| Must-have | error log | lightweight via `activity_events` in V1; root-cause taxonomy in phase 3 |
| Strongly recommended | AI card generation, essay critique, oral-exam questions | phase 2 |
| Strongly recommended | problem bank, interleaving, worked-example fading, confidence calibration | phase 3 |
| Strongly recommended | open learner dashboard, knowledge graph, paper workflow | phase 4 |
| Advanced | knowledge tracing, RAG tutor, misconception detection, transfer prompts, learning analytics | phases 2–4 |
| Advanced/research-grade | Lean/CAS, code execution, adaptive experiments, causal evaluation, literature maps, curriculum generation | backlog |

---

## Verification

- `go test ./...` — table-driven tests for md parser, sync/anchor write-back (incl. edit/rename/delete scenarios), FSRS state transitions, PDF text extraction against `testdata/pdfs/`.
- End-to-end per milestone: run the binary against `testdata/notes` copy → sync → review a card → confirm anchor written to file, schedule advanced, event row logged.
- **Sessions/timing**: start session → review a card, edit a note → stop session → confirm one session row and events carrying its `session_id` with plausible `elapsed_ms`; stats view reflects the session.
- **PDF flow**: upload a PDF → its text matches in FTS search → add it to a note's `sources:` → citation resolves and PDF opens from the note view.
- **Study flow**: create a `stage: skim` note with `## Open questions` → questions appear in queue → transition to `deep`, add Q/A blocks → cards created on sync → transition to `synthesis`.
- LAN check: open from phone on home wifi; review flow usable on mobile.
- Phase 2: generate cards from a fixture note with a cheap OpenRouter model; verify budget accounting and provenance recorded.
