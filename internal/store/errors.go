package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RootCauses is the fixed error taxonomy (plan phase 3). Order matters:
// the UI shows them in this order.
var RootCauses = []string{
	"memory", "concept", "manipulation", "classification",
	"prerequisite", "overconfidence", "careless", "source",
}

func validRootCause(c string) bool {
	for _, v := range RootCauses {
		if v == c {
			return true
		}
	}
	return false
}

// ErrorEntry is one diagnosed failure, joined with its card for display.
type ErrorEntry struct {
	ID             int64  `json:"id"`
	EventID        int64  `json:"event_id"`
	CardID         string `json:"card_id"`
	CardFront      string `json:"card_front"`
	Deck           string `json:"deck"`
	NotePath       string `json:"note_path"`
	RootCause      string `json:"root_cause"`
	Note           string `json:"note"`
	RepairAction   string `json:"repair_action"`
	RepairNotePath string `json:"repair_note_path"`
	RepairDue      string `json:"repair_due"`
	CreatedAt      string `json:"created_at"`
	ResolvedAt     string `json:"resolved_at"`
}

// ErrorPatch carries optional field updates for an entry (nil = unchanged).
type ErrorPatch struct {
	RootCause      *string
	Note           *string
	RepairAction   *string
	RepairNotePath *string
	RepairDue      *string
	Resolved       *bool
}

