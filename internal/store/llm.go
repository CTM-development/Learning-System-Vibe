package store

import "encoding/json"

// LogLLMCall records one OpenRouter call for budget and cost tracking.
func (s *Store) LogLLMCall(model, purpose string, tokensIn, tokensOut int64, cost float64, meta any) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if meta == nil {
		data = []byte("{}")
	}
	_, err = s.DB.Exec(
		`INSERT INTO llm_calls (model, purpose, tokens_in, tokens_out, cost, meta)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		model, purpose, tokensIn, tokensOut, cost, string(data))
	return err
}

// LLMUsageToday sums today's token consumption and spend (local time).
func (s *Store) LLMUsageToday() (tokens int64, cost float64, err error) {
	err = s.DB.QueryRow(`
		SELECT COALESCE(SUM(tokens_in + tokens_out), 0), COALESCE(SUM(cost), 0)
		FROM llm_calls
		WHERE date(ts, 'localtime') = date('now', 'localtime')`).Scan(&tokens, &cost)
	return tokens, cost, err
}
