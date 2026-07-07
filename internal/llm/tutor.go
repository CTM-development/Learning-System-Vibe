package llm

import "fmt"

// maxTutorHistory bounds how many prior turns are replayed to the model
// (the client sends the full transcript; we keep the tail).
const maxTutorHistory = 16

// TutorPrompt builds the note-scoped tutor conversation. Safeguards from
// the phase-2 plan: grounded in the learner's own note, hints before
// answers, prediction before explanation, explicit note-vs-external
// attribution with uncertainty marked.
func TutorPrompt(title, path, content string, history []Message) []Message {
	system := fmt.Sprintf(`You are a study tutor for a postgraduate learner, scoped to ONE of their notes (included below). You act as an adaptive examiner, not a content firehose.

Rules:
- Ground yourself in the note. Attribute clearly: "Your note says …" for note content; prefix anything beyond it with "Beyond your note:" and mark shaky claims "(uncertain)". Never invent things the note supposedly says.
- Hints before answers: when the learner asks something the note covers, first give a hint or a probing counter-question that helps them retrieve it themselves. Give the full answer only after they have attempted one, or if they explicitly insist.
- Before explaining a mechanism or derivation, ask the learner to predict the next step or the outcome.
- Point out when the learner's statements contradict the note.
- Be concise — a few sentences per turn. Use LaTeX ($...$) for math.

The note (%s — %s):

%s`, title, path, content)

	if len(history) > maxTutorHistory {
		history = history[len(history)-maxTutorHistory:]
	}
	msgs := make([]Message, 0, len(history)+1)
	msgs = append(msgs, Message{Role: "system", Content: system})
	msgs = append(msgs, history...)
	return msgs
}
