package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/CTM-development/learning-system-vibe/internal/llm"
	"github.com/CTM-development/learning-system-vibe/internal/sources"
	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// maxTranscribeBytes caps the combined raw page bytes sent to the vision
// model per call — the client downscales to ~1600px, so hitting this
// means un-downscaled originals.
const maxTranscribeBytes = 20 << 20

// handleTranscribe drafts a markdown transcript of a scan source's pages
// with a vision model. Nothing is written to disk: the transcript
// pre-fills the workbench editor and the human corrects it against the
// original before saving. The output is defused of card syntax so an AI
// draft can't mint cards behind the review flow's back.
func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SourceID int64  `json:"source_id"`
		Model    string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SourceID == 0 {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {source_id, model?}"))
		return
	}
	if req.Model == "" {
		req.Model = s.Config.LLMModel
	}

	src, err := s.Store.GetSource(req.SourceID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if src.Kind != "scan" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("source %q is not a scan", src.Key))
		return
	}
	names := sources.ScanPages(src)
	if len(names) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("scan %q has no pages", src.Key))
		return
	}

	if err := s.checkBudget(); err != nil {
		writeError(w, http.StatusTooManyRequests, err)
		return
	}

	pages := make([]llm.ScanImage, 0, len(names))
	total := 0
	for _, name := range names {
		pageSrc := src
		pageSrc.Path = src.Path + "/" + name
		path, err := s.Sources.FilePath(pageSrc)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		total += len(data)
		if total > maxTranscribeBytes {
			writeError(w, http.StatusBadRequest, fmt.Errorf(
				"scan pages exceed %d MiB combined; re-upload downscaled images", maxTranscribeBytes>>20))
			return
		}
		pages = append(pages, llm.ScanImage{MIME: pageMIME(name), Data: data})
	}

	text, usage, err := s.LLM.Chat(r.Context(), req.Model, llm.TranscribePrompt(pages), 8000)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := s.Store.LogLLMCall(req.Model, "transcribe",
		usage.PromptTokens, usage.CompletionTokens, usage.Cost,
		map[string]any{"source_id": src.ID, "source_key": src.Key, "pages": len(pages)}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Same defusing as the wiki: the model is told not to emit card
	// syntax, but the guarantee is enforced here.
	text = strings.TrimSpace(text)
	text = cardLineRe.ReplaceAllString(text, " $1")
	text = clozeMarkRe.ReplaceAllString(text, "{{ c$1::")

	writeJSON(w, http.StatusOK, map[string]any{
		"text":       text,
		"model":      req.Model,
		"usage":      usage,
		"source_key": src.Key,
	})
}

// pageMIME maps a stored page filename (extension assigned by SaveScan)
// to its MIME type.
func pageMIME(name string) string {
	switch {
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".webp"):
		return "image/webp"
	default:
		return "image/jpeg"
	}
}
