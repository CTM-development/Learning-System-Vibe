package store

import (
	"testing"
	"time"
)

func TestEarliestActiveDeadline(t *testing.T) {
	s := openTestStore(t)
	today := "2026-07-10"

	mustProject := func(name, deadline string, dirs []string) {
		if _, err := s.CreateProject(name, deadline, dirs); err != nil {
			t.Fatal(err)
		}
	}
	mustProject("exam", "2026-07-20", []string{"ml"})
	mustProject("sooner", "2026-07-15", []string{"ml/dl", "stats"})
	mustProject("passed", "2026-07-01", []string{"ml"})
	mustProject("no deadline", "", []string{"ml"})
	mustProject("root only", "2026-08-01", []string{""})

	tests := []struct {
		deck string
		want string
		ok   bool
	}{
		{"ml", "2026-07-20", true},          // exact dir; passed project ignored
		{"ml/dl", "2026-07-15", true},       // covered by both — earliest wins
		{"ml/dl/rnn", "2026-07-15", true},   // subfolder of a project dir
		{"stats", "2026-07-15", true},       // exact dir of second project
		{"", "2026-08-01", true},            // root deck matched by dir ""
		{"other", "", false},                // uncovered deck
		{"mlx", "", false},                  // prefix must respect the "/" boundary
	}
	for _, tt := range tests {
		got, ok, err := s.EarliestActiveDeadline(tt.deck, today)
		if err != nil {
			t.Fatalf("EarliestActiveDeadline(%q): %v", tt.deck, err)
		}
		if ok != tt.ok || got != tt.want {
			t.Errorf("EarliestActiveDeadline(%q) = %q/%v, want %q/%v",
				tt.deck, got, ok, tt.want, tt.ok)
		}
	}

	// On the deadline day itself the deadline still counts as active.
	if got, ok, _ := s.EarliestActiveDeadline("ml", "2026-07-20"); !ok || got != "2026-07-20" {
		t.Errorf("deadline day: got %q/%v, want active", got, ok)
	}
	// The day after, it no longer applies.
	if _, ok, _ := s.EarliestActiveDeadline("ml", "2026-07-21"); ok {
		t.Error("passed deadline should not be active")
	}
}

func TestQueueMultiDeckFilter(t *testing.T) {
	s := openTestStore(t)
	now := time.Now()

	mustUpsert := func(id, deck string) {
		if _, err := s.UpsertCard(CardRow{ID: id, NotePath: "n.md", Type: "basic", Front: "f", Back: "b", Deck: deck}); err != nil {
			t.Fatal(err)
		}
	}
	mustUpsert("a", "projA")
	mustUpsert("b", "projB/sub")
	mustUpsert("c", "other")

	newCards, err := s.NewCards(now, 10, []string{"projA", "projB"})
	if err != nil {
		t.Fatal(err)
	}
	if len(newCards) != 2 {
		t.Fatalf("multi-deck new cards = %+v, want a and b", newCards)
	}

	// Overlapping prefixes must not duplicate rows.
	newCards, err = s.NewCards(now, 10, []string{"projA", "projA"})
	if err != nil {
		t.Fatal(err)
	}
	if len(newCards) != 1 {
		t.Fatalf("overlapping decks duplicated cards: %+v", newCards)
	}

	n, err := s.CountNewCards([]string{"projA", "projB"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("CountNewCards = %d, want 2", n)
	}
}

func TestCountNewIntroducedTodayForDecks(t *testing.T) {
	s := openTestStore(t)

	mustUpsert := func(id, deck string) {
		if _, err := s.UpsertCard(CardRow{ID: id, NotePath: "n.md", Type: "basic", Front: "f", Back: "b", Deck: deck}); err != nil {
			t.Fatal(err)
		}
	}
	mustUpsert("in-scope", "projA")
	mustUpsert("out-of-scope", "other")

	introduce := func(cardID string) {
		if _, err := s.LogEvent("card_review", cardID, 0, 0,
			map[string]any{"before": map[string]any{"state": 0}}); err != nil {
			t.Fatal(err)
		}
	}
	introduce("in-scope")
	introduce("out-of-scope")

	n, err := s.CountNewIntroducedTodayForDecks([]string{"projA"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("scoped introduced = %d, want 1", n)
	}
	n, err = s.CountNewIntroducedTodayForDecks(nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("unscoped introduced = %d, want 2", n)
	}
}
