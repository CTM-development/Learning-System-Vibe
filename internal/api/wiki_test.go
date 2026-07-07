package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/CTM-development/learning-system-vibe/internal/llm"
	"github.com/CTM-development/learning-system-vibe/internal/store"
)

const fakeArticle = `Variational inference turns posterior inference into optimization.

## From your notes

The ELBO lower-bounds the evidence (note: ml/vi.md).

## Background

Coordinate ascent VI updates one factor at a time (uncertain).

Q: Should this become a card behind your back?
A: No — defused by the server.

## Connections

Related: [[KL divergence]], [[Bayes Rule]].

## Open questions

- How do normalizing flows change the picture?`

// fakeWikiOpenRouter serves an article and records prompts + call count.
func fakeWikiOpenRouter(t *testing.T, calls *atomic.Int32, lastPrompt *atomic.Value) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat/completions", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		lastPrompt.Store(string(body))
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": fakeArticle}}},
			"usage":   map[string]any{"prompt_tokens": 900, "completion_tokens": 400, "cost": 0.002},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestWikiGenerate drives the M7 loop: retrieval-grounded generation →
// article saved as an ordinary note → red links resolvable → provenance
// and budget recorded → no LLM cards sneak past the accept step → repeat
// call reuses the existing article without spending tokens.
func TestWikiGenerate(t *testing.T) {
	ts, srv, notesDir := newTestServer(t)
	var calls atomic.Int32
	var lastPrompt atomic.Value
	or := fakeWikiOpenRouter(t, &calls, &lastPrompt)
	srv.LLM = &llm.Client{APIKey: "test-key", BaseURL: or.URL}
	srv.Config.LLMDailyTokens = 10_000

	// Grounding material: a note that mentions the topic.
	if err := os.MkdirAll(filepath.Join(notesDir, "ml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notesDir, "ml/vi.md"),
		[]byte("---\ntitle: Variational Inference\n---\n\nThe ELBO lower-bounds the evidence.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	postJSON(t, ts.URL+"/api/sync", nil).Body.Close()

	res := postJSON(t, ts.URL+"/api/wiki/generate", map[string]string{
		"topic": "Variational Inference",
	})
	if res.StatusCode != 200 {
		t.Fatalf("generate wiki: %d", res.StatusCode)
	}
	gen := decode[struct {
		Path     string    `json:"path"`
		Model    string    `json:"model"`
		Usage    llm.Usage `json:"usage"`
		Existing bool      `json:"existing"`
	}](t, res)
	if gen.Path != "wiki/variational-inference.md" || gen.Existing {
		t.Fatalf("gen = %+v", gen)
	}

	// The prompt carried the note excerpt (retrieval grounding).
	prompt, _ := lastPrompt.Load().(string)
	if !strings.Contains(prompt, "The ELBO lower-bounds the evidence") ||
		!strings.Contains(prompt, "note: ml/vi.md") {
		t.Errorf("prompt lacks grounding excerpt: %.300s", prompt)
	}

	// File written with provenance frontmatter and the defused card block.
	raw, err := os.ReadFile(filepath.Join(notesDir, gen.Path))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"title: Variational Inference", "status: generated",
		"generated_by:", "grounding:", "# Variational Inference", "[[KL divergence]]"} {
		if !strings.Contains(text, want) {
			t.Errorf("article missing %q:\n%s", want, text)
		}
	}

	// The sneaky Q:/A: block must NOT have become a card.
	var wikiCards int
	if err := srv.Store.DB.QueryRow(
		`SELECT COUNT(*) FROM cards WHERE note_path = ?`, gen.Path).Scan(&wikiCards); err != nil {
		t.Fatal(err)
	}
	if wikiCards != 0 {
		t.Errorf("wiki article produced %d cards without human review", wikiCards)
	}

	// The article is an ordinary note now: fetchable, with a red link for
	// [[KL divergence]] and a resolved link to the grounding note's title.
	note := decode[store.NoteDetail](t, mustGet(t, ts.URL+"/api/notes/"+gen.Path))
	linkTo := map[string]string{}
	for _, l := range note.Links {
		linkTo[l.Target] = l.ToPath
	}
	if linkTo["KL divergence"] != "" {
		t.Errorf("KL divergence should be a red link, got %q", linkTo["KL divergence"])
	}

	// Budget accounting + provenance event.
	tokens, cost, err := srv.Store.LLMUsageToday()
	if err != nil {
		t.Fatal(err)
	}
	if tokens != 1300 || cost != 0.002 {
		t.Errorf("usage = %d tokens / %v", tokens, cost)
	}
	var payload string
	if err := srv.Store.DB.QueryRow(
		`SELECT payload FROM activity_events WHERE kind = 'llm_wiki'`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, "Variational Inference") || !strings.Contains(payload, "note: ml/vi.md") {
		t.Errorf("llm_wiki payload = %s", payload)
	}

	// Same topic again: no second LLM call, existing article returned.
	res = postJSON(t, ts.URL+"/api/wiki/generate", map[string]string{
		"topic": "Variational Inference",
	})
	again := decode[struct {
		Path     string `json:"path"`
		Existing bool   `json:"existing"`
	}](t, res)
	if !again.Existing || again.Path != gen.Path {
		t.Errorf("regenerate = %+v", again)
	}
	if calls.Load() != 1 {
		t.Errorf("LLM called %d times, want 1", calls.Load())
	}

	// Over budget → 429 for a new topic.
	srv.Config.LLMDailyTokens = 100
	res = postJSON(t, ts.URL+"/api/wiki/generate", map[string]string{"topic": "KL divergence"})
	if res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("over budget: %d, want 429", res.StatusCode)
	}
	res.Body.Close()
}
