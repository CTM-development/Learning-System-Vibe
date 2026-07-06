package srs

import (
	"testing"
	"time"
)

func TestReviewTransitions(t *testing.T) {
	s := NewScheduler()
	now := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)

	newCard := Schedule{CardID: "x", Due: now, State: 0}

	// Good on a new card → learning/review, due in the future, 1 rep.
	after, err := s.Review(newCard, 3, now)
	if err != nil {
		t.Fatal(err)
	}
	if after.Reps != 1 {
		t.Errorf("reps = %d", after.Reps)
	}
	if !after.Due.After(now) {
		t.Errorf("due = %v, want after now", after.Due)
	}
	if after.State == 0 {
		t.Error("state still new after review")
	}
	if after.LastReview != now {
		t.Errorf("last_review = %v", after.LastReview)
	}

	// Again on a reviewed card increments lapses once it was in review state.
	later := after.Due.Add(time.Hour)
	reviewed, err := s.Review(after, 3, later)
	if err != nil {
		t.Fatal(err)
	}
	lapsed, err := s.Review(reviewed, 1, reviewed.Due.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if lapsed.Lapses != reviewed.Lapses+1 && lapsed.State == 3 {
		t.Errorf("lapses = %d after Again (was %d)", lapsed.Lapses, reviewed.Lapses)
	}

	// Easy grows the interval faster than Hard.
	easy, _ := s.Review(reviewed, 4, reviewed.Due)
	hard, _ := s.Review(reviewed, 2, reviewed.Due)
	if !easy.Due.After(hard.Due) {
		t.Errorf("easy due %v should be after hard due %v", easy.Due, hard.Due)
	}
}

func TestReviewRejectsBadRating(t *testing.T) {
	s := NewScheduler()
	if _, err := s.Review(Schedule{}, 0, time.Now()); err == nil {
		t.Error("want error for rating 0")
	}
	if _, err := s.Review(Schedule{}, 5, time.Now()); err == nil {
		t.Error("want error for rating 5")
	}
}
