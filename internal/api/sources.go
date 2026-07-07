package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/CTM-development/learning-system-vibe/internal/sources"
	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// handleUploadSource accepts one of: a multipart PDF upload (field "file",
// optional "title" and "key" fields), a multipart scan upload (repeated
// "pages" image fields, in order), or a JSON body {kind: url|book, title,
// key?, url?} registering a file-less reference. Returns the created source.
func (s *Server) handleUploadSource(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var req struct {
			Kind  string `json:"kind"`
			Title string `json:"title"`
			Key   string `json:"key"`
			URL   string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("want JSON body {kind, title, key?, url?}"))
			return
		}
		src, err := s.Sources.CreateReference(req.Kind, req.Title, req.Key, req.URL)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, src)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, sources.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("want multipart form with a 'file' field"))
		return
	}
	defer r.MultipartForm.RemoveAll()

	if pages := r.MultipartForm.File["pages"]; len(pages) > 0 {
		readers := make([]io.Reader, 0, len(pages))
		for _, fh := range pages {
			f, err := fh.Open()
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			defer f.Close()
			readers = append(readers, f)
		}
		src, err := s.Sources.SaveScan(r.FormValue("title"), r.FormValue("key"), readers)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, src)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("missing 'file' or 'pages' field"))
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
	if src.Kind != "pdf" || src.Path == "" {
		writeError(w, http.StatusNotFound, errors.New("source has no stored file"))
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

// handleScanPage streams one page image of a scan source. n is 1-based;
// the filename comes from the source's meta and stays confined to the
// attachments directory.
func (s *Server) handleScanPage(w http.ResponseWriter, r *http.Request) {
	src, ok := s.sourceFromPath(w, r)
	if !ok {
		return
	}
	if src.Kind != "scan" {
		writeError(w, http.StatusNotFound, errors.New("source is not a scan"))
		return
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || n < 1 {
		writeError(w, http.StatusBadRequest, errors.New("invalid page number"))
		return
	}
	pages := sources.ScanPages(src)
	if n > len(pages) {
		writeError(w, http.StatusNotFound, errors.New("page out of range"))
		return
	}
	pageSrc := src
	pageSrc.Path = src.Path + "/" + pages[n-1]
	path, err := s.Sources.FilePath(pageSrc)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	http.ServeFile(w, r, path)
}
