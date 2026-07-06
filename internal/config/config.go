// Package config loads server configuration from an optional YAML file,
// with environment-variable overrides and sensible defaults.
package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config holds all server settings.
type Config struct {
	Port           int    `yaml:"port"`
	NotesDir       string `yaml:"notes_dir"`
	AttachmentsDir string `yaml:"attachments_dir"`
	DBPath         string `yaml:"db_path"`
	NewPerDay      int    `yaml:"new_per_day"` // new cards introduced per day
}

// Default returns the configuration used when no file or env overrides exist.
func Default() Config {
	return Config{
		Port:           8844,
		NotesDir:       "notes",
		AttachmentsDir: "attachments",
		DBPath:         "learning.db",
		NewPerDay:      10,
	}
}

// Load reads configuration from the YAML file at path (if path is non-empty),
// then applies environment overrides (LEARN_PORT, LEARN_NOTES_DIR,
// LEARN_ATTACHMENTS_DIR, LEARN_DB_PATH) on top of the defaults.
func Load(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config %s: %w", path, err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", path, err)
		}
	}

	if v := os.Getenv("LEARN_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("LEARN_PORT: %w", err)
		}
		cfg.Port = p
	}
	if v := os.Getenv("LEARN_NOTES_DIR"); v != "" {
		cfg.NotesDir = v
	}
	if v := os.Getenv("LEARN_ATTACHMENTS_DIR"); v != "" {
		cfg.AttachmentsDir = v
	}
	if v := os.Getenv("LEARN_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("LEARN_NEW_PER_DAY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("LEARN_NEW_PER_DAY: %w", err)
		}
		cfg.NewPerDay = n
	}

	if cfg.Port < 1 || cfg.Port > 65535 {
		return cfg, fmt.Errorf("invalid port %d", cfg.Port)
	}
	return cfg, nil
}
