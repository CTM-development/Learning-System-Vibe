package store

import (
	"errors"
	"testing"
	"time"
)

func TestProjectCreateGetRoundTrip(t *testing.T) {
	s := openTestStore(t)

	p, err := s.CreateProject("ML Course", "2026-12-31", []string{"ml/dl", "ml", "ml"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "ML Course" || p.Deadline != "2026-12-31" {
		t.Errorf("created project = %+v", p)
	}
	// Deduped, trimmed, ordered.
	if len(p.Dirs) != 2 || p.Dirs[0] != "ml" || p.Dirs[1] != "ml/dl" {
		t.Errorf("dirs = %v, want [ml ml/dl]", p.Dirs)
	}

	got, err := s.GetProject(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != p.Name || got.Deadline != p.Deadline {
		t.Errorf("get = %+v, want %+v", got, p)
	}
	if len(got.Dirs) != 2 || got.Dirs[0] != "ml" || got.Dirs[1] != "ml/dl" {
		t.Errorf("get dirs = %v", got.Dirs)
	}
}

func TestProjectDirsTrimSlashes(t *testing.T) {
	s := openTestStore(t)
	p, err := s.CreateProject("X", "", []string{"/ml/", "  ml2  "})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Dirs) != 2 || p.Dirs[0] != "ml" || p.Dirs[1] != "ml2" {
		t.Errorf("dirs = %v, want [ml ml2]", p.Dirs)
	}
}

func TestProjectCreateNoDeadline(t *testing.T) {
	s := openTestStore(t)
	p, err := s.CreateProject("No Deadline", "", []string{""})
	if err != nil {
		t.Fatal(err)
	}
	if p.Deadline != "" {
		t.Errorf("deadline = %q, want empty", p.Deadline)
	}
	if len(p.Dirs) != 1 || p.Dirs[0] != "" {
		t.Errorf("dirs = %v, want [\"\"] (root)", p.Dirs)
	}
}

func TestProjectListOrderedByName(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.CreateProject("Zebra", "", []string{"z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProject("Alpha", "", []string{"a"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "Alpha" || list[1].Name != "Zebra" {
		t.Errorf("list = %+v", list)
	}
}

func TestProjectUpdateNameOnly(t *testing.T) {
	s := openTestStore(t)
	p, err := s.CreateProject("Orig", "2026-01-01", []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	newName := "Renamed"
	updated, err := s.UpdateProject(p.ID, &newName, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Renamed" {
		t.Errorf("name = %q", updated.Name)
	}
	if updated.Deadline != "2026-01-01" {
		t.Errorf("deadline changed unexpectedly: %q", updated.Deadline)
	}
	if len(updated.Dirs) != 1 || updated.Dirs[0] != "a" {
		t.Errorf("dirs changed unexpectedly: %v", updated.Dirs)
	}
}

func TestProjectUpdateDirsReplaced(t *testing.T) {
	s := openTestStore(t)
	p, err := s.CreateProject("Orig", "", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.UpdateProject(p.ID, nil, nil, []string{"c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Dirs) != 1 || updated.Dirs[0] != "c" {
		t.Errorf("dirs = %v, want [c]", updated.Dirs)
	}
	if updated.Name != "Orig" {
		t.Errorf("name changed unexpectedly: %q", updated.Name)
	}
}

func TestProjectUpdateDeadlineCleared(t *testing.T) {
	s := openTestStore(t)
	p, err := s.CreateProject("Orig", "2026-01-01", []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	empty := ""
	updated, err := s.UpdateProject(p.ID, nil, &empty, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Deadline != "" {
		t.Errorf("deadline = %q, want cleared", updated.Deadline)
	}
}

func TestProjectUpdateUnknownID(t *testing.T) {
	s := openTestStore(t)
	name := "X"
	_, err := s.UpdateProject(999, &name, nil, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestProjectDelete(t *testing.T) {
	s := openTestStore(t)
	p, err := s.CreateProject("Doomed", "", []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProject(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProject(p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("get after delete: err = %v, want ErrNotFound", err)
	}
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM project_dirs WHERE project_id = ?`, p.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("project_dirs rows remaining = %d, want 0", n)
	}
}

func TestProjectDeleteUnknownID(t *testing.T) {
	s := openTestStore(t)
	if err := s.DeleteProject(999); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// TestProjectCardStats seeds real cards across a deck and its subdeck (plus
// one unrelated deck) and checks total/new/due counts, including that a
// dir of "" matches only root-level cards (deck == ""), not the whole tree.
func TestProjectCardStats(t *testing.T) {
	s := openTestStore(t)
	now := time.Now()

	mustUpsert := func(id, deck string) {
		if _, err := s.UpsertCard(CardRow{ID: id, NotePath: "n.md", Type: "basic", Front: "f", Back: "b", Deck: deck}); err != nil {
			t.Fatal(err)
		}
	}
	mustUpsert("new-ml", "ml")       // new, in ml
	mustUpsert("due-ml-dl", "ml/dl") // will become due, in ml/dl (subdeck of ml)
	mustUpsert("new-other", "other") // unrelated deck
	mustUpsert("root-card", "")      // root-level card

	// Mark due-ml-dl as reviewed and due in the past.
	past := now.Add(-time.Hour).UTC().Format(time.RFC3339)
	if _, err := s.DB.Exec(`UPDATE card_schedule SET state = 2, due = ? WHERE card_id = ?`, past, "due-ml-dl"); err != nil {
		t.Fatal(err)
	}

	total, newCount, dueNow, err := s.ProjectCardStats([]string{"ml"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || newCount != 1 || dueNow != 1 {
		t.Errorf("ml stats = total=%d new=%d due=%d, want 2/1/1", total, newCount, dueNow)
	}

	// dir "" matches only the root-level card, not the whole tree.
	total, newCount, dueNow, err = s.ProjectCardStats([]string{""}, now)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || newCount != 1 || dueNow != 0 {
		t.Errorf(`dir "" stats = total=%d new=%d due=%d, want 1/1/0`, total, newCount, dueNow)
	}

	total, newCount, dueNow, err = s.ProjectCardStats(nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || newCount != 0 || dueNow != 0 {
		t.Errorf("empty dirs stats = total=%d new=%d due=%d, want 0/0/0", total, newCount, dueNow)
	}
}

func TestProjectValidation(t *testing.T) {
	s := openTestStore(t)

	if _, err := s.CreateProject("  ", "", []string{"a"}); err == nil {
		t.Error("want error for empty name")
	}
	if _, err := s.CreateProject("X", "", nil); err == nil {
		t.Error("want error for no dirs")
	}
	if _, err := s.CreateProject("X", "", []string{}); err == nil {
		t.Error("want error for empty dirs slice")
	}
	if _, err := s.CreateProject("X", "not-a-date", []string{"a"}); err == nil {
		t.Error("want error for bad deadline")
	}
}
