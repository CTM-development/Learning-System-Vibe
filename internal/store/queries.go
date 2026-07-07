package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// NoteRow mirrors the notes table.
type NoteRow struct {
	Path        string
	Title       string
	Frontmatter map[string]any
	Stage       string
	Status      string
	Type        string // "reading" | "thought"
	Tags        []string
	Sources     []string
	Mtime       int64
	Content     string
}

// CardRow mirrors the cards table.
type CardRow struct {
	ID       string
	NotePath string
	Type     string
	Front    string
	Back     string
	Deck     string
	Tags     []string
}

// UpsertNote inserts or updates a note and refreshes its FTS row.
func (s *Store) UpsertNote(n NoteRow) error {
	fm, _ := json.Marshal(n.Frontmatter)
	tags, _ := json.Marshal(n.Tags)
	sources, _ := json.Marshal(n.Sources)
	noteType := n.Type
	if noteType == "" {
		noteType = "reading"
	}
	_, err := s.DB.Exec(`
		INSERT INTO notes (path, title, frontmatter, stage, status, type, tags, sources, mtime, content)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			title = excluded.title, frontmatter = excluded.frontmatter,
			stage = excluded.stage, status = excluded.status,
			type = excluded.type,
			tags = excluded.tags, sources = excluded.sources,
			mtime = excluded.mtime, content = excluded.content`,
		n.Path, n.Title, string(fm), nullIfEmpty(n.Stage), nullIfEmpty(n.Status),
		noteType, string(tags), string(sources), n.Mtime, n.Content)
	if err != nil {
		return fmt.Errorf("upsert note %s: %w", n.Path, err)
	}
	if _, err := s.DB.Exec(`DELETE FROM notes_fts WHERE path = ?`, n.Path); err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO notes_fts (path, title, content) VALUES (?, ?, ?)`,
		n.Path, n.Title, n.Content)
	return err
}

// ListNotePaths returns every note path currently stored.
func (s *Store) ListNotePaths() ([]string, error) {
	return s.queryStrings(`SELECT path FROM notes`)
}

// DeleteNote removes a note row and its FTS entry (cards are handled
// separately via orphaning).
func (s *Store) DeleteNote(path string) error {
	if _, err := s.DB.Exec(`DELETE FROM notes WHERE path = ?`, path); err != nil {
		return err
	}
	if _, err := s.DB.Exec(`DELETE FROM note_links WHERE from_path = ?`, path); err != nil {
		return err
	}
	_, err := s.DB.Exec(`DELETE FROM notes_fts WHERE path = ?`, path)
	return err
}

// UpsertCard inserts a card (with a fresh schedule row due immediately) or
// updates its content snapshot, clearing any orphan mark. Scheduling state
// is never touched on update — editing wording must not reset history.
// Returns true when the card was newly created.
func (s *Store) UpsertCard(c CardRow) (bool, error) {
	tags, _ := json.Marshal(c.Tags)
	res, err := s.DB.Exec(`
		INSERT INTO cards (id, note_path, type, front, back, deck, tags)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			note_path = excluded.note_path, type = excluded.type,
			front = excluded.front, back = excluded.back,
			deck = excluded.deck, tags = excluded.tags,
			orphaned_at = NULL,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		WHERE cards.note_path != excluded.note_path
		   OR cards.front != excluded.front OR cards.back != excluded.back
		   OR cards.deck != excluded.deck OR cards.tags != excluded.tags
		   OR cards.orphaned_at IS NOT NULL`,
		c.ID, c.NotePath, c.Type, c.Front, c.Back, c.Deck, string(tags))
	if err != nil {
		return false, fmt.Errorf("upsert card %s: %w", c.ID, err)
	}

	// Ensure a schedule row exists (new cards: due now, state 0 = new).
	created := false
	err = s.DB.QueryRow(`SELECT NOT EXISTS (SELECT 1 FROM card_schedule WHERE card_id = ?)`, c.ID).Scan(&created)
	if err != nil {
		return false, err
	}
	if created {
		_, err = s.DB.Exec(`INSERT INTO card_schedule (card_id, due) VALUES (?, ?)`,
			c.ID, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			return false, err
		}
	}
	_ = res
	return created, nil
}

// ListActiveCardIDs returns ids of all non-orphaned cards.
func (s *Store) ListActiveCardIDs() ([]string, error) {
	return s.queryStrings(`SELECT id FROM cards WHERE orphaned_at IS NULL`)
}

// ListCardBaseIDs returns the anchor part of every card id (cloze ids are
// "anchor#n"), used for collision-free anchor generation.
func (s *Store) ListCardBaseIDs() (map[string]bool, error) {
	ids, err := s.queryStrings(`SELECT DISTINCT substr(id, 1, CASE WHEN instr(id,'#') > 0 THEN instr(id,'#')-1 ELSE length(id) END) FROM cards`)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set, nil
}

// OrphanCards soft-deletes the given card ids (keeps schedule + history).
func (s *Store) OrphanCards(ids []string) error {
	for _, id := range ids {
		if _, err := s.DB.Exec(
			`UPDATE cards SET orphaned_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			 WHERE id = ? AND orphaned_at IS NULL`, id); err != nil {
			return err
		}
	}
	return nil
}

// SyncOpenQuestions reconciles a note's open questions with the parsed
// list: new texts are inserted as 'open'; stored 'open' questions no longer
// present in the file are marked 'dropped'.
func (s *Store) SyncOpenQuestions(notePath string, questions []string) error {
	existing := map[string]bool{}
	rows, err := s.DB.Query(`SELECT text FROM open_questions WHERE note_path = ?`, notePath)
	if err != nil {
		return err
	}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return err
		}
		existing[t] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	current := map[string]bool{}
	for _, q := range questions {
		current[q] = true
		if !existing[q] {
			if _, err := s.DB.Exec(
				`INSERT INTO open_questions (note_path, text) VALUES (?, ?)`,
				notePath, q); err != nil {
				return err
			}
		}
	}
	for text := range existing {
		if !current[text] {
			if _, err := s.DB.Exec(
				`UPDATE open_questions SET status = 'dropped'
				 WHERE note_path = ? AND text = ? AND status = 'open'`,
				notePath, text); err != nil {
				return err
			}
		}
	}
	return nil
}

// LogEvent appends one row to the universal activity log and returns its
// id (so diagnoses like error-log entries can point back at the evidence).
// sessionID 0 means "no active session". payload is marshalled to JSON.
func (s *Store) LogEvent(kind, ref string, elapsedMs int64, sessionID int64, payload any) (int64, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	if payload == nil {
		data = []byte("{}")
	}
	var sid sql.NullInt64
	if sessionID > 0 {
		sid = sql.NullInt64{Int64: sessionID, Valid: true}
	}
	var ms sql.NullInt64
	if elapsedMs > 0 {
		ms = sql.NullInt64{Int64: elapsedMs, Valid: true}
	}
	res, err := s.DB.Exec(
		`INSERT INTO activity_events (kind, ref, elapsed_ms, session_id, payload)
		 VALUES (?, ?, ?, ?, ?)`,
		kind, nullIfEmpty(ref), ms, sid, string(data))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) queryStrings(query string, args ...any) ([]string, error) {
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
