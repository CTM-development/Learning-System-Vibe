package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/CTM-development/learning-system-vibe/internal/llm"
	"github.com/CTM-development/learning-system-vibe/internal/mdsync"
	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// modelsCache holds the OpenRouter catalog for an hour.
type modelsCache struct {
	mu      sync.Mutex
	models  []llm.ModelInfo
	fetched time.Time
}

func (s *Server) handleLLMStatus(w http.ResponseWriter, r *http.Request) {
	tokens, cost, err := s.Store.LLMUsageToday()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":   s.LLM.APIKey != "",
		"model":        s.Config.LLMModel,
		"daily_tokens": s.Config.LLMDailyTokens,
		"tokens_today": tokens,
		"cost_today":   cost,
	})
}

func (s *Server) handleLLMModels(w http.ResponseWriter, r *http.Request) {
	s.Models.mu.Lock()
	defer s.Models.mu.Unlock()
	if time.Since(s.Models.fetched) > time.Hour || s.Models.models == nil {
		models, err := s.LLM.ListModels(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		s.Models.models = models
		s.Models.fetched = time.Now()
	}
	writeJSON(w, http.StatusOK, s.Models.models)
}

// checkBudget returns an error when today's token usage is at or over the
// configured daily budget.
func (s *Server) checkBudget() error {
	tokens, _, err := s.Store.LLMUsageToday()
	if err != nil {
		return err
	}
	if tokens >= s.Config.LLMDailyTokens {
		return fmt.Errorf("daily LLM token budget exhausted (%d/%d used); try again tomorrow or raise llm_daily_tokens",
			tokens, s.Config.LLMDailyTokens)
	}
	return nil
}

// handleGenerateCards asks the LLM for card proposals grounded in one
// note. Nothing is written — the human accept/edit step follows.
func (s *Server) handleGenerateCards(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NotePath string `json:"note_path"`
		Model    string `json:"model"`
		Count    int    `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NotePath == "" {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {note_path, model?, count?}"))
		return
	}
	if req.Model == "" {
		req.Model = s.Config.LLMModel
	}
	if req.Count <= 0 {
		req.Count = 8
	}
	req.Count = min(req.Count, 20)

	if err := s.checkBudget(); err != nil {
		writeError(w, http.StatusTooManyRequests, err)
		return
	}
	note, err := s.Store.GetNote(req.NotePath)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	content, usage, err := s.LLM.Chat(r.Context(), req.Model,
		llm.CardPrompt(note.Title, note.Content, req.Count), 4000)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := s.Store.LogLLMCall(req.Model, "generate_cards",
		usage.PromptTokens, usage.CompletionTokens, usage.Cost,
		map[string]string{"note_path": req.NotePath}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	cards, err := llm.ParseCards(content)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cards": cards,
		"model": req.Model,
		"usage": usage,
	})
}

// handleAcceptCards writes human-approved cards into the note under the
// generated-cards heading, syncs (assigning anchors), and records
// provenance: which card ids came from which model for which note.
func (s *Server) handleAcceptCards(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NotePath string             `json:"note_path"`
		Model    string             `json:"model"`
		Cards    []llm.ProposedCard `json:"cards"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NotePath == "" || len(req.Cards) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("want JSON body {note_path, model, cards: [{front, back}]}"))
		return
	}

	before, err := s.Store.ListActiveCardIDs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	beforeSet := make(map[string]bool, len(before))
	for _, id := range before {
		beforeSet[id] = true
	}

	blocks := make([]mdsync.QABlock, 0, len(req.Cards))
	for _, c := range req.Cards {
		blocks = append(blocks, mdsync.QABlock{Front: c.Front, Back: c.Back})
	}
	if err := s.Syncer.AppendQABlocks(req.NotePath, mdsync.GeneratedHeading, blocks); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.Syncer.SyncAll(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	after, err := s.Store.ListActiveCardIDs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var newIDs []string
	for _, id := range after {
		if !beforeSet[id] {
			newIDs = append(newIDs, id)
		}
	}

	// Provenance trail: the generation event links model → note → card ids.
	if err := s.Store.LogEvent("llm_generate", req.NotePath, 0, s.Store.ActiveSessionID(),
		map[string]any{"model": req.Model, "card_ids": newIDs, "accepted": len(req.Cards)}); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"added":    len(newIDs),
		"card_ids": newIDs,
	})
}
