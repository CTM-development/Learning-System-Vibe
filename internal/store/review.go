package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/CTM-development/learning-system-vibe/internal/srs"
)

// QueueCard is a card as presented in the review queue.
type QueueCard struct {
	ID       string `json:"id"`
	NotePath string `json:"note_path"`
	Type     string `json:"type"`
	Front    string `json:"front"`
	Back     string `json:"back"`
	Deck     string `json:"deck"`
	Due      string `json:"due"`
	State    int    `json:"state"`
}

const queueCardQuery = `
	SELECT c.id, c.note_path, c.type, c.front, c.back, c.deck, cs.due, cs.state
	FROM cards c
	JOIN card_schedule cs ON cs.card_id = c.id
	WHERE c.orphaned_at IS NULL AND c.suspended = 0`

func (s *Store) queryQueueCards(query string, args ...any) ([]QueueCard, error) {
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []QueueCard{}
	for rows.Next() {
		var c QueueCard
		if err := rows.Scan(&c.ID, &c.NotePath, &c.Type, &c.Front, &c.Back, &c.Deck, &c.Due, &c.State); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DueCards returns non-new cards due at or before now, oldest due first.
func (s *Store) DueCards(now time.Time) ([]QueueCard, error) {
	return s.queryQueueCards(
		queueCardQuery+` AND cs.state != 0 AND cs.due <= ? ORDER BY cs.due`,
		now.UTC().Format(time.RFC3339))
}

// NewCards returns up to limit never-reviewed cards, oldest first.
func (s *Store) NewCards(limit int) ([]QueueCard, error) {
	if limit <= 0 {
		return []QueueCard{}, nil
	}
	return s.queryQueueCards(
		queueCardQuery+` AND cs.state = 0 ORDER BY c.created_at, c.id LIMIT ?`, limit)
}

// CountNewIntroducedToday counts reviews of previously-new cards since local
// midnight — the basis for the new-cards/day limit.
func (s *Store) CountNewIntroducedToday() (int, error) {
	var n int
	err := s.DB.QueryRow(`
		SELECT COUNT(*) FROM activity_events
		WHERE kind = 'card_review'
		  AND json_extract(payload, '$.before.state') = 0
		  AND date(ts, 'localtime') = date('now', 'localtime')`).Scan(&n)
	return n, err
}

// GetSchedule loads a card's current FSRS state.
func (s *Store) GetSchedule(cardID string) (srs.Schedule, error) {
	var sc srs.Schedule
	var due string
	var lastReview sql.NullString
	err := s.DB.QueryRow(`
		SELECT card_id, due, stability, difficulty, elapsed_days, scheduled_days,
		       reps, lapses, state, last_review
		FROM card_schedule WHERE card_id = ?`, cardID).
		Scan(&sc.CardID, &due, &sc.Stability, &sc.Difficulty, &sc.ElapsedDays,
			&sc.ScheduledDays, &sc.Reps, &sc.Lapses, &sc.State, &lastReview)
	if errors.Is(err, sql.ErrNoRows) {
		return sc, fmt.Errorf("schedule for card %s: %w", cardID, ErrNotFound)
	}
	if err != nil {
		return sc, err
	}
	if sc.Due, err = time.Parse(time.RFC3339, due); err != nil {
		return sc, fmt.Errorf("parse due %q: %w", due, err)
	}
	if lastReview.Valid {
		if sc.LastReview, err = time.Parse(time.RFC3339, lastReview.String); err != nil {
			return sc, fmt.Errorf("parse last_review %q: %w", lastReview.String, err)
		}
	}
	return sc, nil
}

// UpdateSchedule persists a card's FSRS state after a review.
func (s *Store) UpdateSchedule(sc srs.Schedule) error {
	_, err := s.DB.Exec(`
		UPDATE card_schedule SET due = ?, stability = ?, difficulty = ?,
			elapsed_days = ?, scheduled_days = ?, reps = ?, lapses = ?,
			state = ?, last_review = ?
		WHERE card_id = ?`,
		sc.Due.UTC().Format(time.RFC3339), sc.Stability, sc.Difficulty,
		sc.ElapsedDays, sc.ScheduledDays, sc.Reps, sc.Lapses, sc.State,
		sc.LastReview.UTC().Format(time.RFC3339), sc.CardID)
	return err
}

// Session mirrors one sessions row.
type Session struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at,omitempty"`
	Note      string `json:"note"`
}

// StartSession ends any active session, then starts a new one.
func (s *Store) StartSession(kind, note string) (Session, error) {
	if kind != "productivity" && kind != "learning" {
		return Session{}, fmt.Errorf("invalid session kind %q", kind)
	}
	if _, err := s.StopSession(); err != nil {
		return Session{}, err
	}
	res, err := s.DB.Exec(`INSERT INTO sessions (kind, note) VALUES (?, ?)`, kind, note)
	if err != nil {
		return Session{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Session{}, err
	}
	return s.getSession(id)
}

// StopSession ends the active session if any; returns it (zero Session when
// none was active).
func (s *Store) StopSession() (Session, error) {
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM sessions WHERE ended_at IS NULL ORDER BY id DESC LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, nil
	}
	if err != nil {
		return Session{}, err
	}
	if _, err := s.DB.Exec(
		`UPDATE sessions SET ended_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?`, id); err != nil {
		return Session{}, err
	}
	return s.getSession(id)
}

// ActiveSessionID returns the running session's id, or 0.
func (s *Store) ActiveSessionID() int64 {
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM sessions WHERE ended_at IS NULL ORDER BY id DESC LIMIT 1`).Scan(&id)
	if err != nil {
		return 0
	}
	return id
}

func (s *Store) getSession(id int64) (Session, error) {
	var sess Session
	var ended sql.NullString
	err := s.DB.QueryRow(
		`SELECT id, kind, started_at, ended_at, note FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.Kind, &sess.StartedAt, &ended, &sess.Note)
	sess.EndedAt = ended.String
	return sess, err
}

// ListSessions returns the most recent sessions, active first.
func (s *Store) ListSessions(limit int) ([]Session, error) {
	rows, err := s.DB.Query(
		`SELECT id, kind, started_at, ended_at, note FROM sessions ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var sess Session
		var ended sql.NullString
		if err := rows.Scan(&sess.ID, &sess.Kind, &sess.StartedAt, &ended, &sess.Note); err != nil {
			return nil, err
		}
		sess.EndedAt = ended.String
		out = append(out, sess)
	}
	return out, rows.Err()
}
