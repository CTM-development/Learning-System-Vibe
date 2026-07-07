package llm

import (
	"fmt"
	"strings"
)

// Excerpt is one retrieval-grounding snippet from the user's own knowledge
// base, fed to the wiki prompt.
type Excerpt struct {
	Origin string // "note: ml/vi.md" or "source: bishop2006"
	Title  string
	Text   string
}

// WikiPrompt builds the wiki-article conversation. It implements the
// phase-2 AI-tutor safeguards: retrieval-grounded against the user's notes
// and cited sources, explicit separation of "from your notes" vs external
// knowledge, marked uncertainty, and no flashcard blocks (cards must go
// through the human accept step).
func WikiPrompt(topic string, excerpts []Excerpt) []Message {
	system := `You write a concise wiki article for a postgraduate learner's personal study wiki. The reader is mathematically mature; be precise, use LaTeX ($...$) for math.

Structure (markdown, no top-level # heading — the system adds the title):
- One-paragraph intuition first.
- "## From your notes" — ONLY what the provided excerpts support. Cite every claim with its origin, e.g. (note: ml/vi.md) or (source: bishop2006). If the excerpts don't cover the topic, write exactly one sentence saying so instead of inventing coverage.
- "## Background" — your own knowledge, clearly separated from the notes. Flag anything you are not certain about with "(uncertain)".
- "## Connections" — related concepts as [[wikilinks]] (e.g. [[KL divergence]]); link generously, missing pages become red links the learner can generate next.
- "## Open questions" — 2-4 questions worth studying next, as a bullet list.

Hard rules:
- Never fabricate citations: only cite origins that appear in the excerpts.
- Do NOT include "Q:"/"A:" flashcard blocks or {{c1::cloze}} markers anywhere — card creation has a separate human-review flow.
- Output the markdown article body only: no frontmatter, no preamble, no code fence around the whole article.`

	var b strings.Builder
	fmt.Fprintf(&b, "Write the article for the topic: %s\n\n", topic)
	if len(excerpts) == 0 {
		b.WriteString("No matching excerpts were found in the learner's notes or sources. Say so in \"## From your notes\".\n")
	} else {
		b.WriteString("Excerpts from the learner's knowledge base:\n")
		for _, e := range excerpts {
			fmt.Fprintf(&b, "\n--- %s (%s) ---\n%s\n", e.Title, e.Origin, e.Text)
		}
	}
	return []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: b.String()},
	}
}
