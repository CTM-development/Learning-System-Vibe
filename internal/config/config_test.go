package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8844 {
		t.Errorf("Port = %d, want 8844", cfg.Port)
	}
	if cfg.NotesDir != "notes" || cfg.AttachmentsDir != "attachments" || cfg.DBPath != "learning.db" {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "port: 9000\nnotes_dir: /data/notes\ndb_path: /data/learning.db\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9000 {
		t.Errorf("Port = %d, want 9000", cfg.Port)
	}
	if cfg.NotesDir != "/data/notes" {
		t.Errorf("NotesDir = %q", cfg.NotesDir)
	}
	// Not set in YAML: keeps default.
	if cfg.AttachmentsDir != "attachments" {
		t.Errorf("AttachmentsDir = %q, want default", cfg.AttachmentsDir)
	}
}

func TestEnvOverridesYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("port: 9000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LEARN_PORT", "9001")
	t.Setenv("LEARN_NOTES_DIR", "/env/notes")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9001 {
		t.Errorf("Port = %d, want 9001 (env wins)", cfg.Port)
	}
	if cfg.NotesDir != "/env/notes" {
		t.Errorf("NotesDir = %q, want /env/notes", cfg.NotesDir)
	}
}

func TestInvalidPort(t *testing.T) {
	t.Setenv("LEARN_PORT", "0")
	if _, err := Load(""); err == nil {
		t.Error("want error for port 0")
	}
	t.Setenv("LEARN_PORT", "abc")
	if _, err := Load(""); err == nil {
		t.Error("want error for non-numeric port")
	}
}

func TestMissingFile(t *testing.T) {
	if _, err := Load("/nonexistent/config.yaml"); err == nil {
		t.Error("want error for missing config file")
	}
}
