package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// backupKeep is how many daily snapshots Backup retains.
const backupKeep = 7

// Backup writes a consistent snapshot of the database to
// dir/learning-YYYYMMDD.db via VACUUM INTO and prunes old snapshots,
// keeping the newest backupKeep. If today's snapshot already exists it is
// left alone. Returns the snapshot path ("" when skipped).
func (s *Store) Backup(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(dir, "learning-"+time.Now().Format("20060102")+".db")
	if _, err := os.Stat(target); err == nil {
		return "", nil // already snapshotted today
	}
	// VACUUM INTO takes a string literal; single quotes in the path must be
	// doubled (the path is config-controlled, this is belt and braces).
	quoted := strings.ReplaceAll(target, "'", "''")
	if _, err := s.DB.Exec("VACUUM INTO '" + quoted + "'"); err != nil {
		return "", fmt.Errorf("vacuum into %s: %w", target, err)
	}

	// Prune: snapshot names sort chronologically.
	matches, err := filepath.Glob(filepath.Join(dir, "learning-*.db"))
	if err != nil {
		return target, err
	}
	sort.Strings(matches)
	for _, old := range matches[:max(0, len(matches)-backupKeep)] {
		if err := os.Remove(old); err != nil {
			return target, err
		}
	}
	return target, nil
}
