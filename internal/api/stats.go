package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

func queryDays(r *http.Request, def int) int {
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 730 {
			return n
		}
	}
	return def
}

func (s *Server) handleStatsSummary(w http.ResponseWriter, r *http.Request) {
	sum, err := s.Store.Summary(s.Config.NewPerDay, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

func (s *Server) handleStatsHeatmap(w http.ResponseWriter, r *http.Request) {
	days, err := s.Store.ReviewHeatmap(queryDays(r, 182))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, days)
}

func (s *Server) handleStatsForecast(w http.ResponseWriter, r *http.Request) {
	forecast, overdue, err := s.Store.DueForecast(queryDays(r, 30))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"forecast": forecast,
		"overdue":  overdue,
	})
}

func (s *Server) handleStatsTime(w http.ResponseWriter, r *http.Request) {
	n := queryDays(r, 30)
	byKind, err := s.Store.TimeByKind(n)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	byDeck, err := s.Store.TimeByDeck(n)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"by_kind": byKind,
		"by_deck": byDeck,
	})
}

func (s *Server) handleBrowseCards(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cards, err := s.Store.BrowseCards(q.Get("q"), q.Get("deck"), q.Get("status"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, cards)
}

func (s *Server) handlePatchCard(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Suspended *bool `json:"suspended"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Suspended == nil {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {suspended: bool}"))
		return
	}
	id := r.PathValue("id")
	if err := s.Store.SetCardSuspended(id, *req.Suspended); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "suspended": *req.Suspended})
}

func (s *Server) handleListDecks(w http.ResponseWriter, r *http.Request) {
	decks, err := s.Store.ListDecks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, decks)
}

// handleSearch searches notes and extracted PDF text in one call.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	notes, err := s.Store.SearchNotes(q, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	srcs, err := s.Store.SearchSources(q, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"notes":   notes,
		"sources": srcs,
	})
}
