package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// handlePatchQuestion updates an open question's lifecycle status
// (open → carded / folded / dropped).
func (s *Server) handlePatchQuestion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid question id"))
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Status == "" {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {status}"))
		return
	}
	if err := s.Store.SetQuestionStatus(id, req.Status); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCapture appends a quick-captured question to the inbox note and
// syncs, so it lands in the open-question queue immediately.
func (s *Server) handleCapture(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {text}"))
		return
	}
	if err := s.Syncer.AppendOpenQuestion(req.Text); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := s.Syncer.SyncAll(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.Store.LogEvent("capture", "", 0, s.Store.ActiveSessionID(),
		map[string]any{"text": req.Text}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// handleNoteAsset serves non-markdown files (images, mainly) from the
// notes directory so relative references inside notes and cards resolve.
// The path is confined to the notes dir.
func (s *Server) handleNoteAsset(w http.ResponseWriter, r *http.Request) {
	rel := filepath.Clean(filepath.FromSlash(r.PathValue("path")))
	notesAbs, err := filepath.Abs(s.Syncer.NotesDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	abs := filepath.Join(notesAbs, rel)
	if abs != notesAbs && !strings.HasPrefix(abs, notesAbs+string(filepath.Separator)) {
		writeError(w, http.StatusBadRequest, errors.New("path escapes notes directory"))
		return
	}
	http.ServeFile(w, r, abs)
}

// handleToday is the start-of-day dashboard: what is due, what is stuck,
// what needs repair.
func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	summary, err := s.Store.Summary(s.Config.NewPerDay, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// "New available" should reflect cards that actually exist, not just
	// the configured daily allowance.
	if newCards, err := s.Store.NewCards(time.Now(), summary.NewRemaining, ""); err == nil {
		summary.NewRemaining = len(newCards)
	}
	stale, err := s.Store.StaleNotes(14, 8)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	questions, err := s.Store.ListOpenQuestions("open")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// ListOpenQuestions sorts newest first; the tail is the oldest.
	oldest := questions
	if len(oldest) > 3 {
		oldest = oldest[len(oldest)-3:]
	}
	leeches, err := s.Store.CountLeeches()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"summary":          summary,
		"stale_notes":      stale,
		"open_questions":   len(questions),
		"oldest_questions": oldest,
		"leeches":          leeches,
	})
}
