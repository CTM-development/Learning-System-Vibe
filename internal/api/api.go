// Package api wires HTTP routes: JSON endpoints under /api and the embedded
// frontend for everything else (SPA fallback to index.html).
package api

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/CTM-development/learning-system-vibe/internal/config"
	"github.com/CTM-development/learning-system-vibe/internal/mdsync"
	"github.com/CTM-development/learning-system-vibe/internal/sources"
	"github.com/CTM-development/learning-system-vibe/internal/srs"
	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// Server holds handler dependencies.
type Server struct {
	Store     *store.Store
	Syncer    *mdsync.Syncer
	Scheduler *srs.Scheduler
	Sources   *sources.Manager
	Config    config.Config
	Version   string
}

// Handler builds the root http.Handler.
func (s *Server) Handler(dist fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/sync", s.handleSync)
	mux.HandleFunc("GET /api/notes", s.handleListNotes)
	mux.HandleFunc("GET /api/notes/{path...}", s.handleGetNote)
	mux.HandleFunc("POST /api/notes/stage", s.handleSetStage)
	mux.HandleFunc("GET /api/questions", s.handleListQuestions)
	mux.HandleFunc("GET /api/queue", s.handleQueue)
	mux.HandleFunc("POST /api/reviews", s.handleReview)
	mux.HandleFunc("POST /api/sessions/start", s.handleSessionStart)
	mux.HandleFunc("POST /api/sessions/stop", s.handleSessionStop)
	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("POST /api/events", s.handlePostEvent)
	mux.HandleFunc("GET /api/stats/summary", s.handleStatsSummary)
	mux.HandleFunc("GET /api/stats/heatmap", s.handleStatsHeatmap)
	mux.HandleFunc("GET /api/stats/forecast", s.handleStatsForecast)
	mux.HandleFunc("GET /api/stats/time", s.handleStatsTime)
	mux.HandleFunc("GET /api/cards", s.handleBrowseCards)
	mux.HandleFunc("PATCH /api/cards/{id}", s.handlePatchCard)
	mux.HandleFunc("GET /api/decks", s.handleListDecks)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("POST /api/sources", s.handleUploadSource)
	mux.HandleFunc("GET /api/sources", s.handleListSources)
	mux.HandleFunc("GET /api/sources/{id}", s.handleGetSource)
	mux.HandleFunc("GET /api/sources/{id}/file", s.handleSourceFile)
	mux.Handle("/", spaHandler(dist))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DB.Ping(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "degraded", "error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": s.Version,
	})
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	res, err := s.Syncer.SyncAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	notes, err := s.Store.ListNotes(r.URL.Query().Get("stage"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, notes)
}

func (s *Server) handleGetNote(w http.ResponseWriter, r *http.Request) {
	note, err := s.Store.GetNote(r.PathValue("path"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, note)
}

func (s *Server) handleSetStage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path  string `json:"path"`
		Stage string `json:"stage"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {path, stage}"))
		return
	}
	if err := s.Syncer.SetStage(req.Path, req.Stage); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.Syncer.SyncAll(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	note, err := s.Store.GetNote(req.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, note)
}

func (s *Server) handleListQuestions(w http.ResponseWriter, r *http.Request) {
	questions, err := s.Store.ListOpenQuestions(r.URL.Query().Get("status"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, questions)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// spaHandler serves static files from dist; unknown paths (client-side
// routes) fall back to index.html. If the frontend was never built it
// returns a plain notice instead of 404s.
func spaHandler(dist fs.FS) http.Handler {
	fileServer := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(dist, p); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		index, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			http.Error(w, "UI not built. Run `make web` and rebuild the server.", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	})
}
