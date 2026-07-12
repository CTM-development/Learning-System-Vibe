package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Project groups note directories ("decks") under a name, optionally with a
// deadline that later milestones use to pace review and new-card
// introduction.
type Project struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Deadline  string   `json:"deadline"` // "YYYY-MM-DD" or "" = none
	Dirs      []string `json:"dirs"`
	CreatedAt string   `json:"created_at"`
}

// validateProjectName trims and rejects an empty project name.
func validateProjectName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("project name must not be empty")
	}
	return name, nil
}

// validateProjectDeadline accepts "" (no deadline) or a YYYY-MM-DD date.
func validateProjectDeadline(deadline string) (string, error) {
	if deadline == "" {
		return "", nil
	}
	if _, err := time.Parse("2006-01-02", deadline); err != nil {
		return "", fmt.Errorf("invalid deadline %q (want YYYY-MM-DD): %w", deadline, err)
	}
	return deadline, nil
}

// validateProjectDirs trims slashes and whitespace off each dir, dedupes
// preserving order, and requires at least one entry ("" means the notes
// root).
func validateProjectDirs(dirs []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		d = strings.Trim(strings.TrimSpace(d), "/")
		if seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, errors.New(`at least one directory is required (use "" for the notes root)`)
	}
	return out, nil
}

// CreateProject validates and inserts a project with its directories in a
// single transaction.
func (s *Store) CreateProject(name, deadline string, dirs []string) (Project, error) {
	cleanName, err := validateProjectName(name)
	if err != nil {
		return Project{}, err
	}
	cleanDeadline, err := validateProjectDeadline(deadline)
	if err != nil {
		return Project{}, err
	}
	cleanDirs, err := validateProjectDirs(dirs)
	if err != nil {
		return Project{}, err
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO projects (name, deadline) VALUES (?, ?)`,
		cleanName, nullIfEmpty(cleanDeadline))
	if err != nil {
		return Project{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Project{}, err
	}
	for _, d := range cleanDirs {
		if _, err := tx.Exec(`INSERT INTO project_dirs (project_id, dir) VALUES (?, ?)`, id, d); err != nil {
			return Project{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Project{}, err
	}
	return s.GetProject(id)
}

// ListProjects returns every project ordered by name, each with its dirs.
func (s *Store) ListProjects() ([]Project, error) {
	rows, err := s.DB.Query(`SELECT id, name, deadline, created_at FROM projects ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var projects []Project
	for rows.Next() {
		var p Project
		var deadline sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &deadline, &p.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		p.Deadline = deadline.String
		p.Dirs = []string{}
		projects = append(projects, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	dirRows, err := s.DB.Query(`SELECT project_id, dir FROM project_dirs ORDER BY dir`)
	if err != nil {
		return nil, err
	}
	defer dirRows.Close()
	dirsByProject := map[int64][]string{}
	for dirRows.Next() {
		var pid int64
		var dir string
		if err := dirRows.Scan(&pid, &dir); err != nil {
			return nil, err
		}
		dirsByProject[pid] = append(dirsByProject[pid], dir)
	}
	if err := dirRows.Err(); err != nil {
		return nil, err
	}

	out := make([]Project, 0, len(projects))
	for _, p := range projects {
		if d, ok := dirsByProject[p.ID]; ok {
			p.Dirs = d
		}
		out = append(out, p)
	}
	return out, nil
}

// GetProject returns one project with its dirs, or ErrNotFound.
func (s *Store) GetProject(id int64) (Project, error) {
	var p Project
	var deadline sql.NullString
	err := s.DB.QueryRow(`SELECT id, name, deadline, created_at FROM projects WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &deadline, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, fmt.Errorf("project %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return p, err
	}
	p.Deadline = deadline.String

	dirs, err := s.queryStrings(`SELECT dir FROM project_dirs WHERE project_id = ? ORDER BY dir`, id)
	if err != nil {
		return p, err
	}
	if dirs == nil {
		dirs = []string{}
	}
	p.Dirs = dirs
	return p, nil
}

// UpdateProject patches a project: nil name/deadline leave the field
// unchanged, a nil dirs slice leaves the directories unchanged. A non-nil
// but empty deadline pointer clears the deadline. Returns ErrNotFound for
// an unknown id.
func (s *Store) UpdateProject(id int64, name, deadline *string, dirs []string) (Project, error) {
	if _, err := s.GetProject(id); err != nil {
		return Project{}, err
	}

	var cleanName, cleanDeadline string
	var cleanDirs []string
	var err error
	if name != nil {
		if cleanName, err = validateProjectName(*name); err != nil {
			return Project{}, err
		}
	}
	if deadline != nil {
		if cleanDeadline, err = validateProjectDeadline(*deadline); err != nil {
			return Project{}, err
		}
	}
	if dirs != nil {
		if cleanDirs, err = validateProjectDirs(dirs); err != nil {
			return Project{}, err
		}
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return Project{}, err
	}
	defer tx.Rollback()

	var sets []string
	var args []any
	if name != nil {
		sets = append(sets, "name = ?")
		args = append(args, cleanName)
	}
	if deadline != nil {
		sets = append(sets, "deadline = ?")
		args = append(args, nullIfEmpty(cleanDeadline))
	}
	if len(sets) > 0 {
		args = append(args, id)
		if _, err := tx.Exec(`UPDATE projects SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
			return Project{}, err
		}
	}
	if dirs != nil {
		if _, err := tx.Exec(`DELETE FROM project_dirs WHERE project_id = ?`, id); err != nil {
			return Project{}, err
		}
		for _, d := range cleanDirs {
			if _, err := tx.Exec(`INSERT INTO project_dirs (project_id, dir) VALUES (?, ?)`, id, d); err != nil {
				return Project{}, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return Project{}, err
	}
	return s.GetProject(id)
}

// DeleteProject removes a project (its dirs cascade). Returns ErrNotFound
// for an unknown id.
func (s *Store) DeleteProject(id int64) error {
	res, err := s.DB.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("project %d: %w", id, ErrNotFound)
	}
	return nil
}

// ProjectCardStats aggregates active-card counts over a project's dirs:
// total non-orphaned/non-suspended cards, how many are still new (state
// 0), and how many are due now (not buried). now is compared in UTC,
// matching the review queue's convention.
func (s *Store) ProjectCardStats(dirs []string, now time.Time) (total, newCount, dueNow int, err error) {
	if len(dirs) == 0 {
		return 0, 0, 0, nil
	}
	nowStr := now.UTC().Format(time.RFC3339)
	query := `
		SELECT COUNT(*),
		       COALESCE(SUM(cs.state = 0), 0),
		       COALESCE(SUM(cs.state != 0 AND cs.due <= ? AND (cs.buried_until IS NULL OR cs.buried_until <= ?)), 0)
		FROM cards c
		JOIN card_schedule cs ON cs.card_id = c.id
		WHERE c.orphaned_at IS NULL AND c.suspended = 0`
	where, dirArgs := decksWhere(dirs)
	args := append([]any{nowStr, nowStr}, dirArgs...)
	err = s.DB.QueryRow(query+where, args...).Scan(&total, &newCount, &dueNow)
	return total, newCount, dueNow, err
}
