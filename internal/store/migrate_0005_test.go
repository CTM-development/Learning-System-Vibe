package store

import (
	"io/fs"
	"path/filepath"
	"sort"
	"testing"
)

// TestMigration0005RebuildsPopulatedSources simulates upgrading a live
// pre-M10 database: migrations 0001-0004 applied, data present. 0005 must
// rebuild sources without losing rows, keep the kind CHECK (now including
// 'scan') and default notes.type to 'reading'.
func TestMigration0005RebuildsPopulatedSources(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Apply only 0001-0004, recording them the way Migrate does.
	if _, err := st.DB.Exec(`CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY, name TEXT NOT NULL,
		applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')))`); err != nil {
		t.Fatal(err)
	}
	names, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			t.Fatal(err)
		}
		if version >= 5 {
			continue
		}
		sqlBytes, _ := migrationsFS.ReadFile(name)
		if _, err := st.DB.Exec(string(sqlBytes)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		if _, err := st.DB.Exec(`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`, version, name); err != nil {
			t.Fatal(err)
		}
	}

	// Live data in the old schema.
	if _, err := st.DB.Exec(
		`INSERT INTO sources (kind, key, path, title) VALUES ('pdf', 'bishop2006', 'pdfs/bishop2006.pdf', 'PRML')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO notes (path, title) VALUES ('vi.md', 'VI')`); err != nil {
		t.Fatal(err)
	}

	// The upgrade: only 0005 remains to apply.
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}

	var kind, key string
	if err := st.DB.QueryRow(`SELECT kind, key FROM sources WHERE id = 1`).Scan(&kind, &key); err != nil {
		t.Fatal(err)
	}
	if kind != "pdf" || key != "bishop2006" {
		t.Errorf("migrated source = %s/%s", kind, key)
	}

	var noteType string
	if err := st.DB.QueryRow(`SELECT type FROM notes WHERE path = 'vi.md'`).Scan(&noteType); err != nil {
		t.Fatal(err)
	}
	if noteType != "reading" {
		t.Errorf("existing note type = %q, want reading", noteType)
	}

	// 'scan' is now a legal kind, garbage still is not, keys stay unique.
	if _, err := st.DB.Exec(`INSERT INTO sources (kind, key) VALUES ('scan', 's1')`); err != nil {
		t.Errorf("insert scan source: %v", err)
	}
	if _, err := st.DB.Exec(`INSERT INTO sources (kind, key) VALUES ('floppy', 's2')`); err == nil {
		t.Error("kind CHECK lost in rebuild")
	}
	if _, err := st.DB.Exec(`INSERT INTO sources (kind, key) VALUES ('pdf', 'bishop2006')`); err == nil {
		t.Error("key UNIQUE lost in rebuild")
	}

	// AUTOINCREMENT continues past migrated ids.
	var maxID int64
	if err := st.DB.QueryRow(`SELECT MAX(id) FROM sources`).Scan(&maxID); err != nil {
		t.Fatal(err)
	}
	if maxID < 2 {
		t.Errorf("max source id = %d", maxID)
	}
}
