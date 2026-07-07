package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CTM-development/learning-system-vibe/internal/store"
)

// TestErrorLogFlow drives the error-log lifecycle end to end: sync two
// cards, fail one (Again), triage it, diagnose it, resolve it, and check
// the Today dashboard and stats aggregates track it throughout. It also
// checks that triage excludes undone reviews.
func TestErrorLogFlow(t *testing.T) {
	ts, _, notesDir := newTestServer(t)

	// 1. A note with two Q/A cards.
	if err := os.MkdirAll(filepath.Join(notesDir, "ml"), 0o755); err != nil {
		t.Fatal(err)
	}
	note := "---\ntitle: Variational Inference\n---\n\n" +
		"Q: What is the KL divergence?\nA: A measure of how one distribution diverges from another.\n\n" +
		"Q: What is the ELBO?\nA: The evidence lower bound.\n"
	if err := os.WriteFile(filepath.Join(notesDir, "ml/vi.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	postJSON(t, ts.URL+"/api/sync", nil).Body.Close()

	// 2. Two new cards in the queue.
	queue := decode[queueResp](t, mustGet(t, ts.URL+"/api/queue"))
	if len(queue.New) != 2 {
		t.Fatalf("queue = %+v", queue)
	}
	cardA := queue.New[0].ID
	cardB := queue.New[1].ID

	// 3. Fail card A (Again).
	res := postJSON(t, ts.URL+"/api/reviews", map[string]any{
		"card_id": cardA, "rating": 1, "elapsed_ms": 1500,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("review A: %d", res.StatusCode)
	}
	reviewA := decode[struct {
		EventID int64 `json:"event_id"`
	}](t, res)
	if reviewA.EventID <= 0 {
		t.Fatalf("review A event_id = %d", reviewA.EventID)
	}

	// 4. Pass card B (Good).
	res = postJSON(t, ts.URL+"/api/reviews", map[string]any{
		"card_id": cardB, "rating": 3, "elapsed_ms": 2000,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("review B: %d", res.StatusCode)
	}
	res.Body.Close()

	// 5. Triage shows only the Again review.
	triage := decode[struct {
		Items  []store.TriageItem `json:"items"`
		Causes []string           `json:"causes"`
	}](t, mustGet(t, ts.URL+"/api/errors/triage"))
	if len(triage.Items) != 1 {
		t.Fatalf("triage items = %+v", triage.Items)
	}
	if triage.Items[0].Kind != "card_review" || triage.Items[0].CardID != cardA {
		t.Errorf("triage item = %+v", triage.Items[0])
	}
	if triage.Items[0].CardFront == "" {
		t.Error("triage item missing card_front")
	}
	if len(triage.Causes) != 8 {
		t.Errorf("causes = %+v", triage.Causes)
	}

	// 6. Diagnose the event.
	today := time.Now().Format("2006-01-02")
	res = postJSON(t, ts.URL+"/api/errors", map[string]any{
		"event_id":         reviewA.EventID,
		"root_cause":       "prerequisite",
		"note":             "forgot the KL definition",
		"repair_action":    "reread the KL section",
		"repair_note_path": "ml/vi.md",
		"repair_due":       today,
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create error: %d", res.StatusCode)
	}
	entry := decode[store.ErrorEntry](t, res)
	if entry.EventID != reviewA.EventID || entry.RootCause != "prerequisite" ||
		entry.Note != "forgot the KL definition" ||
		entry.RepairAction != "reread the KL section" ||
		entry.RepairNotePath != "ml/vi.md" || entry.RepairDue != today {
		t.Fatalf("created entry = %+v", entry)
	}
	if entry.ResolvedAt != "" {
		t.Errorf("new entry already resolved: %+v", entry)
	}

	// 7. Duplicate diagnosis on the same event is rejected.
	res = postJSON(t, ts.URL+"/api/errors", map[string]any{
		"event_id": reviewA.EventID, "root_cause": "memory",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("duplicate diagnosis: %d", res.StatusCode)
	}
	res.Body.Close()

	// A valid cause against an unknown event: 404 (event doesn't exist).
	// Root-cause validity is checked before the event lookup, so this must
	// use a valid cause to actually exercise the "unknown event" path.
	res = postJSON(t, ts.URL+"/api/errors", map[string]any{
		"event_id": 99999, "root_cause": "memory",
	})
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("valid cause on unknown event: %d", res.StatusCode)
	}
	res.Body.Close()

	// Get a second real, undiagnosed event: fail card A again.
	res = postJSON(t, ts.URL+"/api/reviews", map[string]any{
		"card_id": cardA, "rating": 1, "elapsed_ms": 1800,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("review A again: %d", res.StatusCode)
	}
	reviewA2 := decode[struct {
		EventID int64 `json:"event_id"`
	}](t, res)
	if reviewA2.EventID <= 0 {
		t.Fatalf("review A2 event_id = %d", reviewA2.EventID)
	}

	// Invalid cause on a real, existing event: 400 (event ok, cause not).
	res = postJSON(t, ts.URL+"/api/errors", map[string]any{
		"event_id": reviewA2.EventID, "root_cause": "bogus",
	})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("bogus cause on real event: %d", res.StatusCode)
	}
	res.Body.Close()

	// 8. Triage now shows just the still-undiagnosed second Again event.
	triage2 := decode[struct {
		Items []store.TriageItem `json:"items"`
	}](t, mustGet(t, ts.URL+"/api/errors/triage"))
	if len(triage2.Items) != 1 || triage2.Items[0].EventID != reviewA2.EventID {
		t.Fatalf("triage after diagnosis = %+v", triage2.Items)
	}

	// 9. Listing errors.
	open := decode[[]store.ErrorEntry](t, mustGet(t, ts.URL+"/api/errors"))
	if len(open) != 1 {
		t.Fatalf("open errors = %+v", open)
	}
	byCause := decode[[]store.ErrorEntry](t, mustGet(t, ts.URL+"/api/errors?cause=prerequisite"))
	if len(byCause) != 1 {
		t.Errorf("errors?cause=prerequisite = %+v", byCause)
	}
	byOtherCause := decode[[]store.ErrorEntry](t, mustGet(t, ts.URL+"/api/errors?cause=memory"))
	if len(byOtherCause) != 0 {
		t.Errorf("errors?cause=memory = %+v", byOtherCause)
	}

	// 10. Today reflects one open error, one repair due today, and one
	// still-undiagnosed triage item.
	todayResp := decode[struct {
		ErrorTriage int                `json:"error_triage"`
		OpenErrors  int                `json:"open_errors"`
		RepairsDue  []store.ErrorEntry `json:"repairs_due"`
	}](t, mustGet(t, ts.URL+"/api/today"))
	if todayResp.OpenErrors != 1 {
		t.Errorf("today.open_errors = %d, want 1", todayResp.OpenErrors)
	}
	if len(todayResp.RepairsDue) != 1 {
		t.Errorf("today.repairs_due = %+v", todayResp.RepairsDue)
	}
	if todayResp.ErrorTriage != len(triage2.Items) {
		t.Errorf("today.error_triage = %d, want %d", todayResp.ErrorTriage, len(triage2.Items))
	}

	// 11. Resolve the diagnosed entry.
	res = patchJSON(t, ts.URL+"/api/errors/"+itoa(entry.ID), map[string]any{"resolved": true})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("resolve: %d", res.StatusCode)
	}
	resolved := decode[store.ErrorEntry](t, res)
	if resolved.ResolvedAt == "" {
		t.Error("resolved entry has empty resolved_at")
	}

	todayResp2 := decode[struct {
		OpenErrors int                `json:"open_errors"`
		RepairsDue []store.ErrorEntry `json:"repairs_due"`
	}](t, mustGet(t, ts.URL+"/api/today"))
	if todayResp2.OpenErrors != 0 {
		t.Errorf("today.open_errors after resolve = %d, want 0", todayResp2.OpenErrors)
	}
	if len(todayResp2.RepairsDue) != 0 {
		t.Errorf("today.repairs_due after resolve = %+v", todayResp2.RepairsDue)
	}
	resolvedList := decode[[]store.ErrorEntry](t, mustGet(t, ts.URL+"/api/errors?status=resolved"))
	if len(resolvedList) != 1 {
		t.Errorf("errors?status=resolved = %+v", resolvedList)
	}

	// 12. Stats.
	stats := decode[struct {
		ByCause []store.CauseCount `json:"by_cause"`
		ByDeck  []store.CauseCount `json:"by_deck"`
	}](t, mustGet(t, ts.URL+"/api/errors/stats"))
	if len(stats.ByCause) != 1 || stats.ByCause[0].Cause != "prerequisite" ||
		stats.ByCause[0].Open != 0 || stats.ByCause[0].Total != 1 {
		t.Errorf("by_cause = %+v", stats.ByCause)
	}
	foundDeck := false
	for _, c := range stats.ByDeck {
		if c.Deck == "ml" {
			foundDeck = true
		}
	}
	if !foundDeck {
		t.Errorf("by_deck missing ml: %+v", stats.ByDeck)
	}

	// 13. Patching an unknown error id 404s.
	res = patchJSON(t, ts.URL+"/api/errors/99999", map[string]any{"resolved": true})
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("patch unknown error: %d", res.StatusCode)
	}
	res.Body.Close()

	// Undone reviews never surface in triage.
	beforeCount := len(triage2.Items)
	res = postJSON(t, ts.URL+"/api/reviews", map[string]any{
		"card_id": cardB, "rating": 1, "elapsed_ms": 1000,
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("review B (again): %d", res.StatusCode)
	}
	res.Body.Close()
	res = postJSON(t, ts.URL+"/api/reviews/undo", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("undo: %d", res.StatusCode)
	}
	res.Body.Close()
	triage3 := decode[struct {
		Items []store.TriageItem `json:"items"`
	}](t, mustGet(t, ts.URL+"/api/errors/triage"))
	if len(triage3.Items) != beforeCount {
		t.Errorf("triage after undo = %d items, want %d", len(triage3.Items), beforeCount)
	}
	for _, it := range triage3.Items {
		if it.CardID == cardB {
			t.Errorf("undone review's card leaked into triage: %+v", it)
		}
	}
}
