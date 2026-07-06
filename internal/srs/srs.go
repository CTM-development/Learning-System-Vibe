// Package srs wraps go-fsrs: rating transitions on card schedules and
// review-queue construction.
package srs

import (
	"fmt"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

// Schedule mirrors one card_schedule row in scheduler-friendly types.
type Schedule struct {
	CardID        string    `json:"card_id"`
	Due           time.Time `json:"due"`
	Stability     float64   `json:"stability"`
	Difficulty    float64   `json:"difficulty"`
	ElapsedDays   uint64    `json:"elapsed_days"`
	ScheduledDays uint64    `json:"scheduled_days"`
	Reps          uint64    `json:"reps"`
	Lapses        uint64    `json:"lapses"`
	State         int       `json:"state"` // 0 new, 1 learning, 2 review, 3 relearning
	LastReview    time.Time `json:"last_review"` // zero when never reviewed
}

// Scheduler applies FSRS transitions. Interval fuzz is enabled so due dates
// spread instead of clumping.
type Scheduler struct {
	f *fsrs.FSRS
}

// NewScheduler returns a scheduler with default FSRS parameters and fuzz on.
func NewScheduler() *Scheduler {
	param := fsrs.DefaultParam()
	param.EnableFuzz = true
	return &Scheduler{f: fsrs.NewFSRS(param)}
}

// Review applies a rating (1 Again, 2 Hard, 3 Good, 4 Easy) at time now and
// returns the updated schedule.
func (s *Scheduler) Review(cur Schedule, rating int, now time.Time) (Schedule, error) {
	if rating < 1 || rating > 4 {
		return cur, fmt.Errorf("rating %d out of range 1-4", rating)
	}
	card := fsrs.Card{
		Due:           cur.Due,
		Stability:     cur.Stability,
		Difficulty:    cur.Difficulty,
		ElapsedDays:   cur.ElapsedDays,
		ScheduledDays: cur.ScheduledDays,
		Reps:          cur.Reps,
		Lapses:        cur.Lapses,
		State:         fsrs.State(cur.State),
		LastReview:    cur.LastReview,
	}
	next := s.f.Next(card, now, fsrs.Rating(rating)).Card
	return Schedule{
		CardID:        cur.CardID,
		Due:           next.Due,
		Stability:     next.Stability,
		Difficulty:    next.Difficulty,
		ElapsedDays:   next.ElapsedDays,
		ScheduledDays: next.ScheduledDays,
		Reps:          next.Reps,
		Lapses:        next.Lapses,
		State:         int(next.State),
		LastReview:    next.LastReview,
	}, nil
}
