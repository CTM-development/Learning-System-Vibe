package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/CTM-development/learning-system-vibe/internal/mdsync"
	"github.com/CTM-development/learning-system-vibe/internal/sources"
)

// thoughtsDir is the conventional home of type:thought notes.
const thoughtsDir = "thoughts"

// handleCreateNote creates a markdown note file from the workbench: a
// transcribed paper note (type reading, optional stage) or a born-digital
// Thought (type thought, thoughts/YYYY-MM-DD-slug.md). The body is
// human-written — cards and questions in it become live on the immediate
// sync, by design. elapsed_ms carries the workbench editing time into a
// note_edit event.
func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title         string   `json:"title"`
		Type          string   `json:"type"`   // "reading" (default) | "thought"
		Stage         string   `json:"stage"`  // reading notes only
		Folder        string   `json:"folder"` // reading notes only, relative
		Tags          []string `json:"tags"`
		Sources       []string `json:"sources"` // citation keys, must exist
		Body          string   `json:"body"`
		TranscribedBy string   `json:"transcribed_by"` // model id when the draft came from AI
		ElapsedMs     int64    `json:"elapsed_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {title, type?, stage?, folder?, tags?, sources?, body?, transcribed_by?, elapsed_ms?}"))
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Type == "" {
		req.Type = "reading"
	}
	if req.Type != "reading" && req.Type != "thought" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid type %q (want reading or thought)", req.Type))
		return
	}
	if req.Type == "thought" {
		req.Stage = "" // thoughts are stageless
	} else if req.Stage != "" && !slices.Contains(mdsync.ValidStages, req.Stage) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid stage %q (want one of %s)", req.Stage, strings.Join(mdsync.ValidStages, "|")))
		return
	}
	for _, key := range req.Sources {
		exists, err := s.Store.SourceKeyExists(key)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if !exists {
			writeError(w, http.StatusBadRequest, fmt.Errorf("unknown source key %q", key))
			return
		}
	}

	slug := sources.Slugify(req.Title)
	if slug == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("title %q yields an empty slug", req.Title))
		return
	}

	// Slugified folder segments + slug filename make traversal impossible;
	// the absolute-path check below is defense in depth.
	folder := ""
	name := slug
	if req.Type == "thought" {
		folder = thoughtsDir
		name = time.Now().Format("2006-01-02") + "-" + slug
	} else if f := strings.Trim(req.Folder, "/"); f != "" {
		var segs []string
		for _, seg := range strings.Split(f, "/") {
			if seg = sources.Slugify(seg); seg != "" {
				segs = append(segs, seg)
			}
		}
		folder = strings.Join(segs, "/")
	}

	notesAbs, err := filepath.Abs(s.Syncer.NotesDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	rel, abs := notePathFor(notesAbs, folder, name)
	if !strings.HasPrefix(abs, notesAbs+string(filepath.Separator)) {
		writeError(w, http.StatusBadRequest, errors.New("note path escapes notes directory"))
		return
	}

	fm := map[string]any{"title": req.Title}
	if req.Type == "thought" {
		fm["type"] = "thought"
	}
	if req.Stage != "" {
		fm["stage"] = req.Stage
	}
	if len(req.Tags) > 0 {
		fm["tags"] = req.Tags
	}
	if len(req.Sources) > 0 {
		fm["sources"] = req.Sources
	}
	if req.TranscribedBy != "" {
		fm["transcribed_by"] = req.TranscribedBy
		fm["transcribed_at"] = time.Now().Format("2006-01-02")
	}
	fmYAML, _ := yaml.Marshal(fm)
	content := "---\n" + string(fmYAML) + "---\n\n# " + req.Title + "\n"
	if body := strings.TrimSpace(req.Body); body != "" {
		content += "\n" + body + "\n"
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := s.Syncer.SyncAll(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if _, err := s.Store.LogEvent("note_edit", rel, req.ElapsedMs, s.Store.ActiveSessionID(),
		map[string]any{"created": true, "type": req.Type, "sources": req.Sources}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	note, err := s.Store.GetNote(rel)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, note)
}

// notePathFor picks the first free filename (name.md, name-2.md, …) under
// notesAbs/folder and returns its slash-relative and absolute paths.
func notePathFor(notesAbs, folder, name string) (rel, abs string) {
	for i := 1; ; i++ {
		fname := name
		if i > 1 {
			fname = fmt.Sprintf("%s-%d", name, i)
		}
		rel = fname + ".md"
		if folder != "" {
			rel = folder + "/" + fname + ".md"
		}
		abs = filepath.Join(notesAbs, filepath.FromSlash(rel))
		if _, err := os.Stat(abs); os.IsNotExist(err) {
			return rel, abs
		}
	}
}
