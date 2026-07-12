package api

import (
	"math"
	"time"
)

// Deadline math for projects. All calculations run in server-local time,
// matching the date('now','localtime') day boundaries used elsewhere.
//
// The contract for a project deadline is twofold:
//   - every card comes due at least once more before the deadline day
//     (interval cap, applied per review in handleReview);
//   - every still-new card is introduced before the deadline
//     (pacing quota, applied in handleQueue for project-scoped queues).
//
// The deadline day itself is reserved as a final pass: reviews landing on it
// are not capped further, and it still counts as a study day for pacing.

// parseLocalDate parses a YYYY-MM-DD string as local midnight.
func parseLocalDate(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", s, time.Local)
}

// startOfDay truncates t to local midnight.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// capDue squeezes an FSRS due date so the card surfaces at least once more
// before capTime (local midnight starting the deadline day). Reviews at or
// after capTime pass through uncapped — the card was just seen, which is the
// guarantee we want. The 0.9 factor staggers capped dues short of the
// deadline instead of piling them all onto the last day; within the final
// 24h the cap becomes hard at capTime. FSRS fuzz runs before this, so a
// fuzzed due can never escape the cap.
func capDue(fsrsDue, now, capTime time.Time) (time.Time, bool) {
	if !now.Before(capTime) {
		return fsrsDue, false
	}
	remaining := capTime.Sub(now)
	squeezed := time.Duration(0.9 * float64(remaining))
	if squeezed < 24*time.Hour {
		squeezed = remaining
	}
	capAt := now.Add(squeezed)
	if fsrsDue.After(capAt) {
		return capAt, true
	}
	return fsrsDue, false
}

// daysLeft counts study days from the start of today through the deadline
// day inclusive; 0 or negative means the deadline has passed. Rounding
// absorbs DST-shortened or -lengthened days.
func daysLeft(deadline string, now time.Time) (int, error) {
	d, err := parseLocalDate(deadline)
	if err != nil {
		return 0, err
	}
	end := startOfDay(d).AddDate(0, 0, 1)
	return int(math.Round(end.Sub(startOfDay(now)).Hours() / 24)), nil
}

// targetNewToday is today's new-card quota under a deadline: spread the
// whole backlog (cards already introduced today count toward it, so the
// quota stays stable across repeated queue fetches) evenly over the
// remaining days, never dropping below the configured daily limit. With no
// days left the deadline is inert and the normal limit applies.
func targetNewToday(remainingNew, introducedToday, daysLeft, newPerDay int) int {
	if daysLeft <= 0 {
		return newPerDay
	}
	total := remainingNew + introducedToday
	target := (total + daysLeft - 1) / daysLeft
	if target < newPerDay {
		return newPerDay
	}
	return target
}
