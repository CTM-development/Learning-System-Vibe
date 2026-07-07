package store

import (
	"path"
	"strings"
)

// NoteLink is one outgoing [[wikilink]] from a note. ToPath is empty while
// the target has no matching note (a "red link").
type NoteLink struct {
	Target string `json:"target"`
	ToPath string `json:"to_path"`
}

// NoteRef points at a note (used for backlinks).
type NoteRef struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

// SetNoteLinks replaces a note's outgoing links. Resolution happens later
// in ResolveNoteLinks, once all notes of a sync run are upserted.
func (s *Store) SetNoteLinks(fromPath string, targets []string) error {
	if _, err := s.DB.Exec(`DELETE FROM note_links WHERE from_path = ?`, fromPath); err != nil {
		return err
	}
	for _, t := range targets {
		if _, err := s.DB.Exec(
			`INSERT OR IGNORE INTO note_links (from_path, target) VALUES (?, ?)`,
			fromPath, t); err != nil {
			return err
		}
	}
	return nil
}

// ResolveNoteLinks (re)resolves every link target against current notes.
// A target matches a note by exact path, path without extension, file stem
// or title (all case-insensitive).
func (s *Store) ResolveNoteLinks() error {
	rows, err := s.DB.Query(`SELECT path, title FROM notes`)
	if err != nil {
		return err
	}
	byName := map[string]string{}
	// Later (alphabetically) notes must not displace earlier exact matches;
	// collect in order and keep the first.
	add := func(key, notePath string) {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			return
		}
		if _, taken := byName[key]; !taken {
			byName[key] = notePath
		}
	}
	var paths, titles []string
	for rows.Next() {
		var p, t string
		if err := rows.Scan(&p, &t); err != nil {
			rows.Close()
			return err
		}
		paths = append(paths, p)
		titles = append(titles, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range paths {
		noExt := strings.TrimSuffix(p, path.Ext(p))
		add(p, p)
		add(noExt, p)
		add(path.Base(noExt), p)
	}
	for i, t := range titles {
		add(t, paths[i])
	}

	linkRows, err := s.DB.Query(`SELECT DISTINCT target FROM note_links`)
	if err != nil {
		return err
	}
	var targets []string
	for linkRows.Next() {
		var t string
		if err := linkRows.Scan(&t); err != nil {
			linkRows.Close()
			return err
		}
		targets = append(targets, t)
	}
	linkRows.Close()
	if err := linkRows.Err(); err != nil {
		return err
	}

	for _, t := range targets {
		to, ok := byName[strings.ToLower(strings.TrimSpace(t))]
		var val any
		if ok {
			val = to
		}
		if _, err := s.DB.Exec(
			`UPDATE note_links SET to_path = ? WHERE target = ?`, val, t); err != nil {
			return err
		}
	}
	return nil
}

// NoteLinks returns a note's outgoing links, resolved first.
func (s *Store) NoteLinks(fromPath string) ([]NoteLink, error) {
	rows, err := s.DB.Query(`
		SELECT target, COALESCE(to_path, '') FROM note_links
		WHERE from_path = ? ORDER BY to_path IS NULL, target`, fromPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NoteLink{}
	for rows.Next() {
		var l NoteLink
		if err := rows.Scan(&l.Target, &l.ToPath); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Backlinks returns notes that link to the given note.
func (s *Store) Backlinks(toPath string) ([]NoteRef, error) {
	rows, err := s.DB.Query(`
		SELECT DISTINCT n.path, n.title
		FROM note_links l JOIN notes n ON n.path = l.from_path
		WHERE l.to_path = ? ORDER BY n.title`, toPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NoteRef{}
	for rows.Next() {
		var r NoteRef
		if err := rows.Scan(&r.Path, &r.Title); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
