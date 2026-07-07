package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// NoteSummary is the list-view projection of a note.
type NoteSummary struct {
	Path      string   `json:"path"`
	Title     string   `json:"title"`
	Stage     string   `json:"stage"`
	Status    string   `json:"status"`
	Type      string   `json:"type"` // "reading" | "thought"
	Tags      []string `json:"tags"`
	Sources   []string `json:"sources"`
	Mtime     int64    `json:"mtime"`
	CardCount int      `json:"card_count"`
}

// NoteDetail adds the raw markdown content plus the link graph around the
// note: outgoing wikilinks (resolved or red) and backlinks from other notes.
type NoteDetail struct {
	NoteSummary
	Content   string     `json:"content"`
	Links     []NoteLink `json:"links"`
	Backlinks []NoteRef  `json:"backlinks"`
}

// OpenQuestion is one row of the open-question queue.
type OpenQuestion struct {
	ID       int64  `json:"id"`
	NotePath string `json:"note_path"`
	Text     string `json:"text"`
	Status   string `json:"status"`
	Created  string `json:"created_at"`
}

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

const noteSummaryQuery = `
	SELECT n.path, n.title, COALESCE(n.stage,''), COALESCE(n.status,''), n.type,
	       n.tags, n.sources, n.mtime,
	       (SELECT COUNT(*) FROM cards c WHERE c.note_path = n.path AND c.orphaned_at IS NULL)
	FROM notes n`

func scanNoteSummary(scan func(dest ...any) error) (NoteSummary, error) {
	var s NoteSummary
	var tags, sources string
	err := scan(&s.Path, &s.Title, &s.Stage, &s.Status, &s.Type, &tags, &sources, &s.Mtime, &s.CardCount)
	if err != nil {
		return s, err
	}
	json.Unmarshal([]byte(tags), &s.Tags)
	json.Unmarshal([]byte(sources), &s.Sources)
	if s.Tags == nil {
		s.Tags = []string{}
	}
	if s.Sources == nil {
		s.Sources = []string{}
	}
	return s, nil
}

// ListNotes returns all notes, optionally filtered by stage and/or type
// (reading | thought), newest first.
func (s *Store) ListNotes(stage, noteType string) ([]NoteSummary, error) {
	query := noteSummaryQuery
	var where []string
	var args []any
	if stage != "" {
		where = append(where, `n.stage = ?`)
		args = append(args, stage)
	}
	if noteType != "" {
		where = append(where, `n.type = ?`)
		args = append(args, noteType)
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}
	query += ` ORDER BY n.mtime DESC`

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NoteSummary{}
	for rows.Next() {
		n, err := scanNoteSummary(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// GetNote returns one note with content.
func (s *Store) GetNote(path string) (NoteDetail, error) {
	var d NoteDetail
	row := s.DB.QueryRow(noteSummaryQuery+` WHERE n.path = ?`, path)
	n, err := scanNoteSummary(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return d, fmt.Errorf("note %s: %w", path, ErrNotFound)
	}
	if err != nil {
		return d, err
	}
	d.NoteSummary = n
	if err := s.DB.QueryRow(`SELECT content FROM notes WHERE path = ?`, path).Scan(&d.Content); err != nil {
		return d, err
	}
	if d.Links, err = s.NoteLinks(path); err != nil {
		return d, err
	}
	d.Backlinks, err = s.Backlinks(path)
	return d, err
}

// SetQuestionStatus updates one open question's lifecycle status.
func (s *Store) SetQuestionStatus(id int64, status string) error {
	switch status {
	case "open", "carded", "folded", "dropped":
	default:
		return fmt.Errorf("invalid question status %q", status)
	}
	res, err := s.DB.Exec(`UPDATE open_questions SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("question %d: %w", id, ErrNotFound)
	}
	return nil
}

// StaleNote is a note stuck in an early stage.
type StaleNote struct {
	Path     string `json:"path"`
	Title    string `json:"title"`
	Stage    string `json:"stage"`
	IdleDays int    `json:"idle_days"`
}

// StaleNotes returns skim/deep notes untouched for at least minIdleDays,
// stalest first.
func (s *Store) StaleNotes(minIdleDays, limit int) ([]StaleNote, error) {
	rows, err := s.DB.Query(`
		SELECT path, title, stage,
		       CAST((strftime('%s','now') - mtime) / 86400 AS INTEGER)
		FROM notes
		WHERE stage IN ('skim', 'deep')
		  AND mtime <= strftime('%s','now') - ? * 86400
		ORDER BY mtime LIMIT ?`, minIdleDays, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StaleNote{}
	for rows.Next() {
		var n StaleNote
		if err := rows.Scan(&n.Path, &n.Title, &n.Stage, &n.IdleDays); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// CountLeeches counts active cards at or past the leech lapse threshold.
func (s *Store) CountLeeches() (int, error) {
	var n int
	err := s.DB.QueryRow(`
		SELECT COUNT(*) FROM cards c
		JOIN card_schedule cs ON cs.card_id = c.id
		WHERE c.suspended = 0 AND c.orphaned_at IS NULL AND cs.lapses >= ?`,
		LeechLapses).Scan(&n)
	return n, err
}

// ListOpenQuestions returns questions, optionally filtered by status,
// newest first.
func (s *Store) ListOpenQuestions(status string) ([]OpenQuestion, error) {
	query := `SELECT id, note_path, text, status, created_at FROM open_questions`
	var args []any
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC, id DESC`

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OpenQuestion{}
	for rows.Next() {
		var q OpenQuestion
		if err := rows.Scan(&q.ID, &q.NotePath, &q.Text, &q.Status, &q.Created); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}
