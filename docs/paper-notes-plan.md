# Paper notes & Thoughts — proposal (M10)

Goal: paper is the primary capture medium for first/second-pass reading;
those notes must enter the system as ordinary staged markdown notes, and a
new **Thoughts** note type covers free-standing ideas. Digitization is an
entry ramp into the existing skim → deep → synthesis flow, not a parallel
system.

## Principles

- The original page always survives as evidence (scan stored + cited),
  exactly like PDFs: local files, one `sources` row, viewable from notes.
- Nothing AI-written is saved without human review (same safeguard as card
  generation and the wiki).
- Digitized notes are ordinary notes: stage frontmatter, cards, open
  questions, wikilinks, FTS — nothing special downstream.

## Stages (each independently shippable)

### M10a — Scans as sources + the Thoughts type
- Source kind `scan`: one row per capture bundle, ordered image pages in
  `attachments/scans/<key>/page-NN.jpg`, upload via multi-file input with
  `capture="environment"` so a phone on the LAN can shoot pages directly.
- Reader integration: notes citing a scan show it in the source chips; a
  scan viewer pages through the images.
- Thoughts: frontmatter `type: thought` (default `reading`) parsed into a
  column; `thoughts/` folder as conventional home; Notes view filter;
  thoughts are full citizens (cards, links, questions).

### M10b — Transcription workbench (no LLM required)
- Side-by-side view: scan pages left, markdown editor right.
- Save = create note file (`title`, `stage` picker or `type: thought`,
  `sources: [scan-key]`, tags) + immediate sync; cards/questions in the
  transcript become live on save.
- Doubles as the born-digital Thought create form (title + body →
  `thoughts/YYYY-MM-DD-slug.md`).
- Editing time logged as `note_edit` events (the timing gap V1 documented).

### M10c — LLM vision drafting
- "Draft with AI" in the workbench: pages → OpenRouter vision model →
  transcript pre-fills the editor; the human corrects against the original
  before saving.
- Client extension: multimodal message content (image parts, base64),
  client-side downscale to ~1600px before upload to bound vision tokens.
- Prompt safeguards: faithful transcription only; LaTeX for math; mark
  `[illegible: …]`; `[diagram: description]` placeholders for sketches;
  keep the note's original language; never invent content; never emit
  Q:/A: or cloze syntax (card creation stays a human decision — use the
  existing Generate flow after saving).
- Provenance frontmatter (`transcribed_by`, `transcribed_at`, scan key),
  `llm_calls` purpose `transcribe`, daily budget applies.

## Explicitly out of scope
- OCR search inside scan images (transcripts are the searchable text).
- General WYSIWYG note editing beyond the workbench.
- Automatic card extraction from transcripts without review.

## Decisions (2026-07-07)
- **Full pipeline A→B→C** confirmed, built as three shippable stages
  (M10a scans+thoughts type → M10b workbench → M10c AI drafting).
- **Thoughts**: `thoughts/` folder + parsed `type: thought` + Notes filter
  + in-app create form reusing the workbench editor.
