package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/CTM-development/learning-system-vibe/internal/config"
	"github.com/CTM-development/learning-system-vibe/internal/mdsync"
	"github.com/CTM-development/learning-system-vibe/internal/sources"
	"github.com/CTM-development/learning-system-vibe/internal/srs"
	"github.com/CTM-development/learning-system-vibe/internal/store"
)

func newTestServer(t *testing.T) (*httptest.Server, *store.Store, string) {
	t.Helper()
	notesDir := t.TempDir()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	srv := &Server{
		Store:     st,
		Syncer:    &mdsync.Syncer{Store: st, NotesDir: notesDir},
		Scheduler: srs.NewScheduler(),
		Sources:   &sources.Manager{Store: st, AttachmentsDir: t.TempDir()},
		Config:    config.Default(),
		Version:   "test",
	}
	ts := httptest.NewServer(srv.Handler(fstest.MapFS{}))
	t.Cleanup(ts.Close)
	return ts, st, notesDir
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	data, _ := json.Marshal(body)
	res, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func decode[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	defer res.Body.Close()
	var v T
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TestReviewLoop drives the full cycle: session start → sync a note →
// queue → review → schedule advanced + event attributed to the session.
func TestReviewLoop(t *testing.T) {
	ts, st, notesDir := newTestServer(t)

	// Start a learning session.
	res := postJSON(t, ts.URL+"/api/sessions/start", map[string]string{"kind": "learning"})
	if res.StatusCode != 200 {
		t.Fatalf("session start: %d", res.StatusCode)
	}
	sess := decode[store.Session](t, res)

	// Create a note and sync it.
	err := os.WriteFile(filepath.Join(notesDir, "n.md"), []byte("Q: q?\nA: a.\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	res = postJSON(t, ts.URL+"/api/sync", nil)
	if res.StatusCode != 200 {
		t.Fatalf("sync: %d", res.StatusCode)
	}
	res.Body.Close()

	// Queue: one new card, none due.
	queue := decode[struct {
		Due []store.QueueCard `json:"due"`
		New []store.QueueCard `json:"new"`
	}](t, mustGet(t, ts.URL+"/api/queue"))
	if len(queue.Due) != 0 || len(queue.New) != 1 {
		t.Fatalf("queue = %+v", queue)
	}
	cardID := queue.New[0].ID

	// Review it with Good.
	res = postJSON(t, ts.URL+"/api/reviews", map[string]any{
		"card_id": cardID, "rating": 3, "elapsed_ms": 3500,
	})
	if res.StatusCode != 200 {
		t.Fatalf("review: %d", res.StatusCode)
	}
	after := decode[srs.Schedule](t, res)
	if after.Reps != 1 || after.State == 0 {
		t.Errorf("schedule after review = %+v", after)
	}

	// The card leaves the new queue.
	queue2 := decode[struct {
		New []store.QueueCard `json:"new"`
	}](t, mustGet(t, ts.URL+"/api/queue"))
	if len(queue2.New) != 0 {
		t.Errorf("card still in new queue: %+v", queue2.New)
	}

	// Event was logged with latency and session attribution.
	var kind string
	var elapsed, sessionID int64
	var payload string
	err = st.DB.QueryRow(
		`SELECT kind, elapsed_ms, session_id, payload FROM activity_events WHERE kind = 'card_review'`).
		Scan(&kind, &elapsed, &sessionID, &payload)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed != 3500 || sessionID != sess.ID {
		t.Errorf("event elapsed=%d session=%d, want 3500/%d", elapsed, sessionID, sess.ID)
	}
	var p struct {
		Rating int `json:"rating"`
		Before struct {
			State int `json:"state"`
		} `json:"before"`
	}
	json.Unmarshal([]byte(payload), &p)
	if p.Rating != 3 || p.Before.State != 0 {
		t.Errorf("payload = %s", payload)
	}

	// New-per-day accounting sees the introduction.
	n, err := st.CountNewIntroducedToday()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("introduced today = %d", n)
	}

	// Stop the session; a client event after that carries no session.
	res = postJSON(t, ts.URL+"/api/sessions/stop", nil)
	stopped := decode[store.Session](t, res)
	if stopped.EndedAt == "" {
		t.Error("session not ended")
	}
	res = postJSON(t, ts.URL+"/api/events", map[string]any{
		"kind": "note_read", "ref": "n.md", "elapsed_ms": 60000,
	})
	if res.StatusCode != 201 {
		t.Fatalf("post event: %d", res.StatusCode)
	}
	res.Body.Close()
	var sid *int64
	err = st.DB.QueryRow(`SELECT session_id FROM activity_events WHERE kind = 'note_read'`).Scan(&sid)
	if err != nil {
		t.Fatal(err)
	}
	if sid != nil {
		t.Errorf("note_read session_id = %v, want NULL", *sid)
	}
}

func mustGet(t *testing.T, url string) *http.Response {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("GET %s: %d", url, res.StatusCode)
	}
	return res
}
