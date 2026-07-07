package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// handleErrorTriage lists recent undiagnosed failures (Again-rated reviews
// and incorrect free-text answers) awaiting root-cause classification.
func (s *Server) handleErrorTriage(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ErrorTriage(14, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"causes": store.RootCauses,
	})
}

// handleCreateError attaches a diagnosis to a failure event.
func (s *Server) handleCreateError(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventID        int64  `json:"event_id"`
		RootCause      string `json:"root_cause"`
		Note           string `json:"note"`
		RepairAction   string `json:"repair_action"`
		RepairNotePath string `json:"repair_note_path"`
		RepairDue      string `json:"repair_due"` // YYYY-MM-DD
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.EventID == 0 || req.RootCause == "" {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {event_id, root_cause, note?, repair_action?, repair_note_path?, repair_due?}"))
		return
	}
	entry, err := s.Store.CreateError(req.EventID, req.RootCause,
		strings.TrimSpace(req.Note), strings.TrimSpace(req.RepairAction),
		strings.TrimSpace(req.RepairNotePath), strings.TrimSpace(req.RepairDue))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

// handleListErrors lists diagnoses filtered by status and cause.
func (s *Server) handleListErrors(w http.ResponseWriter, r *http.Request) {
	entries, err := s.Store.ListErrors(
		r.URL.Query().Get("status"), r.URL.Query().Get("cause"), 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// handlePatchError updates repair fields, the cause/note, or resolution.
func (s *Server) handlePatchError(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid error id"))
		return
	}
	var req struct {
		RootCause      *string `json:"root_cause"`
		Note           *string `json:"note"`
		RepairAction   *string `json:"repair_action"`
		RepairNotePath *string `json:"repair_note_path"`
		RepairDue      *string `json:"repair_due"`
		Resolved       *bool   `json:"resolved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body with fields to update"))
		return
	}
	entry, err := s.Store.UpdateError(id, store.ErrorPatch{
		RootCause:      req.RootCause,
		Note:           req.Note,
		RepairAction:   req.RepairAction,
		RepairNotePath: req.RepairNotePath,
		RepairDue:      req.RepairDue,
		Resolved:       req.Resolved,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

// handleErrorStats returns the diagnosis breakdown by cause and cause×deck.
func (s *Server) handleErrorStats(w http.ResponseWriter, r *http.Request) {
	byCause, byDeck, err := s.Store.ErrorStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"by_cause": byCause,
		"by_deck":  byDeck,
	})
}
