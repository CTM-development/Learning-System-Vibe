package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CTM-development/learning-system-vibe/internal/srs"
	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// TestProjectQueueDeadline drives the deadline flow end to end: a project
// over two directories with a deadline tomorrow must (a) pull cards from
// both dirs, (b) raise the new-card quota above the global limit so the
// backlog fits before the deadline, and (c) cap a reviewed card's next due
// date to land before the deadline day.
func TestProjectQueueDeadline(t *testing.T) {
	ts, srv, notesDir := newTestServer(t)
	srv.Config.NewPerDay = 1

	// Three new cards across two project dirs.
	for _, f := range []struct{ path, body string }{
		{"projA/a1.md", "Q: a1?\nA: x.\n"},
		{"projA/a2.md", "Q: a2?\nA: x.\n"},
		{"projB/b1.md", "Q: b1?\nA: x.\n"},
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(notesDir, f.path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(notesDir, f.path), []byte(f.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res := postJSON(t, ts.URL+"/api/sync", nil)
	if res.StatusCode != 200 {
		t.Fatalf("sync: %d", res.StatusCode)
	}
	res.Body.Close()

	// Project over both dirs, deadline tomorrow → 2 study days for 3 new
	// cards → quota ceil(3/2) = 2, above the global limit of 1.
	deadline := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	res = postJSON(t, ts.URL+"/api/projects", map[string]any{
		"name": "DL", "dirs": []string{"projA", "projB"}, "deadline": deadline,
	})
	if res.StatusCode != 200 {
		t.Fatalf("create project: %d", res.StatusCode)
	}
	project := decode[ProjectInfo](t, res)
	if project.NewCards != 3 {
		t.Fatalf("project new_cards = %d, want 3", project.NewCards)
	}

	// deck and project are mutually exclusive.
	res, err := http.Get(fmt.Sprintf("%s/api/queue?deck=projA&project=%d", ts.URL, project.ID))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("deck+project status = %d, want 400", res.StatusCode)
	}

	// Unknown project → 404.
	res, err = http.Get(ts.URL + "/api/queue?project=999")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown project status = %d, want 404", res.StatusCode)
	}

	// Project queue: cards from both dirs, quota raised above NewPerDay.
	queue := decode[struct {
		Due            []store.QueueCard `json:"due"`
		New            []store.QueueCard `json:"new"`
		NewRemaining   int               `json:"new_remaining"`
		Project        int64             `json:"project"`
		Deadline       string            `json:"deadline"`
		DaysLeft       int               `json:"days_left"`
		TargetNewToday int               `json:"target_new_today"`
	}](t, mustGet(t, fmt.Sprintf("%s/api/queue?project=%d", ts.URL, project.ID)))
	if queue.Project != project.ID || queue.Deadline != deadline {
		t.Errorf("queue project/deadline = %d/%q", queue.Project, queue.Deadline)
	}
	if queue.DaysLeft != 2 {
		t.Errorf("days_left = %d, want 2", queue.DaysLeft)
	}
	if queue.TargetNewToday != 2 || len(queue.New) != 2 || queue.NewRemaining != 2 {
		t.Errorf("target=%d new=%d remaining=%d, want 2/2/2",
			queue.TargetNewToday, len(queue.New), queue.NewRemaining)
	}
	decksSeen := map[string]bool{}
	for _, c := range queue.New {
		decksSeen[c.Deck] = true
	}
	if !decksSeen["projA"] {
		t.Errorf("expected a projA card in %+v", queue.New)
	}

	// The unscoped queue is untouched by the project quota: global limit 1.
	plain := decode[struct {
		New []store.QueueCard `json:"new"`
	}](t, mustGet(t, ts.URL+"/api/queue"))
	if len(plain.New) != 1 {
		t.Errorf("unscoped new = %d, want NewPerDay(1)", len(plain.New))
	}

	// Reviewing a project card caps its next due before the deadline day,
	// whatever FSRS (with fuzz) wanted. Easy on a new card normally lands
	// days out, past our tomorrow deadline.
	capTime, err := parseLocalDate(deadline)
	if err != nil {
		t.Fatal(err)
	}
	res = postJSON(t, ts.URL+"/api/reviews", map[string]any{
		"card_id": queue.New[0].ID, "rating": 4, "elapsed_ms": 1000,
	})
	if res.StatusCode != 200 {
		t.Fatalf("review: %d", res.StatusCode)
	}
	reviewed := decode[struct {
		Schedule srs.Schedule `json:"schedule"`
	}](t, res)
	if reviewed.Schedule.Due.After(capTime) {
		t.Errorf("due %v not capped before deadline day start %v",
			reviewed.Schedule.Due, capTime)
	}

	// Cram works with a project scope.
	cram := decode[struct {
		Due  []store.QueueCard `json:"due"`
		Cram bool              `json:"cram"`
	}](t, mustGet(t, fmt.Sprintf("%s/api/queue?project=%d&cram=1", ts.URL, project.ID)))
	if !cram.Cram || len(cram.Due) != 3 {
		t.Errorf("cram = %v with %d cards, want 3", cram.Cram, len(cram.Due))
	}
}

// TestProjectQueueNoDeadline: a deadline-less project scopes the queue but
// leaves the global new-card limit untouched.
func TestProjectQueueNoDeadline(t *testing.T) {
	ts, srv, notesDir := newTestServer(t)
	srv.Config.NewPerDay = 1

	if err := os.MkdirAll(filepath.Join(notesDir, "projA"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a1.md", "a2.md"} {
		if err := os.WriteFile(filepath.Join(notesDir, "projA", f), []byte("Q: q?\nA: a.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res := postJSON(t, ts.URL+"/api/sync", nil)
	res.Body.Close()

	res = postJSON(t, ts.URL+"/api/projects", map[string]any{
		"name": "NoDl", "dirs": []string{"projA"},
	})
	project := decode[ProjectInfo](t, res)

	queue := decode[struct {
		New            []store.QueueCard `json:"new"`
		Project        int64             `json:"project"`
		TargetNewToday *int              `json:"target_new_today"`
	}](t, mustGet(t, fmt.Sprintf("%s/api/queue?project=%d", ts.URL, project.ID)))
	if queue.Project != project.ID {
		t.Errorf("queue project = %d, want %d", queue.Project, project.ID)
	}
	if len(queue.New) != 1 {
		t.Errorf("new = %d, want global limit 1", len(queue.New))
	}
	if queue.TargetNewToday != nil {
		t.Errorf("target_new_today should be absent without a deadline, got %d", *queue.TargetNewToday)
	}
}
