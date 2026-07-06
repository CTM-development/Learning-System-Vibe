package store

import (
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMigrateCreatesSchema(t *testing.T) {
	s := openTestStore(t)

	want := []string{
		"notes", "cards", "card_schedule", "sessions",
		"activity_events", "sources", "open_questions",
		"notes_fts", "sources_fts", "schema_migrations",
	}
	for _, table := range want {
		var name string
		err := s.DB.QueryRow(
			`SELECT name FROM sqlite_master WHERE name = ?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := openTestStore(t)
	if err := s.Migrate(); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("schema_migrations rows = %d, want 1", n)
	}
}

func TestFTSRoundTrip(t *testing.T) {
	s := openTestStore(t)
	_, err := s.DB.Exec(
		`INSERT INTO notes_fts (path, title, content) VALUES (?, ?, ?)`,
		"ml/variational-inference.md", "Variational Inference",
		"The ELBO lower-bounds the marginal log likelihood.",
	)
	if err != nil {
		t.Fatal(err)
	}
	var path string
	err = s.DB.QueryRow(
		`SELECT path FROM notes_fts WHERE notes_fts MATCH ?`, "elbo",
	).Scan(&path)
	if err != nil {
		t.Fatalf("FTS match: %v", err)
	}
	if path != "ml/variational-inference.md" {
		t.Errorf("path = %q", path)
	}
}

func TestActivityEventWithSession(t *testing.T) {
	s := openTestStore(t)
	res, err := s.DB.Exec(`INSERT INTO sessions (kind) VALUES ('learning')`)
	if err != nil {
		t.Fatal(err)
	}
	sid, _ := res.LastInsertId()
	_, err = s.DB.Exec(
		`INSERT INTO activity_events (kind, ref, elapsed_ms, session_id, payload)
		 VALUES ('card_review', 'a1b2c3', 4200, ?, '{"rating":3}')`, sid,
	)
	if err != nil {
		t.Fatal(err)
	}
	// Foreign keys are on: a bogus session_id must fail.
	_, err = s.DB.Exec(
		`INSERT INTO activity_events (kind, session_id) VALUES ('sync', 99999)`,
	)
	if err == nil {
		t.Error("want FK violation for bogus session_id")
	}
}

func TestSessionKindConstraint(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.DB.Exec(`INSERT INTO sessions (kind) VALUES ('gaming')`); err == nil {
		t.Error("want CHECK violation for invalid session kind")
	}
}
