package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// NoteSummary is the list-view projection of a note.
type NoteSummary struct {
	Path      string   `json:"path"`
	Title     string   `json:"title"`
	Stage     string   `json:"stage"`
	Status    string   `json:"status"`
	Tags      []string `json:"tags"`
	Sources   []string `json:"sources"`
	Mtime     int64    `json:"mtime"`
	CardCount int      `json:"card_count"`
}

// NoteDetail adds the raw markdown content.
type NoteDetail struct {
	NoteSummary
	Content string `json:"content"`
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
	SELECT n.path, n.title, COALESCE(n.stage,''), COALESCE(n.status,''),
	       n.tags, n.sources, n.mtime,
	       (SELECT COUNT(*) FROM cards c WHERE c.note_path = n.path AND c.orphaned_at IS NULL)
	FROM notes n`

func scanNoteSummary(scan func(dest ...any) error) (NoteSummary, error) {
	var s NoteSummary
	var tags, sources string
	err := scan(&s.Path, &s.Title, &s.Stage, &s.Status, &tags, &sources, &s.Mtime, &s.CardCount)
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

// ListNotes returns all notes, optionally filtered by stage, newest first.
func (s *Store) ListNotes(stage string) ([]NoteSummary, error) {
	query := noteSummaryQuery
	var args []any
	if stage != "" {
		query += ` WHERE n.stage = ?`
		args = append(args, stage)
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
	err = s.DB.QueryRow(`SELECT content FROM notes WHERE path = ?`, path).Scan(&d.Content)
	return d, err
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
