package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// TestSourceUploadFlow drives upload → list → search (notes+sources) →
// file download.
func TestSourceUploadFlow(t *testing.T) {
	ts, _, _ := newTestServer(t)

	pdfData, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pdfs", "fixture.pdf"))
	if err != nil {
		t.Fatal(err)
	}

	// Multipart upload.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "spectral notes.pdf")
	fw.Write(pdfData)
	mw.WriteField("title", "Spectral Theory Notes")
	mw.Close()

	res, err := http.Post(ts.URL+"/api/sources", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("upload: %d %s", res.StatusCode, body)
	}
	src := decode[store.SourceRow](t, res)
	if src.Key != "spectral-notes" || src.Title != "Spectral Theory Notes" {
		t.Errorf("source = %+v", src)
	}

	// Listed.
	list := decode[[]store.SourceRow](t, mustGet(t, ts.URL+"/api/sources"))
	if len(list) != 1 {
		t.Fatalf("list = %+v", list)
	}

	// Searchable alongside notes.
	search := decode[struct {
		Notes   []store.SearchHit       `json:"notes"`
		Sources []store.SourceSearchHit `json:"sources"`
	}](t, mustGet(t, ts.URL+"/api/search?q=orthonormal"))
	if len(search.Sources) != 1 || search.Sources[0].SourceID != src.ID {
		t.Errorf("source search = %+v", search.Sources)
	}

	// File served inline as PDF.
	fileRes := mustGet(t, ts.URL+"/api/sources/1/file")
	defer fileRes.Body.Close()
	if ct := fileRes.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q", ct)
	}
	served, _ := io.ReadAll(fileRes.Body)
	if !bytes.Equal(served, pdfData) {
		t.Error("served file differs from upload")
	}

	// Unknown id → 404.
	res404, err := http.Get(ts.URL + "/api/sources/999")
	if err != nil {
		t.Fatal(err)
	}
	res404.Body.Close()
	if res404.StatusCode != http.StatusNotFound {
		t.Errorf("unknown source: %d", res404.StatusCode)
	}

	// Non-PDF rejected.
	var bad bytes.Buffer
	mw2 := multipart.NewWriter(&bad)
	fw2, _ := mw2.CreateFormFile("file", "junk.pdf")
	fw2.Write([]byte("plain text"))
	mw2.Close()
	resBad, err := http.Post(ts.URL+"/api/sources", mw2.FormDataContentType(), &bad)
	if err != nil {
		t.Fatal(err)
	}
	defer resBad.Body.Close()
	if resBad.StatusCode != http.StatusBadRequest {
		t.Errorf("non-PDF upload: %d", resBad.StatusCode)
	}
	var errBody struct {
		Error string `json:"error"`
	}
	json.NewDecoder(resBad.Body).Decode(&errBody)
	if !strings.Contains(errBody.Error, "not a PDF") {
		t.Errorf("error = %q", errBody.Error)
	}
}
