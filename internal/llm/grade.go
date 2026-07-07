package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GradeResult is the LLM's judgment of a free-text answer. The suggested
// rating is advice for the learner's own FSRS button press — the system
// never rates a card on the LLM's behalf.
type GradeResult struct {
	Verdict         string `json:"verdict"` // correct | partial | incorrect
	Feedback        string `json:"feedback"`
	Missing         string `json:"missing,omitempty"`
	SuggestedRating int    `json:"suggested_rating"` // 1=Again … 4=Easy
}

// GradePrompt builds the grading conversation. The card's reference answer
// is the sole ground truth: the model judges recall against it, not against
// its own knowledge of the topic.
func GradePrompt(front, back, answer string) []Message {
	system := `You grade a learner's free-text answer to a spaced-repetition card against the card's reference answer.

Rules:
- The reference answer is the SOLE ground truth. Do not grade against outside knowledge; if the learner adds extra facts beyond the reference, ignore them — neither reward nor punish.
- Judge meaning, not wording: paraphrases, equivalent notation, and reordered points count as matches. LaTeX and prose forms of the same math are equivalent.
- verdict: "correct" = all essential content of the reference is present; "partial" = some essential content missing or a minor error; "incorrect" = misses the point or contains a fundamental error.
- feedback: 1-3 direct sentences. Name concretely what was wrong or missing; no filler praise.
- missing: the essential reference points absent from the answer ("" when none).
- suggested_rating maps to the learner's review buttons: 1 = Again (incorrect), 2 = Hard (partial), 3 = Good (correct but imprecise or incomplete edges), 4 = Easy (fully correct and precise). When torn between two, suggest the lower.
- Respond with ONLY a JSON object: {"verdict": "...", "feedback": "...", "missing": "...", "suggested_rating": n}`

	user := fmt.Sprintf("Card question:\n%s\n\nReference answer:\n%s\n\nLearner's answer:\n%s",
		front, back, answer)
	return []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}

// ParseGrade extracts the grade JSON from an LLM response, tolerating code
// fences and prose. Verdict is validated; a missing/absurd rating is
// derived from the verdict (conservatively).
func ParseGrade(response string) (GradeResult, error) {
	s := strings.TrimSpace(response)
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return GradeResult{}, fmt.Errorf("no JSON object in LLM response: %s", truncate([]byte(s), 200))
	}
	var g GradeResult
	if err := json.Unmarshal([]byte(s[start:end+1]), &g); err != nil {
		return GradeResult{}, fmt.Errorf("parse grade JSON: %w", err)
	}

	g.Verdict = strings.ToLower(strings.TrimSpace(g.Verdict))
	switch g.Verdict {
	case "correct", "partial", "incorrect":
	default:
		return GradeResult{}, fmt.Errorf("invalid verdict %q", g.Verdict)
	}
	if g.SuggestedRating < 1 || g.SuggestedRating > 4 {
		switch g.Verdict {
		case "correct":
			g.SuggestedRating = 3
		case "partial":
			g.SuggestedRating = 2
		default:
			g.SuggestedRating = 1
		}
	}
	g.Feedback = strings.TrimSpace(g.Feedback)
	g.Missing = strings.TrimSpace(g.Missing)
	return g, nil
}
