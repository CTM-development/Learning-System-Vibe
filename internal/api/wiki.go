package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/CTM-development/learning-system-vibe/internal/llm"
	"github.com/CTM-development/learning-system-vibe/internal/sources"
)

// wikiDir is the notes subfolder generated articles land in. They are
// ordinary notes from there on: searchable, linkable, card-generatable.
const wikiDir = "wiki"

// handleGenerateWiki generates a wiki article for a topic, grounded in the
// user's notes and extracted source text via FTS retrieval, and saves it as
// notes/wiki/<slug>.md. If the article already exists nothing is generated
// — the existing path is returned (no accidental double spend).
func (s *Server) handleGenerateWiki(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Topic string `json:"topic"`
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Topic) == "" {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {topic, model?}"))
		return
	}
	req.Topic = strings.TrimSpace(req.Topic)
	if req.Model == "" {
		req.Model = s.Config.LLMModel
	}
	slug := sources.Slugify(req.Topic)
	if slug == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("topic %q yields an empty slug", req.Topic))
		return
	}
	rel := wikiDir + "/" + slug + ".md"
	abs := filepath.Join(s.Syncer.NotesDir, wikiDir, slug+".md")

	if _, err := os.Stat(abs); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"path": rel, "existing": true})
		return
	}

	if err := s.checkBudget(); err != nil {
		writeError(w, http.StatusTooManyRequests, err)
		return
	}

	excerpts, err := s.retrieveExcerpts(req.Topic)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	article, usage, err := s.LLM.Chat(r.Context(), req.Model,
		llm.WikiPrompt(req.Topic, excerpts), 6000)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := s.Store.LogLLMCall(req.Model, "wiki_article",
		usage.PromptTokens, usage.CompletionTokens, usage.Cost,
		map[string]string{"topic": req.Topic, "path": rel}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	origins := make([]string, 0, len(excerpts))
	for _, e := range excerpts {
		origins = append(origins, e.Origin)
	}
	content := wikiFileContent(req.Topic, req.Model, origins, article)
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

	// Provenance trail, same as generated cards: model → topic → file.
	if _, err := s.Store.LogEvent("llm_wiki", rel, 0, s.Store.ActiveSessionID(), map[string]any{
		"model":     req.Model,
		"topic":     req.Topic,
		"grounding": origins,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"path":     rel,
		"model":    req.Model,
		"usage":    usage,
		"existing": false,
	})
}

// retrieveExcerpts pulls the FTS-best notes and source texts for a topic
// as grounding material.
func (s *Server) retrieveExcerpts(topic string) ([]llm.Excerpt, error) {
	var out []llm.Excerpt

	noteHits, err := s.Store.SearchNotes(topic, 4)
	if err != nil {
		return nil, err
	}
	for _, h := range noteHits {
		note, err := s.Store.GetNote(h.Path)
		if err != nil {
			continue // note vanished between search and read
		}
		text := stripFrontmatter(note.Content)
		out = append(out, llm.Excerpt{
			Origin: "note: " + h.Path,
			Title:  note.Title,
			Text:   truncateRunes(text, 2500),
		})
	}

	sourceHits, err := s.Store.SearchSources(topic, 3)
	if err != nil {
		return nil, err
	}
	for _, h := range sourceHits {
		src, err := s.Store.GetSource(h.SourceID)
		if err != nil {
			continue
		}
		text, err := s.Store.SourceText(h.SourceID)
		if err != nil || strings.TrimSpace(text) == "" {
			continue
		}
		out = append(out, llm.Excerpt{
			Origin: "source: " + src.Key,
			Title:  src.Title,
			Text:   truncateRunes(text, 1500),
		})
	}
	return out, nil
}

var (
	frontmatterRe = regexp.MustCompile(`(?s)^---\n.*?\n---\n?`)
	cardLineRe    = regexp.MustCompile(`(?m)^([QA]:)`)
	clozeMarkRe   = regexp.MustCompile(`\{\{c(\d+)::`)
)

func stripFrontmatter(content string) string {
	return frontmatterRe.ReplaceAllString(content, "")
}

// wikiFileContent assembles the article file: provenance frontmatter, the
// title heading, and the body defused of card syntax — AI-written text must
// not become cards behind the human-review flow's back.
func wikiFileContent(topic, model string, grounding []string, article string) string {
	article = strings.TrimSpace(article)
	article = cardLineRe.ReplaceAllString(article, " $1")   // ^Q:/A: would card on sync
	article = clozeMarkRe.ReplaceAllString(article, "{{ c$1::") // defuse cloze markers

	fm := map[string]any{
		"title":        topic,
		"status":       "generated",
		"tags":         []string{"wiki"},
		"generated_by": model,
		"generated_at": time.Now().Format("2006-01-02"),
	}
	if len(grounding) > 0 {
		fm["grounding"] = grounding
	}
	fmYAML, _ := yaml.Marshal(fm)
	return "---\n" + string(fmYAML) + "---\n\n# " + topic + "\n\n" + article + "\n"
}

func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + " […]"
}
