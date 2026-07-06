package store

import (
	"fmt"
	"time"
)

// DayCount is one day's count for heatmap/forecast series.
type DayCount struct {
	Date  string `json:"date"` // YYYY-MM-DD, local time
	Count int    `json:"count"`
}

// ReviewHeatmap returns reviews per local day over the past `days` days
// (days with zero reviews are absent).
func (s *Store) ReviewHeatmap(days int) ([]DayCount, error) {
	return s.queryDayCounts(`
		SELECT date(ts, 'localtime') AS d, COUNT(*)
		FROM activity_events
		WHERE kind = 'card_review' AND ts >= datetime('now', ?)
		GROUP BY d ORDER BY d`,
		fmt.Sprintf("-%d days", days))
}

// DueForecast returns, per local day from today through today+days-1, how
// many cards fall due, plus the count already overdue before today.
func (s *Store) DueForecast(days int) (forecast []DayCount, overdue int, err error) {
	forecast, err = s.queryDayCounts(`
		SELECT date(cs.due, 'localtime') AS d, COUNT(*)
		FROM card_schedule cs
		JOIN cards c ON c.id = cs.card_id
		WHERE c.orphaned_at IS NULL AND c.suspended = 0 AND cs.state != 0
		  AND date(cs.due, 'localtime') >= date('now', 'localtime')
		  AND date(cs.due, 'localtime') < date('now', 'localtime', ?)
		GROUP BY d ORDER BY d`,
		fmt.Sprintf("+%d days", days))
	if err != nil {
		return nil, 0, err
	}
	err = s.DB.QueryRow(`
		SELECT COUNT(*)
		FROM card_schedule cs
		JOIN cards c ON c.id = cs.card_id
		WHERE c.orphaned_at IS NULL AND c.suspended = 0 AND cs.state != 0
		  AND date(cs.due, 'localtime') < date('now', 'localtime')`).Scan(&overdue)
	return forecast, overdue, err
}

func (s *Store) queryDayCounts(query string, args ...any) ([]DayCount, error) {
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DayCount{}
	for rows.Next() {
		var d DayCount
		if err := rows.Scan(&d.Date, &d.Count); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// StatsSummary is the KPI-row payload.
type StatsSummary struct {
	TotalCards     int     `json:"total_cards"`
	SuspendedCards int     `json:"suspended_cards"`
	OrphanedCards  int     `json:"orphaned_cards"`
	DueNow         int     `json:"due_now"`
	NewRemaining   int     `json:"new_remaining"`
	ReviewsToday   int     `json:"reviews_today"`
	TimeTodayMs    int64   `json:"time_today_ms"`
	Retention30    float64 `json:"retention_30"` // -1 when no review-state reviews in window
	AvgReviewMs    int64   `json:"avg_review_ms"`
}

// Summary computes the dashboard KPIs. newPerDay is the configured daily
// new-card limit.
func (s *Store) Summary(newPerDay int, now time.Time) (StatsSummary, error) {
	var sum StatsSummary
	err := s.DB.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN suspended = 1 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN orphaned_at IS NOT NULL THEN 1 ELSE 0 END), 0)
		FROM cards`).Scan(&sum.TotalCards, &sum.SuspendedCards, &sum.OrphanedCards)
	if err != nil {
		return sum, err
	}

	err = s.DB.QueryRow(`
		SELECT COUNT(*)
		FROM card_schedule cs
		JOIN cards c ON c.id = cs.card_id
		WHERE c.orphaned_at IS NULL AND c.suspended = 0 AND cs.state != 0 AND cs.due <= ?`,
		now.UTC().Format(time.RFC3339)).Scan(&sum.DueNow)
	if err != nil {
		return sum, err
	}

	introduced, err := s.CountNewIntroducedToday()
	if err != nil {
		return sum, err
	}
	sum.NewRemaining = max(newPerDay-introduced, 0)

	// Time today spans all timed activity, not just reviews.
	err = s.DB.QueryRow(`
		SELECT COALESCE(SUM(CASE WHEN kind = 'card_review' THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(elapsed_ms), 0)
		FROM activity_events
		WHERE date(ts, 'localtime') = date('now', 'localtime')`).
		Scan(&sum.ReviewsToday, &sum.TimeTodayMs)
	if err != nil {
		return sum, err
	}

	// True retention: pass rate on cards that were in review state (2).
	var passed, total int
	err = s.DB.QueryRow(`
		SELECT COALESCE(SUM(CASE WHEN json_extract(payload, '$.rating') > 1 THEN 1 ELSE 0 END), 0),
		       COUNT(*)
		FROM activity_events
		WHERE kind = 'card_review'
		  AND json_extract(payload, '$.before.state') = 2
		  AND ts >= datetime('now', '-30 days')`).Scan(&passed, &total)
	if err != nil {
		return sum, err
	}
	if total == 0 {
		sum.Retention30 = -1
	} else {
		sum.Retention30 = float64(passed) / float64(total)
	}

	err = s.DB.QueryRow(`
		SELECT CAST(COALESCE(AVG(elapsed_ms), 0) AS INTEGER)
		FROM activity_events
		WHERE kind = 'card_review' AND elapsed_ms IS NOT NULL
		  AND ts >= datetime('now', '-30 days')`).Scan(&sum.AvgReviewMs)
	return sum, err
}

// TimeBucket is time spent per group (activity kind or deck).
type TimeBucket struct {
	Key   string `json:"key"`
	Ms    int64  `json:"ms"`
	Count int    `json:"count"`
}

// TimeByKind sums elapsed time per activity kind over the past `days` days.
func (s *Store) TimeByKind(days int) ([]TimeBucket, error) {
	return s.queryTimeBuckets(`
		SELECT kind, COALESCE(SUM(elapsed_ms), 0), COUNT(*)
		FROM activity_events
		WHERE ts >= datetime('now', ?) AND elapsed_ms IS NOT NULL
		GROUP BY kind ORDER BY 2 DESC`,
		fmt.Sprintf("-%d days", days))
}

// TimeByDeck sums card-review time per deck over the past `days` days.
func (s *Store) TimeByDeck(days int) ([]TimeBucket, error) {
	return s.queryTimeBuckets(`
		SELECT COALESCE(NULLIF(c.deck, ''), '(root)'), COALESCE(SUM(e.elapsed_ms), 0), COUNT(*)
		FROM activity_events e
		JOIN cards c ON c.id = e.ref
		WHERE e.kind = 'card_review' AND e.ts >= datetime('now', ?)
		GROUP BY 1 ORDER BY 2 DESC`,
		fmt.Sprintf("-%d days", days))
}

func (s *Store) queryTimeBuckets(query string, args ...any) ([]TimeBucket, error) {
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TimeBucket{}
	for rows.Next() {
		var b TimeBucket
		if err := rows.Scan(&b.Key, &b.Ms, &b.Count); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
