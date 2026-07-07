package llm

import (
	"encoding/base64"
	"fmt"
)

// ScanImage is one scan page handed to the vision model.
type ScanImage struct {
	MIME string // image/jpeg | image/png | image/webp
	Data []byte
}

// TranscribePrompt builds the vision conversation that digitizes scanned
// paper notes. Safeguards per the M10 plan: faithful transcription only —
// no invention, no translation, no flashcard syntax (card creation stays
// behind the human-review flow).
func TranscribePrompt(pages []ScanImage) []Message {
	system := `You transcribe scanned handwritten or printed study notes into clean markdown, faithfully.

Rules:
- Transcribe exactly what is written. NEVER invent, complete, or "improve" content.
- Keep the note's original language; do not translate.
- Use LaTeX for mathematics: $...$ inline, $$...$$ for display equations.
- Mark unreadable spots as [illegible: best guess], or [illegible] when you have no guess.
- Replace sketches and figures with [diagram: short description].
- Preserve the visible structure: headings, lists, emphasis, tables where clearly intended.
- Do NOT emit "Q:"/"A:" lines or {{c1::cloze}} markers anywhere — flashcard creation is a separate human step.
- Output the markdown transcript only: no preamble, no commentary, no code fence around the whole transcript.`

	parts := []Part{{
		Type: "text",
		Text: fmt.Sprintf("Transcribe these %d scanned note page(s), in order, as one continuous document.", len(pages)),
	}}
	for _, p := range pages {
		parts = append(parts, Part{Type: "image_url", ImageURL: &ImageRef{
			URL: "data:" + p.MIME + ";base64," + base64.StdEncoding.EncodeToString(p.Data),
		}})
	}
	return []Message{
		{Role: "system", Content: system},
		{Role: "user", Parts: parts},
	}
}
