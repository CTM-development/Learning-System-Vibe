package store

import "database/sql"

// EarliestActiveDeadline returns the earliest not-yet-passed project
// deadline covering the given deck (a project covers a deck when one of its
// dirs equals the deck or is an ancestor folder; the root dir "" covers only
// the root deck). today is a local YYYY-MM-DD date; the deadline day itself
// still counts as active. ok is false when no deadline applies — passed
// deadlines drop out here, which is what reverts scheduling to plain FSRS.
func (s *Store) EarliestActiveDeadline(deck, today string) (deadline string, ok bool, err error) {
	var dl sql.NullString
	err = s.DB.QueryRow(`
		SELECT MIN(p.deadline) FROM projects p
		JOIN project_dirs pd ON pd.project_id = p.id
		WHERE p.deadline IS NOT NULL AND p.deadline != ''
		  AND p.deadline >= ?
		  AND (pd.dir = ? OR ? LIKE pd.dir || '/%')`,
		today, deck, deck).Scan(&dl)
	if err != nil {
		return "", false, err
	}
	if !dl.Valid || dl.String == "" {
		return "", false, nil
	}
	return dl.String, true, nil
}
