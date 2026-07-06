package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// handleQueue returns today's review queue: due cards plus new cards up to
// the daily limit.
func (s *Server) handleQueue(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	due, err := s.Store.DueCards(now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	introduced, err := s.Store.CountNewIntroducedToday()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	remaining := s.Config.NewPerDay - introduced
	newCards, err := s.Store.NewCards(remaining)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"due":           due,
		"new":           newCards,
		"new_remaining": max(remaining, 0),
	})
}

// handleReview applies a rating to a card, persists the new FSRS state and
// appends a card_review event carrying the before/after snapshot.
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CardID    string `json:"card_id"`
		Rating    int    `json:"rating"`
		ElapsedMs int64  `json:"elapsed_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CardID == "" {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {card_id, rating, elapsed_ms}"))
		return
	}

	before, err := s.Store.GetSchedule(req.CardID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	after, err := s.Scheduler.Review(before, req.Rating, time.Now())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Store.UpdateSchedule(after); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	err = s.Store.LogEvent("card_review", req.CardID, req.ElapsedMs,
		s.Store.ActiveSessionID(), map[string]any{
			"rating": req.Rating,
			"before": before,
			"after":  after,
		})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, after)
}

// handleSessionStart starts a session (ending any active one).
func (s *Server) handleSessionStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind string `json:"kind"`
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {kind, note?}"))
		return
	}
	sess, err := s.Store.StartSession(req.Kind, req.Note)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

func (s *Server) handleSessionStop(w http.ResponseWriter, r *http.Request) {
	sess, err := s.Store.StopSession()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// handleListSessions returns the active session (if any) and recent history.
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.Store.ListSessions(50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var active *store.Session
	if len(sessions) > 0 && sessions[0].EndedAt == "" {
		active = &sessions[0]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active":  active,
		"recent":  sessions,
		"serverTime": time.Now().UTC().Format(time.RFC3339),
	})
}

// handlePostEvent records a client-reported timed activity (e.g. note
// reading stints from focus heartbeats).
func (s *Server) handlePostEvent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind      string          `json:"kind"`
		Ref       string          `json:"ref"`
		ElapsedMs int64           `json:"elapsed_ms"`
		Payload   json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Kind == "" {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {kind, ref?, elapsed_ms?, payload?}"))
		return
	}
	var payload any = map[string]any{}
	if len(req.Payload) > 0 {
		payload = req.Payload
	}
	if err := s.Store.LogEvent(req.Kind, req.Ref, req.ElapsedMs, s.Store.ActiveSessionID(), payload); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
