package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/CTM-development/learning-system-vibe/internal/sources"
	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// handleUploadSource accepts a multipart PDF upload (field "file", optional
// "title" and "key" fields) and returns the created source.
func (s *Server) handleUploadSource(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, sources.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("want multipart form with a 'file' field"))
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("missing 'file' field"))
		return
	}
	defer file.Close()

	src, err := s.Sources.SavePDF(
		header.Filename,
		r.FormValue("title"),
		r.FormValue("key"),
		file,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, src)
}

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListSources()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) sourceFromPath(w http.ResponseWriter, r *http.Request) (store.SourceRow, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid source id"))
		return store.SourceRow{}, false
	}
	src, err := s.Store.GetSource(id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return src, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return src, false
	}
	return src, true
}

func (s *Server) handleGetSource(w http.ResponseWriter, r *http.Request) {
	if src, ok := s.sourceFromPath(w, r); ok {
		writeJSON(w, http.StatusOK, src)
	}
}

// handleSourceFile streams the stored PDF for in-browser viewing.
func (s *Server) handleSourceFile(w http.ResponseWriter, r *http.Request) {
	src, ok := s.sourceFromPath(w, r)
	if !ok {
		return
	}
	path, err := s.Sources.FilePath(src)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+src.Key+`.pdf"`)
	http.ServeFile(w, r, path)
}
