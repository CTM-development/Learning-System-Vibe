package store

import (
	"encoding/json"
	"fmt"
)

// CardInfo is the card-browser projection: content plus schedule state.
type CardInfo struct {
	ID        string   `json:"id"`
	NotePath  string   `json:"note_path"`
	Type      string   `json:"type"`
	Front     string   `json:"front"`
	Back      string   `json:"back"`
	Deck      string   `json:"deck"`
	Tags      []string `json:"tags"`
	Suspended bool     `json:"suspended"`
	Orphaned  bool     `json:"orphaned"`
	Due       string   `json:"due"`
	State     int      `json:"state"`
	Reps      int      `json:"reps"`
	Lapses    int      `json:"lapses"`
}

// LeechLapses is the lapse count from which a card counts as a leech —
// repeatedly forgotten, likely badly formulated and worth rewriting.
const LeechLapses = 4

// BrowseCards lists cards filtered by free text (front/back LIKE), deck and
// status ("", "active", "suspended", "orphaned", "leech"), due first.
func (s *Store) BrowseCards(q, deck, status string) ([]CardInfo, error) {
	query := `
		SELECT c.id, c.note_path, c.type, c.front, c.back, c.deck, c.tags,
		       c.suspended, c.orphaned_at IS NOT NULL, cs.due, cs.state, cs.reps, cs.lapses
		FROM cards c
		JOIN card_schedule cs ON cs.card_id = c.id
		WHERE 1=1`
	var args []any
	if q != "" {
		query += ` AND (c.front LIKE ? OR c.back LIKE ?)`
		like := "%" + q + "%"
		args = append(args, like, like)
	}
	if deck != "" {
		query += ` AND c.deck = ?`
		args = append(args, deck)
	}
	switch status {
	case "", "active":
		query += ` AND c.suspended = 0 AND c.orphaned_at IS NULL`
	case "suspended":
		query += ` AND c.suspended = 1`
	case "orphaned":
		query += ` AND c.orphaned_at IS NOT NULL`
	case "leech":
		query += fmt.Sprintf(` AND c.suspended = 0 AND c.orphaned_at IS NULL AND cs.lapses >= %d`, LeechLapses)
	case "all":
		// no filter
	default:
		return nil, fmt.Errorf("invalid status %q", status)
	}
	query += ` ORDER BY cs.due LIMIT 500`

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CardInfo{}
	for rows.Next() {
		var c CardInfo
		var tags string
		if err := rows.Scan(&c.ID, &c.NotePath, &c.Type, &c.Front, &c.Back, &c.Deck,
			&tags, &c.Suspended, &c.Orphaned, &c.Due, &c.State, &c.Reps, &c.Lapses); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(tags), &c.Tags)
		if c.Tags == nil {
			c.Tags = []string{}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetCardSuspended toggles a card's suspended flag.
func (s *Store) SetCardSuspended(id string, suspended bool) error {
	res, err := s.DB.Exec(`UPDATE cards SET suspended = ? WHERE id = ?`, suspended, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("card %s: %w", id, ErrNotFound)
	}
	return nil
}

// ListDecks returns distinct decks with active-card counts.
func (s *Store) ListDecks() ([]TimeBucket, error) {
	rows, err := s.DB.Query(`
		SELECT COALESCE(NULLIF(deck, ''), '(root)'), COUNT(*)
		FROM cards WHERE orphaned_at IS NULL
		GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TimeBucket{}
	for rows.Next() {
		var b TimeBucket
		if err := rows.Scan(&b.Key, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// SearchHit is one FTS match over notes.
type SearchHit struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"` // matched terms wrapped in <mark>
}

// SearchNotes runs full-text search over note titles and content. The user
// query is quoted per term so FTS5 operator syntax cannot break it. Snippets
// are HTML-escaped (note content is untrusted) with matches in <mark> tags.
func (s *Store) SearchNotes(q string, limit int) ([]SearchHit, error) {
	match := ftsMatchQuery(q)
	if match == "" {
		return []SearchHit{}, nil
	}

	// Sentinels survive HTML escaping and become <mark> afterwards.
	rows, err := s.DB.Query(`
		SELECT path, title, snippet(notes_fts, 2, char(1), char(2), '…', 14)
		FROM notes_fts WHERE notes_fts MATCH ? ORDER BY rank LIMIT ?`,
		match, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SearchHit{}
	for rows.Next() {
		var h SearchHit
		if err := rows.Scan(&h.Path, &h.Title, &h.Snippet); err != nil {
			return nil, err
		}
		h.Snippet = escapeSnippet(h.Snippet)
		out = append(out, h)
	}
	return out, rows.Err()
}
