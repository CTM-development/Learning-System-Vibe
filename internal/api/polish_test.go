package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CTM-development/learning-system-vibe/internal/srs"
	"github.com/CTM-development/learning-system-vibe/internal/store"
)

type queueResp struct {
	Due          []store.QueueCard `json:"due"`
	New          []store.QueueCard `json:"new"`
	NewRemaining int               `json:"new_remaining"`
	Cram         bool              `json:"cram"`
}

func patchJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPatch, url, jsonBody(t, body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func jsonBody(t *testing.T, body any) *strings.Reader {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return strings.NewReader(string(data))
}

// TestUndoAndBury reviews a card, undoes the review (schedule restored, new
// count restored), then buries it (vanishes from the queue without rating).
func TestUndoAndBury(t *testing.T) {
	ts, srv, notesDir := newTestServer(t)

	if err := os.WriteFile(filepath.Join(notesDir, "n.md"),
		[]byte("Q: q?\nA: a.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	postJSON(t, ts.URL+"/api/sync", nil).Body.Close()

	queue := decode[queueResp](t, mustGet(t, ts.URL+"/api/queue"))
	if len(queue.New) != 1 {
		t.Fatalf("queue = %+v", queue)
	}
	cardID := queue.New[0].ID

	// Nothing to undo yet.
	res := postJSON(t, ts.URL+"/api/reviews/undo", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("undo with no reviews: %d", res.StatusCode)
	}
	res.Body.Close()

	// Review, then undo.
	res = postJSON(t, ts.URL+"/api/reviews", map[string]any{
		"card_id": cardID, "rating": 3, "elapsed_ms": 1200,
	})
	after := decode[srs.Schedule](t, res)
	if after.State == 0 {
		t.Fatalf("review did not advance state: %+v", after)
	}

	res = postJSON(t, ts.URL+"/api/reviews/undo", nil)
	if res.StatusCode != 200 {
		t.Fatalf("undo: %d", res.StatusCode)
	}
	undo := decode[struct {
		CardID   string       `json:"card_id"`
		Schedule srs.Schedule `json:"schedule"`
	}](t, res)
	if undo.CardID != cardID || undo.Schedule.State != 0 || undo.Schedule.Reps != 0 {
		t.Errorf("undo restored %+v", undo)
	}
	sched, err := srv.Store.GetSchedule(cardID)
	if err != nil {
		t.Fatal(err)
	}
	if sched.State != 0 {
		t.Errorf("schedule after undo = %+v", sched)
	}

	// The undone review no longer counts against the daily new limit and
	// the card is back in the new queue.
	if n, _ := srv.Store.CountNewIntroducedToday(); n != 0 {
		t.Errorf("introduced today after undo = %d", n)
	}
	queue = decode[queueResp](t, mustGet(t, ts.URL+"/api/queue"))
	if len(queue.New) != 1 {
		t.Errorf("card not back in queue after undo: %+v", queue)
	}

	// A second undo has nothing left to revert.
	res = postJSON(t, ts.URL+"/api/reviews/undo", nil)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("second undo: %d", res.StatusCode)
	}
	res.Body.Close()

	// Bury: card leaves the queue without any rating.
	res = postJSON(t, ts.URL+"/api/cards/"+cardID+"/bury", nil)
	if res.StatusCode != 200 {
		t.Fatalf("bury: %d", res.StatusCode)
	}
	res.Body.Close()
	queue = decode[queueResp](t, mustGet(t, ts.URL+"/api/queue"))
	if len(queue.New) != 0 || len(queue.Due) != 0 {
		t.Errorf("buried card still queued: %+v", queue)
	}
	// It reappears once the bury expires.
	if err := srv.Store.BuryCard(cardID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	queue = decode[queueResp](t, mustGet(t, ts.URL+"/api/queue"))
	if len(queue.New) != 1 {
		t.Errorf("card missing after bury expiry: %+v", queue)
	}
}

// TestCramQueue: cram returns all deck cards (incl. not-due ones) weakest
// first, and deck filtering covers subfolders.
func TestCramQueue(t *testing.T) {
	ts, _, notesDir := newTestServer(t)

	if err := os.MkdirAll(filepath.Join(notesDir, "math/linalg"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"math/a.md":        "Q: a?\nA: a.\n",
		"math/linalg/b.md": "Q: b?\nA: b.\n",
		"other.md":         "Q: c?\nA: c.\n",
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(notesDir, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	postJSON(t, ts.URL+"/api/sync", nil).Body.Close()

	// Review one math card so it is scheduled in the future (not due).
	queue := decode[queueResp](t, mustGet(t, ts.URL+"/api/queue?deck=math"))
	if len(queue.New) != 2 {
		t.Fatalf("deck-scoped queue = %+v", queue)
	}
	reviewed := queue.New[0].ID
	postJSON(t, ts.URL+"/api/reviews", map[string]any{
		"card_id": reviewed, "rating": 4, "elapsed_ms": 500, "cram": true,
	}).Body.Close()

	// Normal deck queue no longer offers the reviewed card…
	queue = decode[queueResp](t, mustGet(t, ts.URL+"/api/queue?deck=math"))
	if len(queue.Due)+len(queue.New) != 1 {
		t.Fatalf("queue after review = %+v", queue)
	}
	// …but cram serves both deck cards (subfolder included), not the third.
	cram := decode[queueResp](t, mustGet(t, ts.URL+"/api/queue?deck=math&cram=1"))
	if !cram.Cram || len(cram.Due) != 2 {
		t.Fatalf("cram queue = %+v", cram)
	}
	for _, c := range cram.Due {
		if !strings.HasPrefix(c.Deck, "math") {
			t.Errorf("cram leaked card from deck %q", c.Deck)
		}
	}

	// Cram without a deck is a client error.
	res, err := http.Get(ts.URL + "/api/queue?cram=1")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("cram without deck: %d", res.StatusCode)
	}
	res.Body.Close()
}

// TestQuestionLifecycleAndCapture: quick capture lands in inbox.md and the
// question queue; PATCH moves a question through its lifecycle.
func TestQuestionLifecycleAndCapture(t *testing.T) {
	ts, _, notesDir := newTestServer(t)

	res := postJSON(t, ts.URL+"/api/capture", map[string]string{
		"text": "Why does FSRS use fuzz?",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("capture: %d", res.StatusCode)
	}
	res.Body.Close()
	res = postJSON(t, ts.URL+"/api/capture", map[string]string{
		"text": "What is a leech threshold?",
	})
	res.Body.Close()

	raw, err := os.ReadFile(filepath.Join(notesDir, "inbox.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if !strings.Contains(content, "## Open questions") ||
		!strings.Contains(content, "- Why does FSRS use fuzz?") ||
		!strings.Contains(content, "- What is a leech threshold?") {
		t.Fatalf("inbox.md = %q", content)
	}

	questions := decode[[]store.OpenQuestion](t, mustGet(t, ts.URL+"/api/questions?status=open"))
	if len(questions) != 2 {
		t.Fatalf("questions = %+v", questions)
	}

	// Mark one as carded.
	res = patchJSON(t, ts.URL+"/api/questions/"+itoa(questions[0].ID),
		map[string]string{"status": "carded"})
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("patch question: %d", res.StatusCode)
	}
	res.Body.Close()
	open := decode[[]store.OpenQuestion](t, mustGet(t, ts.URL+"/api/questions?status=open"))
	carded := decode[[]store.OpenQuestion](t, mustGet(t, ts.URL+"/api/questions?status=carded"))
	if len(open) != 1 || len(carded) != 1 {
		t.Errorf("open=%d carded=%d", len(open), len(carded))
	}

	// Invalid status rejected.
	res = patchJSON(t, ts.URL+"/api/questions/"+itoa(questions[1].ID),
		map[string]string{"status": "bogus"})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("invalid status: %d", res.StatusCode)
	}
	res.Body.Close()
}

// TestNoteAssetsAndLinks: relative images are served path-confined; note
// detail carries resolved links, red links and backlinks.
func TestNoteAssetsAndLinks(t *testing.T) {
	ts, _, notesDir := newTestServer(t)

	if err := os.MkdirAll(filepath.Join(notesDir, "ml"), 0o755); err != nil {
		t.Fatal(err)
	}
	png := []byte("\x89PNG\r\n\x1a\nfakeimage")
	if err := os.WriteFile(filepath.Join(notesDir, "ml/fig.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"ml/vi.md": "---\ntitle: Variational Inference\n---\n\nSee [[Bayes Rule]] and [[Missing Topic]].\n\n![fig](fig.png)\n",
		"bayes.md": "---\ntitle: Bayes Rule\n---\n\nRelates to [[Variational Inference]].\n",
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(notesDir, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	postJSON(t, ts.URL+"/api/sync", nil).Body.Close()

	// Asset served.
	res := mustGet(t, ts.URL+"/api/notes-assets/ml/fig.png")
	res.Body.Close()

	// Path traversal blocked.
	req, _ := http.NewRequest("GET", ts.URL+"/api/notes-assets/../secret.txt", nil)
	req.URL.Path = "/api/notes-assets/../secret.txt" // bypass client-side cleaning
	res, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode == 200 {
		t.Error("path traversal served a file")
	}
	res.Body.Close()

	// Links: one resolved (by title), one red; backlink present both ways.
	note := decode[store.NoteDetail](t, mustGet(t, ts.URL+"/api/notes/ml/vi.md"))
	if len(note.Links) != 2 {
		t.Fatalf("links = %+v", note.Links)
	}
	byTarget := map[string]string{}
	for _, l := range note.Links {
		byTarget[l.Target] = l.ToPath
	}
	if byTarget["Bayes Rule"] != "bayes.md" {
		t.Errorf("Bayes Rule resolved to %q", byTarget["Bayes Rule"])
	}
	if byTarget["Missing Topic"] != "" {
		t.Errorf("Missing Topic resolved to %q, want red link", byTarget["Missing Topic"])
	}
	if len(note.Backlinks) != 1 || note.Backlinks[0].Path != "bayes.md" {
		t.Errorf("backlinks = %+v", note.Backlinks)
	}
}

// TestReferenceSourcesAndToday: JSON source creation for url/book kinds,
// and the Today dashboard aggregates.
func TestReferenceSourcesAndToday(t *testing.T) {
	ts, _, notesDir := newTestServer(t)

	res := postJSON(t, ts.URL+"/api/sources", map[string]string{
		"kind": "book", "title": "Pattern Recognition and Machine Learning",
		"key": "bishop2006",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create book source: %d", res.StatusCode)
	}
	book := decode[store.SourceRow](t, res)
	if book.Kind != "book" || book.Key != "bishop2006" {
		t.Errorf("book = %+v", book)
	}

	res = postJSON(t, ts.URL+"/api/sources", map[string]string{
		"kind": "url", "title": "FSRS explained", "url": "https://example.com/fsrs",
	})
	url := decode[store.SourceRow](t, res)
	if url.Kind != "url" || !strings.Contains(url.Meta, "example.com/fsrs") {
		t.Errorf("url source = %+v", url)
	}

	// A file-less source has no file endpoint.
	fileRes, err := http.Get(ts.URL + "/api/sources/" + itoa(book.ID) + "/file")
	if err != nil {
		t.Fatal(err)
	}
	if fileRes.StatusCode != http.StatusNotFound {
		t.Errorf("file for book source: %d", fileRes.StatusCode)
	}
	fileRes.Body.Close()

	// Invalid kind rejected.
	res = postJSON(t, ts.URL+"/api/sources", map[string]string{"kind": "pdf", "title": "x"})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("pdf via JSON: %d", res.StatusCode)
	}
	res.Body.Close()

	// Today: stale skim note + open question + counts.
	old := time.Now().AddDate(0, 0, -30)
	stalePath := filepath.Join(notesDir, "stale.md")
	if err := os.WriteFile(stalePath,
		[]byte("---\nstage: skim\n---\n# Old\n\n## Open questions\n\n- What?\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(stalePath, old, old); err != nil {
		t.Fatal(err)
	}
	postJSON(t, ts.URL+"/api/sync", nil).Body.Close()

	today := decode[struct {
		Summary       store.StatsSummary `json:"summary"`
		StaleNotes    []store.StaleNote  `json:"stale_notes"`
		OpenQuestions int                `json:"open_questions"`
		Leeches       int                `json:"leeches"`
	}](t, mustGet(t, ts.URL+"/api/today"))
	if len(today.StaleNotes) != 1 || today.StaleNotes[0].Path != "stale.md" ||
		today.StaleNotes[0].IdleDays < 29 {
		t.Errorf("stale notes = %+v", today.StaleNotes)
	}
	if today.OpenQuestions != 1 {
		t.Errorf("open questions = %d", today.OpenQuestions)
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
