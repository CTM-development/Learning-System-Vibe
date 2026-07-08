package mdsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CTM-development/learning-system-vibe/internal/store"
)

func newTestSyncer(t *testing.T) (*Syncer, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	return &Syncer{Store: st, NotesDir: dir}, dir
}

func writeNote(t *testing.T, dir, rel, content string) {
	t.Helper()
	abs := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readNote(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func cardIDs(t *testing.T, s *Syncer) []string {
	t.Helper()
	ids, err := s.Store.ListActiveCardIDs()
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestIncrementalSyncSkipsUnchangedFiles(t *testing.T) {
	s, dir := newTestSyncer(t)
	writeNote(t, dir, "a.md", "Q: What is FSRS?\nA: A scheduler.\n")
	writeNote(t, dir, "b.md", "# Just prose\n\nNo cards here.\n")

	// First run creates the card and stamps the anchor.
	if _, err := s.SyncAll(); err != nil {
		t.Fatal(err)
	}

	// Second run with no file changes must do no work: no cards created,
	// updated or orphaned, and no anchors rewritten.
	res, err := s.SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	if res.CardsCreated != 0 || res.CardsUpdated != 0 || res.CardsOrphaned != 0 || res.AnchorsWritten != 0 {
		t.Errorf("re-sync of unchanged tree did work: %+v", res)
	}
	if ids := cardIDs(t, s); len(ids) != 1 {
		t.Errorf("active cards after no-op re-sync = %v, want 1 (skip must not orphan)", ids)
	}

	// Editing one file re-syncs only it: the card's content updates.
	writeNote(t, dir, "a.md", readNote(t, dir, "a.md")+"\nQ: What does fuzz do?\nA: Spreads due dates.\n")
	res, err = s.SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	if res.CardsCreated != 1 {
		t.Errorf("editing a file did not pick up the new card: %+v", res)
	}
	if ids := cardIDs(t, s); len(ids) != 2 {
		t.Errorf("active cards after edit = %v, want 2", ids)
	}
}

func TestSyncWritesAnchorsAndCreatesCards(t *testing.T) {
	s, dir := newTestSyncer(t)
	writeNote(t, dir, "math/algebra.md", "Q: What is a group?\nA: A set with an associative operation, identity and inverses.\n\nA ring has {{c1::two}} operations.\n")

	res, err := s.SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	if res.CardsCreated != 2 || res.AnchorsWritten != 2 {
		t.Errorf("res = %+v, want 2 created / 2 anchors", res)
	}

	content := readNote(t, dir, "math/algebra.md")
	if n := strings.Count(content, "<!-- srs:"); n != 2 {
		t.Fatalf("anchors in file = %d, want 2\n%s", n, content)
	}
	// Prose is untouched apart from appended anchors.
	if !strings.Contains(content, "A: A set with an associative operation, identity and inverses. <!-- srs:") {
		t.Errorf("anchor not appended to answer line:\n%s", content)
	}

	// Second sync is a no-op: stable IDs, no new anchors.
	before := cardIDs(t, s)
	res2, err := s.SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	if res2.CardsCreated != 0 || res2.AnchorsWritten != 0 {
		t.Errorf("second sync = %+v, want no creations", res2)
	}
	after := cardIDs(t, s)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Errorf("card ids changed across syncs: %v vs %v", before, after)
	}
}

func TestEditingWordingKeepsIDAndSchedule(t *testing.T) {
	s, dir := newTestSyncer(t)
	writeNote(t, dir, "note.md", "Q: What is FSRS?\nA: A scheduler.\n")
	if _, err := s.SyncAll(); err != nil {
		t.Fatal(err)
	}
	ids := cardIDs(t, s)
	if len(ids) != 1 {
		t.Fatalf("ids = %v", ids)
	}
	id := ids[0]

	// Simulate review history on the card.
	if _, err := s.Store.DB.Exec(
		`UPDATE card_schedule SET reps = 5, stability = 12.5 WHERE card_id = ?`, id); err != nil {
		t.Fatal(err)
	}

	// Edit wording, keep anchor (as an editor would).
	content := readNote(t, dir, "note.md")
	content = strings.Replace(content, "A scheduler.", "A modern spaced-repetition scheduler.", 1)
	writeNote(t, dir, "note.md", content)

	res, err := s.SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	if res.CardsCreated != 0 {
		t.Errorf("edit created a new card: %+v", res)
	}

	var back string
	var reps int
	err = s.Store.DB.QueryRow(
		`SELECT c.back, cs.reps FROM cards c JOIN card_schedule cs ON cs.card_id = c.id WHERE c.id = ?`,
		id).Scan(&back, &reps)
	if err != nil {
		t.Fatal(err)
	}
	if back != "A modern spaced-repetition scheduler." {
		t.Errorf("back = %q", back)
	}
	if reps != 5 {
		t.Errorf("reps = %d, schedule was reset", reps)
	}
}

func TestVanishedCardIsOrphanedAndRestorable(t *testing.T) {
	s, dir := newTestSyncer(t)
	writeNote(t, dir, "note.md", "Q: keep?\nA: yes.\n\nQ: delete?\nA: gone.\n")
	if _, err := s.SyncAll(); err != nil {
		t.Fatal(err)
	}
	if len(cardIDs(t, s)) != 2 {
		t.Fatal("want 2 cards")
	}

	// Remove the second card block (keep the first, with its anchor).
	content := readNote(t, dir, "note.md")
	idx := strings.Index(content, "Q: delete?")
	writeNote(t, dir, "note.md", content[:idx])

	res, err := s.SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	if res.CardsOrphaned != 1 {
		t.Errorf("orphaned = %d, want 1", res.CardsOrphaned)
	}
	if len(cardIDs(t, s)) != 1 {
		t.Errorf("active cards = %v", cardIDs(t, s))
	}

	// Restore the block with its original anchor → un-orphaned, same id.
	writeNote(t, dir, "note.md", content)
	if _, err := s.SyncAll(); err != nil {
		t.Fatal(err)
	}
	if len(cardIDs(t, s)) != 2 {
		t.Errorf("restore failed, active = %v", cardIDs(t, s))
	}
}

func TestRenameKeepsCardHistory(t *testing.T) {
	s, dir := newTestSyncer(t)
	writeNote(t, dir, "old.md", "Q: survives a rename?\nA: it should.\n")
	if _, err := s.SyncAll(); err != nil {
		t.Fatal(err)
	}
	id := cardIDs(t, s)[0]

	// Rename the file (anchor travels with content).
	content := readNote(t, dir, "old.md")
	if err := os.Remove(filepath.Join(dir, "old.md")); err != nil {
		t.Fatal(err)
	}
	writeNote(t, dir, "renamed.md", content)

	res, err := s.SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	if res.CardsCreated != 0 || res.CardsOrphaned != 0 {
		t.Errorf("rename should neither create nor orphan: %+v", res)
	}
	var notePath string
	if err := s.Store.DB.QueryRow(`SELECT note_path FROM cards WHERE id = ?`, id).Scan(&notePath); err != nil {
		t.Fatal(err)
	}
	if notePath != "renamed.md" {
		t.Errorf("note_path = %q", notePath)
	}
	// Old note row is gone.
	var n int
	s.Store.DB.QueryRow(`SELECT COUNT(*) FROM notes WHERE path = 'old.md'`).Scan(&n)
	if n != 0 {
		t.Error("old note row still present")
	}
}

func TestDeletedFileOrphansItsCards(t *testing.T) {
	s, dir := newTestSyncer(t)
	writeNote(t, dir, "doomed.md", "Q: q?\nA: a.\n")
	if _, err := s.SyncAll(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "doomed.md")); err != nil {
		t.Fatal(err)
	}
	res, err := s.SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	if res.CardsOrphaned != 1 {
		t.Errorf("orphaned = %d, want 1", res.CardsOrphaned)
	}
}

func TestOpenQuestionLifecycle(t *testing.T) {
	s, dir := newTestSyncer(t)
	writeNote(t, dir, "skim.md", "---\nstage: skim\n---\n## Open questions\n- What is a sigma-algebra, really?\n- Why countable additivity?\n")
	if _, err := s.SyncAll(); err != nil {
		t.Fatal(err)
	}

	var n int
	s.Store.DB.QueryRow(`SELECT COUNT(*) FROM open_questions WHERE status = 'open'`).Scan(&n)
	if n != 2 {
		t.Fatalf("open questions = %d, want 2", n)
	}

	// Remove one question from the file → dropped, not deleted.
	writeNote(t, dir, "skim.md", "---\nstage: skim\n---\n## Open questions\n- What is a sigma-algebra, really?\n")
	if _, err := s.SyncAll(); err != nil {
		t.Fatal(err)
	}
	s.Store.DB.QueryRow(`SELECT COUNT(*) FROM open_questions WHERE status = 'open'`).Scan(&n)
	if n != 1 {
		t.Errorf("open = %d, want 1", n)
	}
	s.Store.DB.QueryRow(`SELECT COUNT(*) FROM open_questions WHERE status = 'dropped'`).Scan(&n)
	if n != 1 {
		t.Errorf("dropped = %d, want 1", n)
	}
}

func TestSyncFixtures(t *testing.T) {
	// Run against a copy of the repo fixtures as an integration smoke test.
	s, dir := newTestSyncer(t)
	for _, rel := range []string{"ml/variational-inference.md", "decks/linear-algebra.md"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "notes", rel))
		if err != nil {
			t.Fatal(err)
		}
		writeNote(t, dir, rel, string(data))
	}
	res, err := s.SyncAll()
	if err != nil {
		t.Fatal(err)
	}
	// 7 cards total; one already has an anchor in the fixture.
	if res.Notes != 2 || res.CardsCreated != 7 {
		t.Errorf("res = %+v, want 2 notes / 7 cards", res)
	}
	if res.OpenQuestions != 2 {
		t.Errorf("open questions = %d", res.OpenQuestions)
	}
	// Deck comes from the folder.
	var deck string
	s.Store.DB.QueryRow(`SELECT deck FROM cards WHERE note_path = 'decks/linear-algebra.md' LIMIT 1`).Scan(&deck)
	if deck != "decks" {
		t.Errorf("deck = %q", deck)
	}
	// A sync event was logged.
	var n int
	s.Store.DB.QueryRow(`SELECT COUNT(*) FROM activity_events WHERE kind = 'sync'`).Scan(&n)
	if n != 1 {
		t.Errorf("sync events = %d", n)
	}
}
