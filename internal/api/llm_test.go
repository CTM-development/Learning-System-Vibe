package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTM-development/learning-system-vibe/internal/llm"
)

// fakeOpenRouter returns proposals referencing the note's content.
func fakeOpenRouter(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat/completions", func(w http.ResponseWriter, r *http.Request) {
		content := `[{"front":"What does the ELBO lower-bound?","back":"The marginal log likelihood."},
		             {"front":"Why is VI mode-seeking?","back":"It minimizes KL(q||p)."}]`
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": content}}},
			"usage":   map[string]any{"prompt_tokens": 500, "completion_tokens": 200, "cost": 0.001},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestGenerateAcceptFlow(t *testing.T) {
	ts, srv, notesDir := newTestServer(t)
	st := srv.Store
	or := fakeOpenRouter(t)
	// Point the server's LLM client at the fake.
	srv.LLM = &llm.Client{APIKey: "test-key", BaseURL: or.URL}
	srv.Config.LLMDailyTokens = 10_000

	// A note to generate from.
	notePath := filepath.Join(notesDir, "vi.md")
	if err := os.WriteFile(notePath, []byte("# VI\n\nThe ELBO lower-bounds the evidence.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := postJSON(t, ts.URL+"/api/sync", nil)
	res.Body.Close()

	// Status: configured, budget fresh.
	status := decode[map[string]any](t, mustGet(t, ts.URL+"/api/llm/status"))
	if status["configured"] != true {
		t.Fatalf("status = %v", status)
	}

	// Generate proposals.
	res = postJSON(t, ts.URL+"/api/llm/generate-cards", map[string]any{
		"note_path": "vi.md", "count": 5,
	})
	if res.StatusCode != 200 {
		t.Fatalf("generate: %d", res.StatusCode)
	}
	gen := decode[struct {
		Cards []llm.ProposedCard `json:"cards"`
		Model string             `json:"model"`
		Usage llm.Usage          `json:"usage"`
	}](t, res)
	if len(gen.Cards) != 2 || gen.Usage.PromptTokens != 500 {
		t.Fatalf("gen = %+v", gen)
	}

	// The call was logged for budget accounting.
	tokens, cost, err := st.LLMUsageToday()
	if err != nil {
		t.Fatal(err)
	}
	if tokens != 700 || cost != 0.001 {
		t.Errorf("usage today = %d tokens / %v", tokens, cost)
	}

	// Accept one edited card.
	res = postJSON(t, ts.URL+"/api/llm/accept-cards", map[string]any{
		"note_path": "vi.md",
		"model":     gen.Model,
		"cards":     []map[string]string{{"front": "What does the ELBO lower-bound? (edited)", "back": "The evidence."}},
	})
	if res.StatusCode != 200 {
		t.Fatalf("accept: %d", res.StatusCode)
	}
	accepted := decode[struct {
		Added   int      `json:"added"`
		CardIDs []string `json:"card_ids"`
	}](t, res)
	if accepted.Added != 1 || len(accepted.CardIDs) != 1 {
		t.Fatalf("accepted = %+v", accepted)
	}

	// Card landed in the file under the generated heading, with an anchor.
	content, _ := os.ReadFile(notePath)
	text := string(content)
	if !strings.Contains(text, "## Test cards generated from this essay") {
		t.Error("generated heading missing")
	}
	if !strings.Contains(text, "Q: What does the ELBO lower-bound? (edited)") {
		t.Error("accepted card missing from file")
	}
	if !strings.Contains(text, "<!-- srs:"+accepted.CardIDs[0]) {
		t.Errorf("anchor for %s missing:\n%s", accepted.CardIDs[0], text)
	}

	// Provenance event recorded with the new card id.
	var payload string
	err = st.DB.QueryRow(
		`SELECT payload FROM activity_events WHERE kind = 'llm_generate'`).Scan(&payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, accepted.CardIDs[0]) {
		t.Errorf("provenance payload = %s", payload)
	}

	// Budget exhaustion blocks generation with 429.
	srv.Config.LLMDailyTokens = 100
	res = postJSON(t, ts.URL+"/api/llm/generate-cards", map[string]any{"note_path": "vi.md"})
	res.Body.Close()
	if res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("over budget: %d, want 429", res.StatusCode)
	}
}

func TestGenerateWithoutKey(t *testing.T) {
	ts, _, _ := newTestServer(t)
	res := postJSON(t, ts.URL+"/api/llm/generate-cards", map[string]any{"note_path": "x.md"})
	defer res.Body.Close()
	if res.StatusCode == 200 {
		t.Error("generation should fail without an API key")
	}
}
