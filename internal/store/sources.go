package store

import (
	"database/sql"
	"errors"
	"fmt"
	"html"
	"strings"
)

// SourceRow mirrors one sources row.
type SourceRow struct {
	ID      int64  `json:"id"`
	Kind    string `json:"kind"`
	Key     string `json:"key"`
	Path    string `json:"path"` // relative to attachments_dir
	Title   string `json:"title"`
	Meta    string `json:"meta"`
	AddedAt string `json:"added_at"`
}

// CreateSource inserts a source row.
func (s *Store) CreateSource(kind, key, path, title string) (SourceRow, error) {
	res, err := s.DB.Exec(
		`INSERT INTO sources (kind, key, path, title) VALUES (?, ?, ?, ?)`,
		kind, key, path, title)
	if err != nil {
		return SourceRow{}, fmt.Errorf("create source %s: %w", key, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return SourceRow{}, err
	}
	return s.GetSource(id)
}

// GetSource returns one source by id.
func (s *Store) GetSource(id int64) (SourceRow, error) {
	var r SourceRow
	err := s.DB.QueryRow(
		`SELECT id, kind, key, path, title, meta, added_at FROM sources WHERE id = ?`, id).
		Scan(&r.ID, &r.Kind, &r.Key, &r.Path, &r.Title, &r.Meta, &r.AddedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r, fmt.Errorf("source %d: %w", id, ErrNotFound)
	}
	return r, err
}

// ListSources returns all sources, newest first.
func (s *Store) ListSources() ([]SourceRow, error) {
	rows, err := s.DB.Query(
		`SELECT id, kind, key, path, title, meta, added_at FROM sources ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SourceRow{}
	for rows.Next() {
		var r SourceRow
		if err := rows.Scan(&r.ID, &r.Kind, &r.Key, &r.Path, &r.Title, &r.Meta, &r.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SourceKeyExists reports whether a citation key is taken.
func (s *Store) SourceKeyExists(key string) (bool, error) {
	var exists bool
	err := s.DB.QueryRow(`SELECT EXISTS (SELECT 1 FROM sources WHERE key = ?)`, key).Scan(&exists)
	return exists, err
}

// IndexSourceText (re)writes a source's full-text index entry.
func (s *Store) IndexSourceText(id int64, title, content string) error {
	if _, err := s.DB.Exec(`DELETE FROM sources_fts WHERE source_id = ?`, id); err != nil {
		return err
	}
	_, err := s.DB.Exec(
		`INSERT INTO sources_fts (source_id, title, content) VALUES (?, ?, ?)`,
		id, title, content)
	return err
}

// SourceSearchHit is one FTS match over extracted source text.
type SourceSearchHit struct {
	SourceID int64  `json:"source_id"`
	Title    string `json:"title"`
	Snippet  string `json:"snippet"`
}

// SearchSources runs full-text search over source titles and extracted
// text, mirroring SearchNotes' quoting and escaping.
func (s *Store) SearchSources(q string, limit int) ([]SourceSearchHit, error) {
	match := ftsMatchQuery(q)
	if match == "" {
		return []SourceSearchHit{}, nil
	}
	rows, err := s.DB.Query(`
		SELECT source_id, title, snippet(sources_fts, 2, char(1), char(2), '…', 14)
		FROM sources_fts WHERE sources_fts MATCH ? ORDER BY rank LIMIT ?`,
		match, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SourceSearchHit{}
	for rows.Next() {
		var h SourceSearchHit
		if err := rows.Scan(&h.SourceID, &h.Title, &h.Snippet); err != nil {
			return nil, err
		}
		h.Snippet = escapeSnippet(h.Snippet)
		out = append(out, h)
	}
	return out, rows.Err()
}

// ftsMatchQuery quotes each user term (with a prefix star) so FTS5
// operator syntax cannot break or subvert the query. Empty when the query
// has no terms.
func ftsMatchQuery(q string) string {
	terms := strings.Fields(q)
	for i, t := range terms {
		terms[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"*`
	}
	return strings.Join(terms, " ")
}

// escapeSnippet HTML-escapes a snippet built with char(1)/char(2)
// sentinels, then converts the sentinels to <mark> tags.
func escapeSnippet(s string) string {
	s = html.EscapeString(s)
	s = strings.ReplaceAll(s, "\x01", "<mark>")
	return strings.ReplaceAll(s, "\x02", "</mark>")
}
