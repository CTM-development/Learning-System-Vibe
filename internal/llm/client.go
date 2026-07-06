// Package llm is a minimal OpenRouter client (OpenAI-compatible chat
// completions) plus prompt building and response parsing for card
// generation. The API key never leaves the server.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to OpenRouter (or a compatible endpoint in tests).
type Client struct {
	APIKey  string
	BaseURL string // e.g. https://openrouter.ai/api/v1
	HTTP    *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 120 * time.Second}
}

// Message is one chat turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Usage is the token/cost accounting OpenRouter reports per call.
type Usage struct {
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	Cost             float64 `json:"cost"`
}

// Chat runs one chat completion and returns the assistant text + usage.
func (c *Client) Chat(ctx context.Context, model string, messages []Message, maxTokens int) (string, Usage, error) {
	if c.APIKey == "" {
		return "", Usage{}, fmt.Errorf("OpenRouter API key not configured (set openrouter_api_key or LEARN_OPENROUTER_API_KEY)")
	}
	body, err := json.Marshal(map[string]any{
		"model":      model,
		"messages":   messages,
		"max_tokens": maxTokens,
		"usage":      map[string]any{"include": true},
	})
	if err != nil {
		return "", Usage{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", Usage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Title", "learning-system")

	res, err := c.http().Do(req)
	if err != nil {
		return "", Usage{}, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return "", Usage{}, err
	}
	if res.StatusCode != http.StatusOK {
		return "", Usage{}, fmt.Errorf("openrouter %s: %s", res.Status, truncate(data, 300))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", Usage{}, fmt.Errorf("parse openrouter response: %w", err)
	}
	if parsed.Error != nil {
		return "", Usage{}, fmt.Errorf("openrouter: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", Usage{}, fmt.Errorf("openrouter returned no choices")
	}
	return parsed.Choices[0].Message.Content, parsed.Usage, nil
}

// ModelInfo is one entry of OpenRouter's model catalog.
type ModelInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

// ListModels fetches the model catalog (for the UI picker).
func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	res, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return nil, fmt.Errorf("openrouter models %s: %s", res.Status, data)
	}
	var parsed struct {
		Data []ModelInfo `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	return parsed.Data, nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "…"
	}
	return string(b)
}
