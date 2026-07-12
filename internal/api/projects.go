package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// ProjectInfo is a Project enriched with card statistics for display.
type ProjectInfo struct {
	store.Project
	TotalCards int  `json:"total_cards"`
	NewCards   int  `json:"new_cards"`
	DueNow     int  `json:"due_now"`
	DaysLeft   *int `json:"days_left"` // nil when no deadline
}

// projectInfo joins a project with its live card stats and days-left. Uses
// the existing daysLeft (deadline.go) for the study-day count; a project's
// deadline is already validated at write time, so an error here only means
// "no deadline set" or a malformed value, both of which render as nil.
func (s *Server) projectInfo(p store.Project, now time.Time) (ProjectInfo, error) {
	total, newCount, dueNow, err := s.Store.ProjectCardStats(p.Dirs, now)
	if err != nil {
		return ProjectInfo{}, err
	}
	info := ProjectInfo{
		Project:    p,
		TotalCards: total,
		NewCards:   newCount,
		DueNow:     dueNow,
	}
	if p.Deadline != "" {
		if n, err := daysLeft(p.Deadline, now); err == nil {
			info.DaysLeft = &n
		}
	}
	return info, nil
}

// handleListProjects lists every project with live card stats.
func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.Store.ListProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	now := time.Now()
	out := make([]ProjectInfo, 0, len(projects))
	for _, p := range projects {
		info, err := s.projectInfo(p, now)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		out = append(out, info)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateProject creates a project with a name, its directories, and
// an optional deadline.
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string   `json:"name"`
		Dirs     []string `json:"dirs"`
		Deadline string   `json:"deadline"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {name, dirs, deadline?}"))
		return
	}
	p, err := s.Store.CreateProject(req.Name, req.Deadline, req.Dirs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	info, err := s.projectInfo(p, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handlePatchProject updates a project's name, dirs, and/or deadline.
// Absent fields (nil pointer, nil dirs slice) are left unchanged; an
// explicit empty deadline clears it.
func (s *Server) handlePatchProject(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid project id"))
		return
	}
	var req struct {
		Name     *string  `json:"name"`
		Dirs     []string `json:"dirs"`
		Deadline *string  `json:"deadline"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body with fields to update"))
		return
	}
	p, err := s.Store.UpdateProject(id, req.Name, req.Deadline, req.Dirs)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	info, err := s.projectInfo(p, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleDeleteProject deletes a project and its dirs.
func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid project id"))
		return
	}
	if err := s.Store.DeleteProject(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}
