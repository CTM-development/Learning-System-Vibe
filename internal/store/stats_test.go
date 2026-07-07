package store

import (
	"strings"
	"testing"
	"time"

	"github.com/CTM-development/learning-system-vibe/internal/srs"
)

// seedCard inserts a card with its schedule row.
func seedCard(t *testing.T, s *Store, id, deck string) {
	t.Helper()
	if _, err := s.UpsertCard(CardRow{
		ID: id, NotePath: "n.md", Type: "basic", Front: "f " + id, Back: "b", Deck: deck,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSummaryAndHeatmapAndTime(t *testing.T) {
	s := openTestStore(t)
	seedCard(t, s, "aaa", "math")
	seedCard(t, s, "bbb", "math")
	seedCard(t, s, "ccc", "cs")

	// Simulate: aaa reviewed (was new), with latency, in a session.
	sess, err := s.StartSession("learning", "")
	if err != nil {
		t.Fatal(err)
	}
	before := srs.Schedule{CardID: "aaa", State: 0}
	after := srs.Schedule{CardID: "aaa", State: 2, Reps: 1,
		Due: time.Now().Add(48 * time.Hour), LastReview: time.Now()}
	if err := s.UpdateSchedule(after); err != nil {
		t.Fatal(err)
	}
	err = s.LogEvent("card_review", "aaa", 5000, sess.ID,
		map[string]any{"rating": 3, "before": before, "after": after})
	if err != nil {
		t.Fatal(err)
	}
	// A review of a review-state card that failed (for retention). 3001ms
	// makes the average non-integral — AVG() returns float64.
	err = s.LogEvent("card_review", "aaa", 3001, sess.ID,
		map[string]any{"rating": 1, "before": after, "after": after})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.LogEvent("note_read", "n.md", 60000, sess.ID, nil); err != nil {
		t.Fatal(err)
	}

	sum, err := s.Summary(10, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if sum.TotalCards != 3 {
		t.Errorf("TotalCards = %d", sum.TotalCards)
	}
	if sum.ReviewsToday != 2 {
		t.Errorf("ReviewsToday = %d", sum.ReviewsToday)
	}
	if sum.TimeTodayMs != 68001 {
		t.Errorf("TimeTodayMs = %d", sum.TimeTodayMs)
	}
	if sum.AvgReviewMs != 4000 { // (5000+3001)/2 truncated
		t.Errorf("AvgReviewMs = %d", sum.AvgReviewMs)
	}
	if sum.NewRemaining != 9 { // one new card introduced today
		t.Errorf("NewRemaining = %d", sum.NewRemaining)
	}
	// One review-state review, rating 1 → retention 0.
	if sum.Retention30 != 0 {
		t.Errorf("Retention30 = %v", sum.Retention30)
	}

	heat, err := s.ReviewHeatmap(30)
	if err != nil {
		t.Fatal(err)
	}
	if len(heat) != 1 || heat[0].Count != 2 {
		t.Errorf("heatmap = %+v", heat)
	}

	byKind, err := s.TimeByKind(30)
	if err != nil {
		t.Fatal(err)
	}
	if len(byKind) != 2 || byKind[0].Key != "note_read" || byKind[0].Ms != 60000 {
		t.Errorf("byKind = %+v", byKind)
	}
	byDeck, err := s.TimeByDeck(30)
	if err != nil {
		t.Fatal(err)
	}
	if len(byDeck) != 1 || byDeck[0].Key != "math" || byDeck[0].Ms != 8001 {
		t.Errorf("byDeck = %+v", byDeck)
	}
}

func TestDueForecastAndBrowse(t *testing.T) {
	s := openTestStore(t)
	seedCard(t, s, "due-tomorrow", "d")
	seedCard(t, s, "overdue", "d")
	seedCard(t, s, "still-new", "d")

	set := func(id string, due time.Time, state int) {
		t.Helper()
		if err := s.UpdateSchedule(srs.Schedule{
			CardID: id, Due: due, State: state, LastReview: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	set("due-tomorrow", time.Now().Add(24*time.Hour), 2)
	set("overdue", time.Now().Add(-48*time.Hour), 2)

	forecast, overdue, err := s.DueForecast(7)
	if err != nil {
		t.Fatal(err)
	}
	if overdue != 1 {
		t.Errorf("overdue = %d", overdue)
	}
	total := 0
	for _, d := range forecast {
		total += d.Count
	}
	if total != 1 {
		t.Errorf("forecast = %+v, want 1 total", forecast)
	}

	// Browse: all active, then suspend one and filter.
	cards, err := s.BrowseCards("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 3 {
		t.Errorf("active cards = %d", len(cards))
	}
	if err := s.SetCardSuspended("overdue", true); err != nil {
		t.Fatal(err)
	}
	cards, _ = s.BrowseCards("", "", "")
	if len(cards) != 2 {
		t.Errorf("after suspend, active = %d", len(cards))
	}
	suspended, _ := s.BrowseCards("", "", "suspended")
	if len(suspended) != 1 || suspended[0].ID != "overdue" {
		t.Errorf("suspended = %+v", suspended)
	}
	// Suspended card leaves the review queue.
	due, err := s.DueCards(time.Now(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Errorf("due queue should be empty, got %+v", due)
	}
	// Text filter.
	hits, _ := s.BrowseCards("f due-tomorrow", "", "all")
	if len(hits) != 1 {
		t.Errorf("text filter hits = %d", len(hits))
	}
	if err := s.SetCardSuspended("nope", true); err == nil {
		t.Error("want ErrNotFound for unknown card")
	}
}

func TestSearchNotes(t *testing.T) {
	s := openTestStore(t)
	err := s.UpsertNote(NoteRow{
		Path: "ml/vi.md", Title: "Variational Inference",
		Content: "The ELBO lower-bounds the marginal log likelihood.",
	})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := s.SearchNotes("elbo marginal", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Path != "ml/vi.md" {
		t.Fatalf("hits = %+v", hits)
	}
	if hits[0].Snippet == "" {
		t.Error("empty snippet")
	}
	if !strings.Contains(hits[0].Snippet, "<mark>") {
		t.Errorf("snippet lacks <mark>: %q", hits[0].Snippet)
	}
	// Note HTML must arrive escaped.
	s.UpsertNote(NoteRow{Path: "evil.md", Title: "evil",
		Content: `<script>alert(1)</script> targetword here`})
	hits, _ = s.SearchNotes("targetword", 10)
	if len(hits) != 1 || strings.Contains(hits[0].Snippet, "<script>") {
		t.Errorf("unescaped snippet: %+v", hits)
	}
	// Prefix match works.
	hits, _ = s.SearchNotes("margin", 10)
	if len(hits) != 1 {
		t.Errorf("prefix search hits = %d", len(hits))
	}
	// FTS operator injection is neutralized.
	if _, err := s.SearchNotes(`elbo AND NEAR( "`, 10); err != nil {
		t.Errorf("operator-ish query errored: %v", err)
	}
	// Empty query returns empty, no error.
	hits, err = s.SearchNotes("   ", 10)
	if err != nil || len(hits) != 0 {
		t.Errorf("empty query: %v %v", hits, err)
	}
}