// CreateError attaches a diagnosis to a failure event. The event must
// exist and not be diagnosed yet; its ref is denormalized into card_id.
func (s *Store) CreateError(eventID int64, rootCause, note, repairAction, repairNotePath, repairDue string) (ErrorEntry, error) {
	if !validRootCause(rootCause) {
		return ErrorEntry{}, fmt.Errorf("invalid root cause %q (want one of %s)",
			rootCause, strings.Join(RootCauses, ", "))
	}
	var ref sql.NullString
	err := s.DB.QueryRow(`SELECT ref FROM activity_events WHERE id = ?`, eventID).Scan(&ref)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrorEntry{}, fmt.Errorf("event %d: %w", eventID, ErrNotFound)
	}
	if err != nil {
		return ErrorEntry{}, err
	}

	res, err := s.DB.Exec(`
		INSERT INTO error_log (event_id, card_id, root_cause, note, repair_action, repair_note_path, repair_due)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		eventID, nullIfEmpty(ref.String), rootCause, note, repairAction,
		nullIfEmpty(repairNotePath), nullIfEmpty(repairDue))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrorEntry{}, fmt.Errorf("event %d is already diagnosed", eventID)
		}
		return ErrorEntry{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ErrorEntry{}, err
	}
	return s.GetError(id)
}

const errorEntryQuery = `
	SELECT el.id, el.event_id, COALESCE(el.card_id, ''),
	       COALESCE(c.front, ''), COALESCE(c.deck, ''), COALESCE(c.note_path, ''),
	       el.root_cause, el.note, el.repair_action,
	       COALESCE(el.repair_note_path, ''), COALESCE(el.repair_due, ''),
	       el.created_at, COALESCE(el.resolved_at, '')
	FROM error_log el
	LEFT JOIN cards c ON c.id = el.card_id`

func scanErrorEntry(scan func(dest ...any) error) (ErrorEntry, error) {
	var e ErrorEntry
	err := scan(&e.ID, &e.EventID, &e.CardID, &e.CardFront, &e.Deck, &e.NotePath,
		&e.RootCause, &e.Note, &e.RepairAction, &e.RepairNotePath, &e.RepairDue,
		&e.CreatedAt, &e.ResolvedAt)
	return e, err
}

// GetError returns one entry.
func (s *Store) GetError(id int64) (ErrorEntry, error) {
	row := s.DB.QueryRow(errorEntryQuery+` WHERE el.id = ?`, id)
	e, err := scanErrorEntry(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return e, fmt.Errorf("error entry %d: %w", id, ErrNotFound)
	}
	return e, err
}

// ListErrors returns entries filtered by status ("open", "resolved", "",
// "all") and root cause (""), newest first.
func (s *Store) ListErrors(status, cause string, limit int) ([]ErrorEntry, error) {
	query := errorEntryQuery + ` WHERE 1=1`
	var args []any
	switch status {
	case "", "open":
		query += ` AND el.resolved_at IS NULL`
	case "resolved":
		query += ` AND el.resolved_at IS NOT NULL`
	case "all":
	default:
		return nil, fmt.Errorf("invalid status %q", status)
	}
	if cause != "" {
		query += ` AND el.root_cause = ?`
		args = append(args, cause)
	}
	query += ` ORDER BY el.id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ErrorEntry{}
	for rows.Next() {
		e, err := scanErrorEntry(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpdateError patches an entry: repair fields, note, root cause, and the
// resolved flag.
func (s *Store) UpdateError(id int64, p ErrorPatch) (ErrorEntry, error) {
	sets := []string{}
	var args []any
	if p.RootCause != nil {
		if !validRootCause(*p.RootCause) {
			return ErrorEntry{}, fmt.Errorf("invalid root cause %q", *p.RootCause)
		}
		sets = append(sets, "root_cause = ?")
		args = append(args, *p.RootCause)
	}
	if p.Note != nil {
		sets = append(sets, "note = ?")
		args = append(args, *p.Note)
	}
	if p.RepairAction != nil {
		sets = append(sets, "repair_action = ?")
		args = append(args, *p.RepairAction)
	}
	if p.RepairNotePath != nil {
		sets = append(sets, "repair_note_path = ?")
		args = append(args, nullIfEmpty(*p.RepairNotePath))
	}
	if p.RepairDue != nil {
		sets = append(sets, "repair_due = ?")
		args = append(args, nullIfEmpty(*p.RepairDue))
	}
	if p.Resolved != nil {
		if *p.Resolved {
			sets = append(sets, "resolved_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')")
		} else {
			sets = append(sets, "resolved_at = NULL")
		}
	}
	if len(sets) == 0 {
		return s.GetError(id)
	}
	args = append(args, id)
	res, err := s.DB.Exec(`UPDATE error_log SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return ErrorEntry{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return ErrorEntry{}, err
	}
	if n == 0 {
		return ErrorEntry{}, fmt.Errorf("error entry %d: %w", id, ErrNotFound)
	}
	return s.GetError(id)
}

// TriageItem is one undiagnosed failure event awaiting classification.
type TriageItem struct {
	EventID   int64  `json:"event_id"`
	Ts        string `json:"ts"`
	Kind      string `json:"kind"` // card_review | free_text_answer
	CardID    string `json:"card_id"`
	CardFront string `json:"card_front"`
	CardBack  string `json:"card_back"`
	Deck      string `json:"deck"`
	Answer    string `json:"answer,omitempty"`  // free-text attempts
	Verdict   string `json:"verdict,omitempty"` // free-text attempts
}

// ErrorTriage lists recent failures without a diagnosis: reviews rated
// Again (not undone) and free-text answers graded incorrect.
func (s *Store) ErrorTriage(days, limit int) ([]TriageItem, error) {
	rows, err := s.DB.Query(`
		SELECT e.id, e.ts, e.kind, e.ref, c.front, c.back, c.deck,
		       COALESCE(json_extract(e.payload, '$.answer'), ''),
		       COALESCE(json_extract(e.payload, '$.verdict'), '')
		FROM activity_events e
		JOIN cards c ON c.id = e.ref
		LEFT JOIN error_log el ON el.event_id = e.id
		WHERE el.id IS NULL
		  AND e.ts >= datetime('now', ?)
		  AND ((e.kind = 'card_review'
		        AND json_extract(e.payload, '$.rating') = 1
		        AND e.id NOT IN (`+undoneReviewIDs+`))
		    OR (e.kind = 'free_text_answer'
		        AND json_extract(e.payload, '$.verdict') = 'incorrect'))
		ORDER BY e.id DESC LIMIT ?`,
		fmt.Sprintf("-%d days", days), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TriageItem{}
	for rows.Next() {
		var it TriageItem
		if err := rows.Scan(&it.EventID, &it.Ts, &it.Kind, &it.CardID,
			&it.CardFront, &it.CardBack, &it.Deck, &it.Answer, &it.Verdict); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// CauseCount is one row of the diagnosis breakdown.
type CauseCount struct {
	Cause string `json:"cause"`
	Deck  string `json:"deck,omitempty"`
	Open  int    `json:"open"`
	Total int    `json:"total"`
}

// ErrorStats returns diagnosis counts by root cause and by cause × deck.
func (s *Store) ErrorStats() (byCause []CauseCount, byDeck []CauseCount, err error) {
	byCause, err = s.queryCauseCounts(`
		SELECT root_cause, '' AS deck,
		       SUM(CASE WHEN resolved_at IS NULL THEN 1 ELSE 0 END), COUNT(*)
		FROM error_log GROUP BY root_cause ORDER BY 4 DESC`)
	if err != nil {
		return nil, nil, err
	}
	byDeck, err = s.queryCauseCounts(`
		SELECT el.root_cause, COALESCE(NULLIF(c.deck, ''), '(root)'),
		       SUM(CASE WHEN el.resolved_at IS NULL THEN 1 ELSE 0 END), COUNT(*)
		FROM error_log el
		LEFT JOIN cards c ON c.id = el.card_id
		GROUP BY 1, 2 ORDER BY 4 DESC`)
	return byCause, byDeck, err
}

func (s *Store) queryCauseCounts(query string) ([]CauseCount, error) {
	rows, err := s.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CauseCount{}
	for rows.Next() {
		var c CauseCount
		if err := rows.Scan(&c.Cause, &c.Deck, &c.Open, &c.Total); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DueRepairs returns open entries whose repair is due today or overdue.
func (s *Store) DueRepairs(now time.Time) ([]ErrorEntry, error) {
	rows, err := s.DB.Query(errorEntryQuery+`
		WHERE el.resolved_at IS NULL AND el.repair_due IS NOT NULL
		  AND el.repair_due <= ?
		ORDER BY el.repair_due, el.id`, now.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ErrorEntry{}
	for rows.Next() {
		e, err := scanErrorEntry(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountOpenErrors counts unresolved diagnoses.
func (s *Store) CountOpenErrors() (int, error) {
	var n int
	err := s.DB.QueryRow(`SELECT COUNT(*) FROM error_log WHERE resolved_at IS NULL`).Scan(&n)
	return n, err
}
