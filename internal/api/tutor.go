package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/CTM-development/learning-system-vibe/internal/llm"
	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// handleGradeAnswer grades a typed answer against the card's reference
// answer (the back). The result is advisory: the learner still presses the
// FSRS rating themselves. The attempt is logged as a free_text_answer
// event — raw material for the phase-3 error log.
func (s *Server) handleGradeAnswer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CardID    string `json:"card_id"`
		Answer    string `json:"answer"`
		Model     string `json:"model"`
		ElapsedMs int64  `json:"elapsed_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.CardID == "" || strings.TrimSpace(req.Answer) == "" {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {card_id, answer, model?, elapsed_ms?}"))
		return
	}
	if req.Model == "" {
		req.Model = s.Config.LLMModel
	}
	if err := s.checkBudget(); err != nil {
		writeError(w, http.StatusTooManyRequests, err)
		return
	}

	card, err := s.Store.GetCard(req.CardID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	content, usage, err := s.LLM.Chat(r.Context(), req.Model,
		llm.GradePrompt(card.Front, card.Back, req.Answer), 1000)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := s.Store.LogLLMCall(req.Model, "grade_answer",
		usage.PromptTokens, usage.CompletionTokens, usage.Cost,
		map[string]string{"card_id": req.CardID}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	grade, err := llm.ParseGrade(content)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	// The attempt itself is learning data: answer + judgment, timed and
	// session-attributed like every other activity. The event id comes back
	// to the client so a failure can be classified into the error log.
	eventID, err := s.Store.LogEvent("free_text_answer", req.CardID, req.ElapsedMs,
		s.Store.ActiveSessionID(), map[string]any{
			"answer":           req.Answer,
			"verdict":          grade.Verdict,
			"suggested_rating": grade.SuggestedRating,
			"model":            req.Model,
		})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"grade":    grade,
		"model":    req.Model,
		"usage":    usage,
		"event_id": eventID,
	})
}

// maxTutorNoteRunes bounds how much note content enters the tutor prompt.
const maxTutorNoteRunes = 8000

// handleTutor is one stateless tutor-chat exchange scoped to a note: the
// client sends the whole transcript, the server grounds the system prompt
// in the note and returns the next reply.
func (s *Server) handleTutor(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NotePath string        `json:"note_path"`
		Messages []llm.Message `json:"messages"`
		Model    string        `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.NotePath == "" || len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {note_path, messages: [{role, content}], model?}"))
		return
	}
	for _, m := range req.Messages {
		if m.Role != "user" && m.Role != "assistant" {
			writeError(w, http.StatusBadRequest, errors.New("message roles must be user or assistant"))
			return
		}
	}
	if req.Model == "" {
		req.Model = s.Config.LLMModel
	}
	if err := s.checkBudget(); err != nil {
		writeError(w, http.StatusTooManyRequests, err)
		return
	}

	note, err := s.Store.GetNote(req.NotePath)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	content := truncateRunes(stripFrontmatter(note.Content), maxTutorNoteRunes)
	reply, usage, err := s.LLM.Chat(r.Context(), req.Model,
		llm.TutorPrompt(note.Title, note.Path, content, req.Messages), 2000)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := s.Store.LogLLMCall(req.Model, "tutor",
		usage.PromptTokens, usage.CompletionTokens, usage.Cost,
		map[string]string{"note_path": req.NotePath}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := s.Store.LogEvent("tutor_chat", req.NotePath, 0,
		s.Store.ActiveSessionID(), map[string]any{
			"model": req.Model,
			"turns": len(req.Messages),
		}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"reply": reply,
		"model": req.Model,
		"usage": usage,
	})
}
