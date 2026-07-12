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
// or title (all case-insensitive). Targets are normalized first: a
// "#Heading" fragment and a leading "/" are stripped. A target containing a
// slash ("concepts/Foo") also matches any note whose path ends in that
// fragment, so Obsidian-style partial paths resolve from any folder.
// Relative targets ("../DL/Foo", "./Foo") are anchored at the linking
// note's folder, so the same target can resolve differently per note.
func (s *Store) ResolveNoteLinks() error {
	rows, err := s.DB.Query(`SELECT path, title FROM notes ORDER BY path`)
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
	var paths, titles, lowerNoExt []string
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
		lowerNoExt = append(lowerNoExt, strings.ToLower(noExt))
		add(p, p)
		add(noExt, p)
		add(path.Base(noExt), p)
	}
	for i, t := range titles {
		add(t, paths[i])
	}

	resolve := func(fromPath, target string) (string, bool) {
		key := strings.ToLower(strings.TrimSpace(target))
		// "[[note#Heading]]" links to the note; the heading is not
		// addressable here, so it only decorates the link text.
		if i := strings.Index(key, "#"); i >= 0 {
			key = strings.TrimSpace(key[:i])
		}
		key = strings.TrimPrefix(key, "/")
		if key == "" {
			return "", false
		}
		if to, ok := byName[key]; ok {
			return to, true
		}
		// "../DL/Foo" / "./Foo" resolve from the linking note's folder.
		// A join that still escapes the vault root can't match anything.
		if strings.HasPrefix(key, "../") || strings.HasPrefix(key, "./") {
			rel := path.Join(strings.ToLower(path.Dir(fromPath)), key)
			if !strings.HasPrefix(rel, "../") {
				if to, ok := byName[rel]; ok {
					return to, true
				}
			}
		}
		// Partial paths: "concepts/Foo" written in some other folder
		// matches ".../concepts/Foo.md"; paths are sorted, so the first
		// suffix hit is stable across runs.
		if strings.Contains(key, "/") {
			for i, ln := range lowerNoExt {
				if strings.HasSuffix(ln, "/"+key) {
					return paths[i], true
				}
			}
		}
		return "", false
	}

	// Relative targets depend on the linking note, so resolution is per
	// (from_path, target) pair — the table's primary key.
	linkRows, err := s.DB.Query(`SELECT from_path, target FROM note_links`)
	if err != nil {
		return err
	}
	type link struct{ from, target string }
	var links []link
	for linkRows.Next() {
		var l link
		if err := linkRows.Scan(&l.from, &l.target); err != nil {
			linkRows.Close()
			return err
		}
		links = append(links, l)
	}
	linkRows.Close()
	if err := linkRows.Err(); err != nil {
		return err
	}

	for _, l := range links {
		to, ok := resolve(l.from, l.target)
		var val any
		if ok {
			val = to
		}
		if _, err := s.DB.Exec(
			`UPDATE note_links SET to_path = ? WHERE from_path = ? AND target = ?`,
			val, l.from, l.target); err != nil {
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
