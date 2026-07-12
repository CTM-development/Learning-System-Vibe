package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// handleQueue returns today's review queue: due cards plus new cards up to
// the daily limit, optionally scoped to a deck or a project (mutually
// exclusive). A project with a deadline paces new cards so the whole
// backlog is introduced before the deadline, raising the daily limit when
// needed. With cram=1 (requires a deck or project) it instead returns every
// active card in scope, weakest first, ignoring due dates — for exam prep.
func (s *Server) handleQueue(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	deck := r.URL.Query().Get("deck")
	projectParam := r.URL.Query().Get("project")
	if deck != "" && projectParam != "" {
		writeError(w, http.StatusBadRequest, errors.New("deck and project are mutually exclusive"))
		return
	}

	var decks []string
	var project *store.Project
	if projectParam != "" {
		id, err := strconv.ParseInt(projectParam, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid project id"))
			return
		}
		p, err := s.Store.GetProject(id)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		project = &p
		decks = p.Dirs
	} else if deck != "" {
		decks = []string{deck}
	}

	if r.URL.Query().Get("cram") == "1" {
		if len(decks) == 0 {
			writeError(w, http.StatusBadRequest, errors.New("cram mode needs a deck or project"))
			return
		}
		cards, err := s.Store.CramCards(now, decks, 500)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"due":           cards,
			"new":           []store.QueueCard{},
			"new_remaining": 0,
			"cram":          true,
		})
		return
	}

	due, err := s.Store.DueCards(now, decks)
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

	resp := map[string]any{"due": due}
	if project != nil {
		resp["project"] = project.ID
		if project.Deadline != "" {
			// Deadline pacing: spread the project's new-card backlog over
			// the remaining days. The quota only ever raises the global
			// limit, and it is computed against project-scoped counts so it
			// stays stable across repeated fetches within a day.
			left, err := daysLeft(project.Deadline, now)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			remainingNew, err := s.Store.CountNewCards(decks)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			introducedProj, err := s.Store.CountNewIntroducedTodayForDecks(decks)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			target := targetNewToday(remainingNew, introducedProj, left, s.Config.NewPerDay)
			remaining = target - introducedProj
			resp["deadline"] = project.Deadline
			resp["days_left"] = left
			resp["target_new_today"] = target
		}
	}

	newCards, err := s.Store.NewCards(now, remaining, decks)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	resp["new"] = newCards
	resp["new_remaining"] = max(remaining, 0)
	writeJSON(w, http.StatusOK, resp)
}

// handleReview applies a rating to a card, persists the new FSRS state and
// appends a card_review event carrying the before/after snapshot.
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CardID    string `json:"card_id"`
		Rating    int    `json:"rating"`
		ElapsedMs int64  `json:"elapsed_ms"`
		Cram      bool   `json:"cram"`
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

	now := time.Now()
	after, err := s.Scheduler.Review(before, req.Rating, now)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	payload := map[string]any{
		"rating": req.Rating,
		"before": before,
	}
	if req.Cram {
		payload["cram"] = true
	}

	// Deadline cap: when a project with an active deadline covers this
	// card's deck, squeeze the FSRS due date so the card surfaces at least
	// once more before the deadline. FSRS fuzz ran inside Review, so the
	// cap is final; undo is unaffected because it restores the before
	// snapshot.
	card, err := s.Store.GetCard(req.CardID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	deadline, active, err := s.Store.EarliestActiveDeadline(card.Deck, now.Format("2006-01-02"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if active {
		capTime, err := parseLocalDate(deadline)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if due, capped := capDue(after.Due, now, capTime); capped {
			payload["deadline_capped"] = true
			payload["fsrs_due"] = after.Due
			after.Due = due
			after.ScheduledDays = uint64(due.Sub(now).Hours() / 24)
		}
	}
	payload["after"] = after

	if err := s.Store.UpdateSchedule(after); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	eventID, err := s.Store.LogEvent("card_review", req.CardID, req.ElapsedMs,
		s.Store.ActiveSessionID(), payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// event_id lets the client classify a failure into the error log.
	writeJSON(w, http.StatusOK, map[string]any{
		"schedule": after,
		"event_id": eventID,
	})
}

// handleUndoReview reverts the most recent not-yet-undone review: the
// card's schedule is restored to the pre-review snapshot stored in the
// event payload. The log stays append-only — a review_undo event marks the
// review as reverted instead of deleting it.
func (s *Server) handleUndoReview(w http.ResponseWriter, r *http.Request) {
	eventID, cardID, before, err := s.Store.LatestUndoableReview()
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.Store.UpdateSchedule(before); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := s.Store.LogEvent("review_undo", cardID, 0, s.Store.ActiveSessionID(),
		map[string]any{"event_id": eventID}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"card_id":  cardID,
		"schedule": before,
	})
}

// handleBuryCard hides a card until local tomorrow without rating it.
func (s *Server) handleBuryCard(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	now := time.Now()
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	if err := s.Store.BuryCard(id, tomorrow); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := s.Store.LogEvent("card_bury", id, 0, s.Store.ActiveSessionID(), nil); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"buried_until": tomorrow.UTC().Format(time.RFC3339),
	})
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
	if _, err := s.Store.LogEvent(req.Kind, req.Ref, req.ElapsedMs, s.Store.ActiveSessionID(), payload); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
