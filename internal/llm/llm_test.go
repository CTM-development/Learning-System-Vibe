package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeOpenRouter serves a canned chat completion and model list.
func fakeOpenRouter(t *testing.T, content string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, `{"error":{"message":"bad key"}}`, http.StatusUnauthorized)
			return
		}
		var req struct {
			Model    string    `json:"model"`
			Messages []Message `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model == "" || len(req.Messages) == 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": content}},
			},
			"usage": map[string]any{
				"prompt_tokens":     120,
				"completion_tokens": 80,
				"cost":              0.0003,
			},
		})
	})
	mux.HandleFunc("GET /models", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"anthropic/claude-haiku-4.5","name":"Claude Haiku 4.5","pricing":{"prompt":"0.000001","completion":"0.000005"}}]}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestChatAndUsage(t *testing.T) {
	srv := fakeOpenRouter(t, "hello")
	c := &Client{APIKey: "test-key", BaseURL: srv.URL}

	content, usage, err := c.Chat(context.Background(), "some/model",
		[]Message{{Role: "user", Content: "hi"}}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello" {
		t.Errorf("content = %q", content)
	}
	if usage.PromptTokens != 120 || usage.CompletionTokens != 80 || usage.Cost != 0.0003 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestChatWithoutKey(t *testing.T) {
	c := &Client{BaseURL: "http://unused"}
	_, _, err := c.Chat(context.Background(), "m", []Message{{Role: "user", Content: "x"}}, 10)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Errorf("err = %v", err)
	}
}

func TestChatAPIError(t *testing.T) {
	srv := fakeOpenRouter(t, "")
	c := &Client{APIKey: "wrong-key", BaseURL: srv.URL}
	_, _, err := c.Chat(context.Background(), "m", []Message{{Role: "user", Content: "x"}}, 10)
	if err == nil {
		t.Error("want error for bad key")
	}
}

func TestListModels(t *testing.T) {
	srv := fakeOpenRouter(t, "")
	c := &Client{APIKey: "test-key", BaseURL: srv.URL}
	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "anthropic/claude-haiku-4.5" {
		t.Errorf("models = %+v", models)
	}
}

func TestParseCards(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    int
		wantErr bool
	}{
		{"plain array", `[{"front":"f","back":"b"}]`, 1, false},
		{"fenced", "```json\n[{\"front\":\"f\",\"back\":\"b\"},{\"front\":\"f2\",\"back\":\"b2\"}]\n```", 2, false},
		{"prose around", "Here are your cards:\n[{\"front\":\"f\",\"back\":\"b\"}]\nEnjoy!", 1, false},
		{"blank entries dropped", `[{"front":"","back":"b"},{"front":"f","back":"b"}]`, 1, false},
		{"no array", "I cannot help with that.", 0, true},
		{"all blank", `[{"front":"","back":""}]`, 0, true},
		{"broken json", `[{"front":"f"`, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cards, err := ParseCards(tc.in)
			if tc.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if len(cards) != tc.want {
				t.Errorf("cards = %d, want %d", len(cards), tc.want)
			}
		})
	}
}

func TestCardPromptGrounding(t *testing.T) {
	msgs := CardPrompt("Title", "Content here", 5)
	if len(msgs) != 2 || msgs[0].Role != "system" {
		t.Fatalf("msgs = %+v", msgs)
	}
	if !strings.Contains(msgs[0].Content, "STRICTLY") {
		t.Error("system prompt lacks grounding rule")
	}
	if !strings.Contains(msgs[1].Content, "Content here") {
		t.Error("user prompt lacks note content")
	}
}
