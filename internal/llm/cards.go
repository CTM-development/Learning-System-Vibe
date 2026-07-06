package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ProposedCard is one LLM-suggested card, pending human accept/edit.
type ProposedCard struct {
	Front string `json:"front"`
	Back  string `json:"back"`
}

// CardPrompt builds the grounded card-generation conversation. The rules
// implement the plan's AI-tutor safeguards: strictly note-grounded, no
// outside facts, concept-level questions over trivia.
func CardPrompt(title, content string, count int) []Message {
	system := `You create spaced-repetition flashcards STRICTLY from the note the user provides.

Rules:
- Use ONLY facts, definitions and derivations present in the note. Never introduce outside knowledge, even if you know more about the topic.
- Prefer questions that test understanding: "why", "how", "what distinguishes X from Y", "when does X fail" — over surface trivia.
- Front must be answerable without seeing the note; back is a complete, self-contained answer.
- Keep LaTeX math ($...$) intact when quoting formulas.
- Do not duplicate existing cards: the note may contain "Q:"/"A:" blocks and {{c1::cloze}} paragraphs; skip anything they already cover.
- Respond with ONLY a JSON array, no prose: [{"front": "...", "back": "..."}, ...]`

	user := fmt.Sprintf("Create up to %d cards from this note.\n\n# %s\n\n%s", count, title, content)
	return []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}

// ParseCards extracts the JSON card array from an LLM response, tolerating
// code fences and surrounding prose.
func ParseCards(response string) ([]ProposedCard, error) {
	s := strings.TrimSpace(response)
	// Cut to the outermost JSON array.
	start := strings.IndexByte(s, '[')
	end := strings.LastIndexByte(s, ']')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("no JSON array in LLM response: %s", truncate([]byte(s), 200))
	}

	var cards []ProposedCard
	if err := json.Unmarshal([]byte(s[start:end+1]), &cards); err != nil {
		return nil, fmt.Errorf("parse card JSON: %w", err)
	}

	out := cards[:0]
	for _, c := range cards {
		c.Front = strings.TrimSpace(c.Front)
		c.Back = strings.TrimSpace(c.Back)
		if c.Front != "" && c.Back != "" {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("LLM returned no usable cards")
	}
	return out, nil
}
